package array_membership_normalization_rewriter_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// The oracle uses ordinary integer equality, not provider metadata, Value.EqualsTo
// or either membership evaluator. Known NULL is separate from an empty array.
func integerMembershipTruth(input []int64, candidate int64, null, negated bool) foundation.TruthValue {
	if null {
		return foundation.Null
	}
	matched := false
	for _, element := range input {
		if element == candidate {
			matched = true
			break
		}
	}
	if negated {
		matched = !matched
	}
	if matched {
		return foundation.True
	}
	return foundation.False
}

func membershipTruthValue(typ *types.BoolType, truth foundation.TruthValue) valueinterfaces.Value {
	if truth == foundation.Null {
		return must(testbedvalues.NewNull(typ))
	}
	return must(testbedvalues.NewBool(typ, truth == foundation.True))
}

func maybeNegateMembership(node expressions.Expression, negated bool) expressions.Expression {
	if negated {
		return must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{node}))
	}
	return node
}

// Check call form as well as truth: singleton normalization must produce contains,
// not merely preserve an equivalent source or fold a known input to a literal.
func normalizedMembershipCall(t *testing.T, result expressions.Expression, negated bool) *expressions.CallExpression {
	t.Helper()
	if negated {
		node, ok := result.(*expressions.LogicalExpression)
		if !ok || node.Operator() != foundation.Not {
			t.Fatal("NOT wrapper was lost")
		}
		result = node.Children()[0]
	}
	call, ok := result.(*expressions.CallExpression)
	if !ok {
		t.Fatal("normalization did not retain a call")
	}
	name, present := must(call.Context().Metadata(call.Handle())).BuiltInName().Get()
	if !present || name != functions.BuiltInArrayContains {
		t.Fatal("singleton call did not normalize to contains")
	}
	return call
}

func TestNormalizationMembershipTruthAndTypes(t *testing.T) {
	candidates := []int64{8792001, 8793001, -1, 0, math.MinInt64, math.MaxInt64}
	checked := 0
	for _, wrapped := range []bool{false, true} {
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			for _, negated := range []bool{false, true} {
				for _, candidate := range candidates {
					t.Run(fmt.Sprintf("wrapped=%t/nullability=%v/not=%t/candidate=%d", wrapped, nullability, negated, candidate), func(t *testing.T) {
						f := newNormalizationFixture(boundaryProvider(wrapped), nullability)
						sourceCall := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(candidate))
						source := maybeNegateMembership(sourceCall, negated)
						expectedType := must(types.NewBoolType(nullability))
						if err := utils.ValidateForOptimization(source); err != nil {
							t.Fatal(err)
						}
						f.context.events = nil
						tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
						result := must(tree.Rewrite(source, struct{}{}))
						assertNoBoundaryEvaluation(t, f.context)
						call := normalizedMembershipCall(t, result, negated)
						if call.Arguments()[0] != f.input || call.Context() != f.context {
							t.Fatal("array input or paired context changed")
						}
						if !source.Type().EqualsTo(expectedType) || !result.Type().EqualsTo(expectedType) {
							t.Fatal("effective Type differs from the declared input nullability")
						}
						if err := utils.ValidateForOptimization(result); err != nil {
							t.Fatal(err)
						}
						rows := []struct {
							name    string
							numbers []int64
							null    bool
						}{
							{"empty", nil, false},
							{"miss", []int64{42}, false},
							{"hit", []int64{candidate}, false},
							{"repeated misses", []int64{42, 42}, false},
							{"repeated hits", []int64{candidate, candidate}, false},
							{"first hit", []int64{candidate, 42}, false},
							{"last hit", []int64{42, candidate}, false},
							{"mixed and extrema", []int64{-1, 0, 8792001, 8793001, math.MinInt64, math.MaxInt64, 42}, false},
							{"extrema neighbors", []int64{math.MinInt64 + 1, math.MaxInt64 - 1}, false},
							{"negative and zero", []int64{-2, -1, 0, 1, 2}, false},
						}
						if nullability == foundation.MayNull {
							rows = append(rows, struct {
								name    string
								numbers []int64
								null    bool
							}{"known NULL", nil, true})
						}
						evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
						for _, row := range rows {
							var input valueinterfaces.Value = f.arrayValue(f.input.Type().(*types.ArrayType), row.numbers...)
							if row.null {
								input = must(testbedvalues.NewNull(f.input.Type()))
							}
							want := membershipTruthValue(expectedType, integerMembershipTruth(row.numbers, candidate, row.null, negated))
							// Each tree is evaluated separately with a fresh environment;
							// neither evaluator's result is used as the other's oracle.
							for _, node := range []expressions.Expression{source, result} {
								environment := &fixtureEnvironment{bindings: []fixtureBinding{{f.input.ID(), input}}}
								actual, err := evaluator.Evaluate(node, environment)
								if err != nil {
									t.Fatalf("%s: %v", row.name, err)
								}
								if !actual.Type().EqualsTo(expectedType) || !actual.EqualsTo(want) {
									t.Fatalf("%s: truth or full Type disagrees with independent oracle", row.name)
								}
							}
							checked++
						}
					})
				}
			}
		}
	}
	t.Logf("checked %d source/replacement/oracle cases", checked)
}

func TestNormalizationKnownNullArrayRemainsCall(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, negated := range []bool{false, true} {
			for _, candidate := range []int64{8792001, 8793001, -1, 0, math.MinInt64, math.MaxInt64} {
				t.Run(fmt.Sprintf("wrapped=%t/not=%t/candidate=%d", wrapped, negated, candidate), func(t *testing.T) {
					f := newNormalizationFixture(boundaryProvider(wrapped), foundation.MayNull)
					nullArray := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(f.input.Type()))))
					sourceCall := f.call(functions.BuiltInArrayContainsAny, nullArray, f.candidates(candidate))
					source := maybeNegateMembership(sourceCall, negated)
					if err := utils.ValidateForOptimization(source); err != nil {
						t.Fatal(err)
					}
					before := expressions.PrettyPrint(source)
					f.context.events = nil
					tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
					result := must(tree.Rewrite(source, struct{}{}))
					assertNoBoundaryEvaluation(t, f.context)
					call := normalizedMembershipCall(t, result, negated)
					if call.Arguments()[0] != nullArray {
						t.Fatal("known-null input was replaced or folded")
					}
					expectedType := must(types.NewBoolType(foundation.MayNull))
					want := membershipTruthValue(expectedType, integerMembershipTruth(nil, candidate, true, negated))
					evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
					for _, node := range []expressions.Expression{source, result} {
						if !node.Type().EqualsTo(expectedType) {
							t.Fatal("NULL input lost its nullable result Type")
						}
						actual := must(evaluator.Evaluate(node, &fixtureEnvironment{}))
						if !actual.Type().EqualsTo(expectedType) || !actual.EqualsTo(want) {
							t.Fatal("NULL propagation or NOT(NULL) differs from oracle")
						}
					}
					if expressions.PrettyPrint(source) != before {
						t.Fatal("normalization mutated known-null source")
					}
					if err := utils.ValidateForOptimization(result); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}
