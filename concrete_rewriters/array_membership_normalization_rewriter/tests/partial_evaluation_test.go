package array_membership_normalization_rewriter_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// The sibling is known during partial evaluation; only the array is missing.
// Decisive siblings still leave residuals under the interpreter's eager policy.
func partialMembershipGroup(node expressions.Expression, group string) (expressions.Expression, []fixtureBinding) {
	if group == "direct" {
		return node, nil
	}
	typ := must(types.NewBoolType(foundation.NonNull))
	flag := must(expressions.NewReferenceExpression(must(ids.NewColumnID("partial", "flag")), typ, "flag"))
	operator := foundation.And
	if group == "or-true" || group == "or-false" {
		operator = foundation.Or
	}
	flagValue := group == "and-true" || group == "or-true"
	return must(expressions.NewLogicalExpression(operator, []expressions.Expression{flag, node})), []fixtureBinding{{id: flag.ID(), value: must(testbedvalues.NewBool(typ, flagValue))}}
}

// Identity/decisive siblings transform the independently computed membership
// truth without using either evaluator or the interpreter's truth-table helpers.
func partialGroupTruth(truth foundation.TruthValue, group string) foundation.TruthValue {
	switch group {
	case "and-false":
		return foundation.False
	case "or-true":
		return foundation.True
	default:
		return truth
	}
}

func assertPartialKnown(t *testing.T, actual interpreter.Result, expected valueinterfaces.Value) {
	t.Helper()
	value, known := actual.Known()
	_, residual := actual.Residual()
	if !known || residual || !actual.Type().EqualsTo(expected.Type()) || !value.EqualsTo(expected) {
		t.Fatalf("partial result must be known with expected truth and exact Type: want %v", expected)
	}
}

func TestNormalizationMissingInputsAndKnownNull(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			for _, negated := range []bool{false, true} {
				for _, group := range []string{"direct", "and-true", "and-false", "or-true", "or-false"} {
					for _, candidate := range []int64{8792001, 8793001} {
						t.Run(fmt.Sprintf("wrapped=%t/nullability=%v/not=%t/%s/candidate=%d", wrapped, nullability, negated, group, candidate), func(t *testing.T) {
							verifyPartialMembership(t, wrapped, nullability, negated, group, candidate)
						})
					}
				}
			}
		}
	}
}

func verifyPartialMembership(t *testing.T, wrapped bool, nullability foundation.Nullability, negated bool, group string, candidate int64) {
	t.Helper()
	f := newNormalizationFixture(boundaryProvider(wrapped), nullability)
	call := f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(candidate))
	source, knownBindings := partialMembershipGroup(maybeNegateMembership(call, negated), group)
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	sourceBefore := expressions.PrettyPrint(source)
	f.context.events = nil
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	normalized := must(tree.Rewrite(source, struct{}{}))
	assertNoBoundaryEvaluation(t, f.context)
	expectedType := must(types.NewBoolType(nullability))
	if !normalized.Type().EqualsTo(expectedType) || !source.Type().EqualsTo(expectedType) {
		t.Fatal("normalization changed the declared result Type")
	}
	if err := utils.ValidateForOptimization(normalized); err != nil {
		t.Fatal(err)
	}
	// Check the intended rewrite occurred even if later partial evaluation folds it.
	normalizedCall := normalized
	if group != "direct" {
		normalizedCall = normalized.Children()[1]
	}
	normalizedMembershipCall(t, normalizedCall, negated)

	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	originals := []expressions.Expression{source, normalized}
	residuals := make([]expressions.Expression, len(originals))
	for index, expression := range originals {
		f.context.events = nil
		environment := &fixtureEnvironment{bindings: knownBindings}
		partial := must(evaluator.PartialEvaluate(expression, environment))
		residual, present := partial.Residual()
		_, known := partial.Known()
		if !present || known || !partial.Type().EqualsTo(expectedType) || !residual.Type().EqualsTo(expectedType) {
			t.Fatalf("tree %d: missing array must remain residual with exact Type", index)
		}
		assertNoBoundaryEvaluation(t, f.context)
		if len(environment.lookups) != 1+len(knownBindings) {
			t.Fatal("partial evaluation lost or duplicated input reads")
		}
		if err := utils.ValidateForOptimization(residual); err != nil {
			t.Fatal(err)
		}
		residuals[index] = residual
		// Even a decisive Boolean sibling cannot convert missing input to a Value.
		value, err := evaluator.Evaluate(residual, &fixtureEnvironment{})
		if value != nil || !errors.Is(err, interpreter.ErrUnavailableInput) {
			t.Fatalf("missing residual input became a Value or wrong error: %v, %v", value, err)
		}
	}

	rows := []struct {
		name    string
		numbers []int64
		null    bool
	}{
		{"empty", nil, false},
		{"hit", []int64{candidate}, false},
		{"miss", []int64{42}, false},
		{"duplicate hits", []int64{42, candidate, candidate}, false},
		{"duplicate misses", []int64{42, 42}, false},
	}
	if nullability == foundation.MayNull {
		rows = append(rows, struct {
			name    string
			numbers []int64
			null    bool
		}{"NULL", nil, true})
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var input valueinterfaces.Value = f.arrayValue(f.input.Type().(*types.ArrayType), row.numbers...)
			if row.null {
				input = must(testbedvalues.NewNull(f.input.Type()))
			}
			truth := partialGroupTruth(integerMembershipTruth(row.numbers, candidate, row.null, negated), group)
			expected := membershipTruthValue(expectedType, truth)
			for index, original := range originals {
				// Complete the two residuals independently. Previously resolved sibling
				// bindings are deliberately absent: they must already be literal testbedvalues.
				environment := &fixtureEnvironment{bindings: []fixtureBinding{{id: f.input.ID(), value: input}}}
				actual, err := evaluator.Evaluate(residuals[index], environment)
				if err != nil || !actual.Type().EqualsTo(expectedType) || !actual.EqualsTo(expected) {
					t.Fatalf("tree %d: completed residual differs from oracle: %v, %v", index, actual, err)
				}
				if len(environment.lookups) != 1 || !environment.lookups[0].EqualsTo(f.input.ID()) {
					t.Fatal("residual still requires resolved sibling or duplicates array read")
				}
				assertPartialKnown(t, must(evaluator.PartialEvaluate(residuals[index], &fixtureEnvironment{bindings: environment.bindings})), expected)

				bindings := append([]fixtureBinding{{id: f.input.ID(), value: input}}, knownBindings...)
				full := must(evaluator.Evaluate(original, &fixtureEnvironment{bindings: bindings}))
				if !full.Type().EqualsTo(expectedType) || !full.EqualsTo(expected) {
					t.Fatalf("tree %d: full-input evaluation differs from oracle", index)
				}
				// In particular a present typed NULL is known, unlike an absent binding.
				// AND false / OR true may produce concrete Bool? from the NULL membership.
				assertPartialKnown(t, must(evaluator.PartialEvaluate(original, &fixtureEnvironment{bindings: bindings})), expected)
			}
		})
	}
	if expressions.PrettyPrint(source) != sourceBefore {
		t.Fatal("source changed during normalization or partial evaluation")
	}
}
