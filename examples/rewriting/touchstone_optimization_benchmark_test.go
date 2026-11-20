package rewriting_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/testbed/touchstone"
)

var touchstoneBenchmarkResult expressions.Expression

// BenchmarkTouchstoneOptimization measures the primary 111-node input through
// the same five passes as Example_touchstoneOptimization. Immutable fixtures and
// reusable rewriters are prepared outside timing; every iteration starts from
// the original input, rather than optimizing the preceding iteration's result.
func BenchmarkTouchstoneOptimization(b *testing.B) {
	fixture := fixtures.Must(touchstone.NewPrimaryTouchstone())
	passes := []struct {
		name string
		tree rewriter.ExpressionTreeRewriter[struct{}]
	}{
		{"Contextual", rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, fixture.Context))},
		{"Cleanup", rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, fixture.Context))},
		{"Flattening", rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())},
		{"Normalization", rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())},
		{"Grouping", rewriter.NewExpressionTreeRewriter[struct{}](array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{}))},
	}
	if err := utils.ValidateForOptimization(fixture.Input); err != nil {
		b.Fatal(err)
	}
	stages := []expressions.Expression{fixture.Input}
	for _, pass := range passes {
		stages = append(stages, fixtures.Must(pass.tree.Rewrite(stages[len(stages)-1], struct{}{})))
	}
	if !stages[len(stages)-1].EqualsTo(fixture.Expected) {
		b.Fatal("schedule did not produce the expected final touchstone")
	}

	for _, validate := range []bool{false, true} {
		name := "FivePasses"
		if validate {
			name = "ValidationAndFivePasses"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result := fixture.Input
				if validate {
					if err := utils.ValidateForOptimization(result); err != nil {
						b.Fatal(err)
					}
				}
				for _, pass := range passes {
					var err error
					result, err = pass.tree.Rewrite(result, struct{}{})
					if err != nil {
						b.Fatal(err)
					}
				}
				touchstoneBenchmarkResult = result
			}
			b.StopTimer()
			if !touchstoneBenchmarkResult.EqualsTo(fixture.Expected) {
				b.Fatal("unexpected final tree")
			}
		})
	}

	// Each isolated pass receives its actual intermediate input. These timings
	// locate costs; the directly measured full sequence is the total latency.
	for index, pass := range passes {
		b.Run("Pass/"+pass.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, err := pass.tree.Rewrite(stages[index], struct{}{})
				if err != nil {
					b.Fatal(err)
				}
				touchstoneBenchmarkResult = result
			}
			b.StopTimer()
			if !touchstoneBenchmarkResult.EqualsTo(stages[index+1]) {
				b.Fatal("unexpected intermediate tree")
			}
		})
	}
}
