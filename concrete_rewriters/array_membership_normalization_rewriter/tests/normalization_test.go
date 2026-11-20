package array_membership_normalization_rewriter_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestNormalizeSingletonMembership(t *testing.T) {
	for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		for _, negated := range []bool{false, true} {
			t.Run(fmt.Sprintf("nullable-%v/negated-%t", nullability, negated), func(t *testing.T) {
				f := newNormalizationFixture(must(testbed.NewStandardContext()), nullability)
				candidates := f.candidates(8792001)
				elements := candidates.Value().(*testbedvalues.ArrayValue).Elements()
				sourceCall := f.call(functions.BuiltInArrayContainsAny, f.input, candidates)
				expectedCall := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
				var source, want expressions.Expression = sourceCall, expectedCall
				if negated {
					source = must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{source}))
					want = must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{want}))
				}
				before := expressions.PrettyPrint(source)
				children := sourceCall.Children()
				f.context.events = nil
				tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
				result := must(tree.Rewrite(source, struct{}{}))
				if !result.EqualsTo(want) || !result.Type().EqualsTo(source.Type()) {
					t.Fatalf("unexpected replacement\n%s", expressions.PrettyPrint(result))
				}
				callNode := result
				if negated {
					callNode = result.Children()[0]
				}
				call := callNode.(*expressions.CallExpression)
				if call.Context() != f.context || !call.Handle().EqualsTo(expectedCall.Handle()) || call.Arguments()[0] != f.input || call.Arguments()[1].(*expressions.LiteralExpression).Value() != elements[0] {
					t.Fatal("replacement lost paired identity or shared input/element")
				}
				if before != expressions.PrettyPrint(source) || !slices.Equal(children, sourceCall.Children()) || !slices.Equal(elements, candidates.Value().(*testbedvalues.ArrayValue).Elements()) {
					t.Fatal("source or literal payload mutated")
				}
				for _, event := range f.context.events {
					if event.operation == "evaluate" {
						t.Fatal("rewriting evaluated membership")
					}
				}
				f.context.events = nil
				if again := must(tree.Rewrite(result, struct{}{})); again != result || len(f.context.events) != 0 {
					t.Fatal("normalized call was rebuilt or inspected again")
				}
				evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
				for _, numbers := range [][]int64{nil, {42}, {8792001, 8792001}} {
					env := &fixtureEnvironment{bindings: []fixtureBinding{{f.input.ID(), f.arrayValue(f.input.Type().(*types.ArrayType), numbers...)}}}
					if !must(evaluator.Evaluate(source, env)).EqualsTo(must(evaluator.Evaluate(result, env))) {
						t.Fatal("replacement changed evaluation")
					}
				}
			})
		}
	}
}

func TestNormalizationMissingTargetOverload(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.NonNull)
	definition := membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{f.array, f.array}, must(types.NewBoolType(foundation.NonNull)), []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified})
	f.context.FunctionContext = testbed.NewContext(must(testbed.NewCatalog([]testbed.Definition{definition})))
	source := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8793001))
	f.context.events = nil
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	if got := must(tree.Rewrite(source, struct{}{})); got != source {
		t.Fatal("missing contains overload changed source")
	}
	if len(f.context.events) != 2 || f.context.events[0].operation != "metadata" || f.context.events[1].operation != "lookup" || f.context.events[1].name != functions.BuiltInArrayContains {
		t.Fatal("unexpected target resolution sequence")
	}
}

func TestNormalizationProviderFailuresAndRetry(t *testing.T) {
	for _, phase := range []string{"source metadata", "lookup", "joined lookup", "target metadata", "construction"} {
		t.Run(phase, func(t *testing.T) {
			f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.MayNull)
			source := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8792001))
			expected := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
			cause := errors.New("provider fault")
			failure := fmt.Errorf("%w: %w", functions.ErrUnsupportedLookup, cause)
			if phase == "joined lookup" {
				failure = errors.Join(failure, functions.ErrContractViolation)
			}
			if phase == "lookup" {
				failure = cause
			}
			targetQueries := 0
			f.context.events = nil
			f.context.fault = func(event functionEvent) error {
				if event.operation == "metadata" {
					if event.handle.EqualsTo(source.Handle()) && phase == "source metadata" {
						return failure
					}
					if event.handle.EqualsTo(expected.Handle()) {
						targetQueries++
						if phase == "target metadata" || phase == "construction" && targetQueries == 2 {
							return failure
						}
					}
				}
				if event.operation == "lookup" && (phase == "lookup" || phase == "joined lookup") {
					return failure
				}
				return nil
			}
			before := expressions.PrettyPrint(source)
			tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
			got, err := tree.Rewrite(source, struct{}{})
			if got != nil || !errors.Is(err, cause) {
				t.Fatal("provider fault was hidden or published a partial result", err)
			}
			if phase == "joined lookup" && !errors.Is(err, functions.ErrContractViolation) {
				t.Fatal("contract failure category lost")
			}
			if phase != "lookup" && !errors.Is(err, functions.ErrUnsupportedLookup) {
				t.Fatal("wrapped unsupported cause lost")
			}
			if phase == "construction" && targetQueries != 2 {
				t.Fatal("fixture did not reach final metadata query")
			}
			if expressions.PrettyPrint(source) != before {
				t.Fatal("failed rewrite mutated source")
			}
			for _, event := range f.context.events {
				if event.operation == "evaluate" {
					t.Fatal("failed rewrite evaluated")
				}
			}
			f.context.fault = nil
			if retry := must(tree.Rewrite(source, struct{}{})); !retry.EqualsTo(expected) {
				t.Fatal("clean retry failed")
			}
		})
	}
}

// A deliberately incorrect lookup result tests the construction boundary without
// forging malformed metadata or expressions. Clearing it restores normal lookup.
type substitutedTargetContext struct {
	functioninterfaces.FunctionContext
	target foundation.Optional[functioninterfaces.FunctionHandle]
}

func (c *substitutedTargetContext) LookupBuiltInFunction(name functions.BuiltInFunctionName, arguments []types.Type) (functioninterfaces.FunctionHandle, error) {
	if name == functions.BuiltInArrayContains {
		if handle, present := c.target.Get(); present {
			return handle, nil
		}
	}
	return c.FunctionContext.LookupBuiltInFunction(name, arguments)
}

func TestNormalizationPropagatesConstructorBindingFailure(t *testing.T) {
	f := newNormalizationFixture(must(testbed.NewStandardContext()), foundation.MayNull)
	narrow := must(types.NewIntType(types.Int32, foundation.NonNull))
	narrowArray := must(types.NewArrayType(narrow, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	policies := []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}
	catalog := must(testbed.NewCatalog([]testbed.Definition{
		membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{f.array, f.array}, boolean, policies),
		membershipDefinition(functions.BuiltInArrayContains, []types.Type{narrowArray, narrow}, boolean, policies),
		membershipDefinition(functions.BuiltInArrayContains, []types.Type{f.array, f.integer}, boolean, policies),
	}))
	provider := &substitutedTargetContext{FunctionContext: testbed.NewContext(catalog)}
	f.context.FunctionContext = provider
	source := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8792001))
	expected := f.call(functions.BuiltInArrayContains, f.input, f.element(8792001))
	wrongTarget := catalog.Handles()[1]
	provider.target = foundation.Some[functioninterfaces.FunctionHandle](wrongTarget)
	before := expressions.PrettyPrint(source)
	f.context.events = nil
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	result, err := tree.Rewrite(source, struct{}{})
	if result != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatalf("constructor binding failure became a successful fallback: %v", err)
	}
	targetQueries := 0
	for _, event := range f.context.events {
		if event.operation == "evaluate" {
			t.Fatal("failed rewrite evaluated")
		}
		if event.operation == "metadata" && event.handle.EqualsTo(wrongTarget) {
			targetQueries++
		}
	}
	if targetQueries != 2 || expressions.PrettyPrint(source) != before {
		t.Fatal("failure did not reach checked construction or changed the source")
	}
	provider.target = foundation.Optional[functioninterfaces.FunctionHandle]{}
	retry := must(tree.Rewrite(source, struct{}{}))
	if !retry.EqualsTo(expected) || !retry.Type().EqualsTo(source.Type()) {
		t.Fatal("clean retry lost the expected candidate or Type")
	}
}
