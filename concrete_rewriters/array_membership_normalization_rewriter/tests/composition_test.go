package array_membership_normalization_rewriter_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func compositionLogical(operator foundation.LogicalOperator, children ...expressions.Expression) expressions.Expression {
	return must(expressions.NewLogicalExpression(operator, children))
}

// These are explicit test schedules, not a pipeline service. Each public tree
// pass finishes before the next starts, so no active scoped attempt is modified.
func normalizationCompositionStages(t *testing.T, source expressions.Expression, context functioninterfaces.FunctionContext, first bool) []expressions.Expression {
	t.Helper()
	normalize := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	contextual := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, context))
	cleanup := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, context))
	flatten := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
	passes := []rewriter.ExpressionTreeRewriter[struct{}]{contextual, cleanup, flatten, normalize}
	if first {
		passes = []rewriter.ExpressionTreeRewriter[struct{}]{normalize, contextual, cleanup, flatten}
	}
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	stages := []expressions.Expression{source}
	snapshots := captureTraversal(source)
	for i, pass := range passes {
		previous := stages[len(stages)-1]
		before := captureTraversal(previous)
		result := must(pass.Rewrite(previous, struct{}{}))
		if !result.Type().EqualsTo(source.Type()) || reconstruction.MeasureExpression(result).NodeCount > reconstruction.MeasureExpression(previous).NodeCount {
			t.Fatal("composition changed Type or grew")
		}
		if err := utils.ValidateForOptimization(result); err != nil {
			t.Fatal(err)
		}
		assertTraversalUnchanged(t, before)
		assertTraversalUnchanged(t, snapshots)
		if first && i == 0 || !first && i == 3 {
			expectedChanges := singletonContainsAnyCount(previous)
			if changes := assertOnlySingletonNormalization(t, previous, result); changes != expectedChanges {
				t.Fatalf("normalized %d calls, want %d", changes, expectedChanges)
			}
			if reconstruction.MeasureExpression(previous) != reconstruction.MeasureExpression(result) {
				t.Fatal("singleton normalization changed structural counts")
			}
		}
		stages = append(stages, result)
	}
	return stages
}

// Compare corresponding occurrences: only singleton ANY calls may be replaced.
// Their array expression, paired context and candidate Value must be retained.
func assertOnlySingletonNormalization(t *testing.T, source, result expressions.Expression) int {
	t.Helper()
	if source == result {
		return 0
	}
	if call, ok := source.(*expressions.CallExpression); ok {
		name, present := must(call.Context().Metadata(call.Handle())).BuiltInName().Get()
		if present && name == functions.BuiltInArrayContainsAny {
			replacement := normalizedMembershipCall(t, result, false)
			args := call.Arguments()
			candidates := args[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue).Elements()
			actual := replacement.Arguments()
			if len(candidates) != 1 || replacement.Context() != call.Context() || actual[0] != args[0] || actual[1].(*expressions.LiteralExpression).Value() != candidates[0] || !replacement.Type().EqualsTo(call.Type()) {
				t.Fatal("singleton replacement changed input, candidate, context or Type")
			}
			return 1
		}
	}
	if source.ChildCount() != result.ChildCount() || !source.Type().EqualsTo(result.Type()) {
		t.Fatal("normalization changed unrelated structure")
	}
	children := result.Children()
	if !must(source.WithChildren(children)).EqualsTo(result) {
		t.Fatal("normalization changed unrelated node properties")
	}
	changes := 0
	for i, child := range source.Children() {
		changes += assertOnlySingletonNormalization(t, child, children[i])
	}
	return changes
}

func TestNormalizationComposition(t *testing.T) {
	for _, first := range []bool{false, true} {
		for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			t.Run(fmt.Sprintf("first=%t/nullability=%v", first, nullability), func(t *testing.T) {
				f := newNormalizationFixture(boundaryProvider(false), nullability)
				yes := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), true))))
				predicate := compositionLogical(foundation.Not, f.call(functions.BuiltInArrayContainsAny, f.input, f.candidates(8792001)))
				source := compositionLogical(foundation.And, compositionLogical(foundation.And, yes, predicate), yes)
				stages := normalizationCompositionStages(t, source, f.context, first)
				expected := compositionLogical(foundation.Not, f.call(functions.BuiltInArrayContains, f.input, f.element(8792001)))
				if !stages[4].EqualsTo(expected) {
					t.Fatalf("composition did not normalize and clean: %s", expressions.PrettyPrint(stages[4]))
				}
				inputs := []valueinterfaces.Value{f.arrayValue(f.input.Type().(*types.ArrayType)), f.arrayValue(f.input.Type().(*types.ArrayType), 8792001), f.arrayValue(f.input.Type().(*types.ArrayType), 42)}
				if nullability == foundation.MayNull {
					inputs = append(inputs, must(testbedvalues.NewNull(f.input.Type())))
				}
				evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
				for _, input := range inputs {
					environment := &fixtureEnvironment{bindings: []fixtureBinding{{id: f.input.ID(), value: input}}}
					want := must(evaluator.Evaluate(expected, environment))
					for i, stage := range stages {
						if actual := must(evaluator.Evaluate(stage, environment)); !actual.EqualsTo(want) || !actual.Type().EqualsTo(want.Type()) {
							t.Fatalf("stage %d changed truth or Type", i)
						}
					}
				}
			})
		}
	}
}
