package array_exclusion_grouping_rewriter_test

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
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Explicit test schedules finish each pass before starting the next. They do
// not add a production pipeline or modify an active scoped rewrite attempt.
func groupingCompositionStages(t *testing.T, source expressions.Expression, context functioninterfaces.FunctionContext, normalizeFirst bool) []expressions.Expression {
	t.Helper()
	contextual := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, context))
	cleanup := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, context))
	flatten := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
	normalize := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	passes := []rewriter.ExpressionTreeRewriter[struct{}]{contextual, cleanup, flatten, normalize, boundaryTree()}
	if !normalizeFirst {
		passes[3], passes[4] = passes[4], passes[3]
	}
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	stages := []expressions.Expression{source}
	snapshots := []string{expressions.PrettyPrint(source)}
	for _, pass := range passes {
		previous := stages[len(stages)-1]
		result := must(pass.Rewrite(previous, struct{}{}))
		if !result.Type().EqualsTo(source.Type()) || reconstruction.MeasureExpression(result).NodeCount > reconstruction.MeasureExpression(previous).NodeCount {
			t.Fatal("composition changed Type or grew")
		}
		if err := utils.ValidateForOptimization(result); err != nil {
			t.Fatal(err)
		}
		for i, node := range stages {
			if expressions.PrettyPrint(node) != snapshots[i] {
				t.Fatal("composition mutated a prior stage")
			}
		}
		stages = append(stages, result)
		snapshots = append(snapshots, expressions.PrettyPrint(result))
	}
	result := stages[len(stages)-1]
	if again := must(boundaryTree().Rewrite(result, struct{}{})); again != result {
		t.Fatal("final grouping was not stable")
	}
	if again := must(normalize.Rewrite(result, struct{}{})); again != result {
		t.Fatal("final normalization was not stable")
	}
	return stages
}

func TestGroupingCompositionOrders(t *testing.T) {
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		integer := must(types.NewIntType(types.Int64, foundation.NonNull))
		fixture := newMembershipFixture(must(testbed.NewStandardContext()), integer, n)
		one, two, three := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2)), must(testbedvalues.NewInteger(integer, 3))
		source := fixtureLogical(foundation.And, fixtureLiteral(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), true))), fixtureLogical(foundation.And, fixtureExclude(fixture.containsAny(fixture.input, one)), fixtureExclude(fixture.contains(fixture.input, two)), fixtureExclude(fixture.containsAny(fixture.input, three))))
		expected := fixture.expectedGroup(fixture.input, one, two, three)
		for _, first := range []bool{true, false} {
			t.Run(fmt.Sprintf("%v/normalize-first=%t", n, first), func(t *testing.T) {
				stages := groupingCompositionStages(t, source, fixture.context, first)
				if !stages[len(stages)-1].EqualsTo(expected) {
					t.Fatal("explicit schedules disagree with expected group")
				}
			})
		}
	}
}
