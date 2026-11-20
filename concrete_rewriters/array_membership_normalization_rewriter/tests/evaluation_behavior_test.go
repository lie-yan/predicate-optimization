package array_membership_normalization_rewriter_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// One trace interleaves environment reads with function invocations, so equal
// invocation counts alone cannot conceal a change in argument/sibling order.
type executionEnvironment struct {
	fixtureEnvironment
	trace *[]string
}

func (e *executionEnvironment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	*e.trace = append(*e.trace, "array")
	return e.fixtureEnvironment.Lookup(id)
}

// These caller-owned functions are outside membership admission. Their retained
// computations may fail or lack an executor, independently of membership itself.
func executionCall(name string, args []expressions.Expression, result types.Type, trace *[]string, evaluate testbed.Evaluator) (*expressions.CallExpression, *recordingContext) {
	parameters := make([]types.Type, len(args))
	policies := make([]functions.NullPolicy, len(args))
	for i, arg := range args {
		parameters[i] = arg.Type()
		if arg.Type().Nullability() == foundation.MayNull {
			policies[i] = functions.PassNull
		}
	}
	metadata := must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(parameters, result)), name, foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, policies))
	catalog := must(testbed.NewCatalog([]testbed.Definition{{Namespace: "execution.fixture", Name: name, Metadata: metadata}}))
	handle := catalog.Handles()[0]
	context := &recordingContext{FunctionContext: must(testbed.NewContextWithEvaluators(catalog, []testbed.EvaluationBinding{{Handle: handle, Evaluate: evaluate}}))}
	context.fault = func(event functionEvent) error {
		if event.operation == "evaluate" {
			*trace = append(*trace, name)
		}
		return nil
	}
	return must(expressions.NewCallExpression(context, handle, args)), context
}

func TestNormalizationRetainedExecution(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, placement := range []string{"direct", "not", "and", "or", "shared", "enclosing"} {
			for _, outcome := range []string{"hit", "miss", "null", "producer-failure", "reference-failure", "opaque", "unavailable", "enclosing-failure"} {
				if outcome == "enclosing-failure" && placement != "enclosing" {
					continue
				}
				t.Run(fmt.Sprintf("wrapped=%t/%s/%s", wrapped, placement, outcome), func(t *testing.T) {
					f := newNormalizationFixture(boundaryProvider(wrapped), foundation.MayNull)
					var trace []string
					arrayType := f.input.Type().(*types.ArrayType)
					var array valueinterfaces.Value = f.arrayValue(arrayType, 8792001)
					if outcome == "miss" {
						array = f.arrayValue(arrayType, 42)
					}
					if outcome == "null" {
						array = must(testbedvalues.NewNull(arrayType))
					}
					producerFailure := errors.New("array producer failed")
					referenceFailure := errors.New("array lookup failed")
					enclosingFailure := errors.New("enclosing call failed")
					laterFailure := errors.New("later sibling failed")
					env := &executionEnvironment{fixtureEnvironment: fixtureEnvironment{bindings: []fixtureBinding{{id: f.input.ID(), value: array}}}, trace: &trace}
					if outcome == "reference-failure" {
						env.failure = referenceFailure
					}
					evaluator := testbed.Evaluator(func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
						if outcome == "producer-failure" {
							return nil, producerFailure
						}
						return args[0], nil
					})
					if outcome == "unavailable" {
						evaluator = nil
					}
					producer, producerContext := executionCall("producer", []expressions.Expression{f.input}, arrayType, &trace, evaluator)
					var input expressions.Expression = producer
					if outcome == "opaque" {
						input = must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("execution", "array")), arrayType, "opaque array"))
					}
					membership := f.call(functions.BuiltInArrayContainsAny, input, f.candidates(8792001))
					boolType := must(types.NewBoolType(foundation.NonNull))
					// Decisive siblings deliberately exercise the interpreter's eager policy.
					siblingValue := must(testbedvalues.NewBool(boolType, placement != "and"))
					before, beforeContext := executionCall("before", nil, boolType, &trace, func([]valueinterfaces.Value) (valueinterfaces.Value, error) { return siblingValue, nil })
					failingInput := outcome == "producer-failure" || outcome == "reference-failure" || outcome == "opaque" || outcome == "unavailable"
					after, afterContext := executionCall("after", nil, boolType, &trace, func([]valueinterfaces.Value) (valueinterfaces.Value, error) {
						if failingInput {
							return nil, laterFailure
						}
						return siblingValue, nil
					})
					contexts := []*recordingContext{f.context, producerContext, beforeContext, afterContext}
					var source expressions.Expression = membership
					switch placement {
					case "not":
						source = maybeNegateMembership(source, true)
					case "and", "or", "shared":
						operator := foundation.And
						if placement == "or" {
							operator = foundation.Or
						}
						children := []expressions.Expression{before, membership, after}
						if placement == "shared" {
							children = []expressions.Expression{before, membership, membership, after}
						}
						source = must(expressions.NewLogicalExpression(operator, children))
					case "enclosing":
						call, context := executionCall("enclosing", []expressions.Expression{before, membership, after}, membership.Type(), &trace, func(args []valueinterfaces.Value) (valueinterfaces.Value, error) {
							if outcome == "enclosing-failure" {
								return nil, enclosingFailure
							}
							return args[1], nil
						})
						source = call
						contexts = append(contexts, context)
					}
					if err := utils.ValidateForOptimization(source); err != nil {
						t.Fatal(err)
					}
					for _, context := range contexts {
						context.events = nil
					}
					tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
					result := must(tree.Rewrite(source, struct{}{}))
					assertNoBoundaryEvaluation(t, contexts...)
					if len(trace) != 0 || len(env.lookups) != 0 {
						t.Fatal("normalization executed retained input")
					}
					if !result.Type().EqualsTo(source.Type()) {
						t.Fatal("result Type changed")
					}
					if err := utils.ValidateForOptimization(result); err != nil {
						t.Fatal(err)
					}

					// Expected traces are independent of traversal and Evaluate implementation.
					surrounded := placement == "and" || placement == "or" || placement == "shared" || placement == "enclosing"
					var expectedTrace []string
					if surrounded {
						expectedTrace = append(expectedTrace, "before")
					}
					if outcome != "opaque" {
						expectedTrace = append(expectedTrace, "array")
						if outcome != "reference-failure" {
							expectedTrace = append(expectedTrace, "producer")
						}
					}
					membershipCount := 0
					if !failingInput {
						membershipCount = 1
						if placement == "shared" {
							expectedTrace = append(expectedTrace, "array", "producer")
							membershipCount++
						}
						if surrounded {
							expectedTrace = append(expectedTrace, "after")
						}
						if placement == "enclosing" {
							expectedTrace = append(expectedTrace, "enclosing")
						}
					}
					var expectedError error
					switch outcome {
					case "producer-failure":
						expectedError = producerFailure
					case "reference-failure":
						expectedError = referenceFailure
					case "opaque", "unavailable":
						expectedError = interpreter.ErrEvaluationUnavailable
					case "enclosing-failure":
						expectedError = enclosingFailure
					}
					truth := foundation.True
					if outcome == "miss" {
						truth = foundation.False
					}
					if outcome == "null" {
						truth = foundation.Null
					}
					if placement == "not" && truth != foundation.Null {
						if truth == foundation.True {
							truth = foundation.False
						} else {
							truth = foundation.True
						}
					}
					if placement == "and" {
						truth = foundation.False
					}
					if placement == "or" {
						truth = foundation.True
					}
					expectedValue := membershipTruthValue(source.Type().(*types.BoolType), truth)
					for index, expression := range []expressions.Expression{source, result} {
						trace = nil
						env.lookups = nil
						f.context.events = nil
						value, err := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(expression, env)
						if !slices.Equal(trace, expectedTrace) {
							t.Fatalf("tree %d: trace %v, want %v", index, trace, expectedTrace)
						}
						if expectedError != nil {
							if value != nil || !errors.Is(err, expectedError) || errors.Is(err, laterFailure) {
								t.Fatalf("tree %d: value %v, error %v; want first cause %v", index, value, err, expectedError)
							}
							if (outcome == "producer-failure" || outcome == "enclosing-failure") && !errors.Is(err, functions.ErrEvaluationFailure) {
								t.Fatalf("failure category lost: %v", err)
							}
						} else if err != nil || !value.EqualsTo(expectedValue) {
							t.Fatalf("tree %d: value %v, error %v; want %v", index, value, err, expectedValue)
						}
						expectedName := functions.BuiltInArrayContainsAny
						if index == 1 {
							expectedName = functions.BuiltInArrayContains
						}
						count := 0
						for _, event := range f.context.events {
							if event.operation != "evaluate" {
								continue
							}
							count++
							name, present := must(f.context.FunctionContext.Metadata(event.handle)).BuiltInName().Get()
							if !present || name != expectedName {
								t.Fatalf("tree %d evaluated wrong membership: %v", index, name)
							}
						}
						if count != membershipCount {
							t.Fatalf("tree %d: membership invocations %d, want %d", index, count, membershipCount)
						}
					}
				})
			}
		}
	}
}
