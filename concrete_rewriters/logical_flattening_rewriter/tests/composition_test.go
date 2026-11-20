package logical_flattening_rewriter_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestFlattenContextualComposition(t *testing.T) {
	var events []string
	provider := &observedContext{FunctionContext: must(testbed.NewStandardContext()), events: &events}
	integerType := must(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	x := must(expressions.NewReferenceExpression(must(ids.NewColumnID("composition", "x")), integerType, "x"))
	y := must(expressions.NewReferenceExpression(must(ids.NewColumnID("composition", "y")), integerType, "y"))
	a, b := reference("a"), reference("b")
	integer := func(n int64) *expressions.LiteralExpression {
		return must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(integerType, n))))
	}
	membership := func(column *expressions.ReferenceExpression) *expressions.SpecialFormExpression {
		return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{column, integer(1), integer(2)}))
	}
	equal := func(column *expressions.ReferenceExpression, n int64) *expressions.CallExpression {
		handle := must(provider.ComparisonFunction(foundation.Equal, column.Type(), integerType))
		return must(expressions.NewCallExpression(provider, handle, []expressions.Expression{column, integer(n)}))
	}
	and := func(children ...expressions.Expression) *expressions.LogicalExpression {
		return logical(foundation.And, children...)
	}
	or := func(children ...expressions.Expression) *expressions.LogicalExpression {
		return logical(foundation.Or, children...)
	}
	not := func(child expressions.Expression) *expressions.LogicalExpression {
		return logical(foundation.Not, child)
	}
	guardX, guardY := membership(x), membership(y)
	impossibleX, impossibleY := equal(x, 3), equal(y, 3)
	one, low, high := equal(x, 1), equal(x, 1), equal(x, 2)
	// Expected shapes are specified independently for each schedule. In particular,
	// cleanup can expose an AND after an earlier flattening pass has already ended.
	for _, tc := range []struct {
		name                    string
		source                  expressions.Expression
		before, after           expressions.Expression
		laterBefore, laterAfter expressions.Expression
		outsideShared           bool
	}{
		{
			name:   "nested guard and later useful owner",
			source: and(and(guardX, not(impossibleX)), and(one, one)),
			before: and(guardX, one, one), after: and(guardX, one, one),
			laterBefore: one, laterAfter: one,
		},
		{
			name:   "independently useful siblings",
			source: or(and(and(guardX, not(impossibleX)), a), and(and(guardY, not(impossibleY)), b)),
			before: or(and(guardX, a), and(guardY, b)), after: or(and(guardX, a), and(guardY, b)),
			laterBefore: or(and(guardX, a), and(guardY, b)), laterAfter: or(and(guardX, a), and(guardY, b)),
		},
		{
			name:   "shared target inside and outside owners",
			source: or(and(low, one), or(and(high, one), one)),
			before: or(low, one), after: or(low, one),
			laterBefore: or(low, one), laterAfter: or(low, one), outsideShared: true,
		},
		{
			name:   "cleanup exposes a new AND boundary",
			source: and(guardX, or(and(a, b), impossibleX)),
			before: and(guardX, and(a, b)), after: and(guardX, a, b),
			laterBefore: and(guardX, and(a, b)), laterAfter: and(guardX, a, b),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, flattenFirst := range []bool{true, false} {
				name := "flatten after"
				want, laterWant := tc.after, tc.laterAfter
				if flattenFirst {
					name = "flatten before"
					want, laterWant = tc.before, tc.laterBefore
				}
				t.Run(name, func(t *testing.T) {
					events = nil
					sourceSnapshots := snapshotChildren(tc.source)
					flatten := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
					contextual := rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, provider))
					cleanup := rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, provider))
					passes := []rewriter.ExpressionTreeRewriter[struct{}]{contextual, cleanup, flatten}
					if flattenFirst {
						passes = []rewriter.ExpressionTreeRewriter[struct{}]{flatten, contextual, cleanup}
					}
					stages := []expressions.Expression{tc.source}
					apply := func(pass rewriter.ExpressionTreeRewriter[struct{}]) {
						previous := stages[len(stages)-1]
						snapshots := snapshotChildren(previous)
						result := must(pass.Rewrite(previous, struct{}{}))
						if !result.Type().EqualsTo(tc.source.Type()) {
							t.Fatal("composition changed exact Type")
						}
						if reconstruction.MeasureExpression(result).NodeCount > reconstruction.MeasureExpression(previous).NodeCount {
							t.Fatal("composition stage grew")
						}
						if len(events) != 0 {
							t.Fatal("optimization evaluated functions")
						}
						checkSnapshots(t, snapshots)
						checkSnapshots(t, sourceSnapshots)
						stages = append(stages, result)
					}
					// Each public Rewrite completes before the next starts. Flattening never
					// runs inside an active contextual attempt or edits that attempt's paths.
					for _, pass := range passes {
						apply(pass)
					}
					result := stages[len(stages)-1]
					if !result.EqualsTo(want) {
						t.Fatalf("unexpected composed shape\n%s\nwant\n%s", expressions.PrettyPrint(result), expressions.PrettyPrint(want))
					}
					if tc.outsideShared && result.Children()[1] != one {
						t.Fatal("a guard fact escaped to the shared occurrence outside its owner")
					}
					// Reuse the public rule on the published tree: all ownership/path state
					// must be derived afresh. The first fixture still has useful work to do.
					apply(contextual)
					apply(cleanup)
					later := stages[len(stages)-1]
					if !later.EqualsTo(laterWant) {
						t.Fatalf("unexpected fresh-pass shape\n%s\nwant\n%s", expressions.PrettyPrint(later), expressions.PrettyPrint(laterWant))
					}
					if tc.name == "nested guard and later useful owner" && later == result {
						t.Fatal("fixture failed to exercise fresh contextual work")
					}
					evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
					for _, xv := range []int64{-1, 0, 1, 2, 3, 4} {
						for _, yv := range []int64{-1, 0, 1, 2, 3, 4} {
							for _, av := range []foundation.TruthValue{foundation.True, foundation.False, foundation.Null} {
								for _, bv := range []foundation.TruthValue{foundation.True, foundation.False, foundation.Null} {
									env := truthEnvironment{
										{x.ID(), integer(xv).Value()},
										{y.ID(), integer(yv).Value()},
										{a.ID(), booleanValue(av, a.Type().(*types.BoolType))},
										{b.ID(), booleanValue(bv, b.Type().(*types.BoolType))},
									}
									original := must(evaluator.Evaluate(tc.source, env))
									for i, node := range append(stages, want, laterWant) {
										actual := must(evaluator.Evaluate(node, env))
										if !actual.EqualsTo(original) || !actual.Type().EqualsTo(tc.source.Type()) {
											t.Fatalf("stage %d changed truth/Type at x=%d y=%d a=%v b=%v", i, xv, yv, av, bv)
										}
									}
								}
							}
						}
					}
				})
			}
		})
	}
}
