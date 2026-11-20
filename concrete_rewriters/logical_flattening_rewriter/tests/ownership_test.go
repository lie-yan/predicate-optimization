package logical_flattening_rewriter_test

import (
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Keep the original Children slice as well as a separate copy, so checking
// freshly fetched children alone cannot hide mutation of a previously read slice.
type childSnapshot struct {
	node               expressions.Expression
	borrowed, expected []expressions.Expression
}

func snapshotChildren(node expressions.Expression) []childSnapshot {
	children := node.Children()
	result := []childSnapshot{{node, children, slices.Clone(children)}}
	for _, child := range children {
		result = append(result, snapshotChildren(child)...)
	}
	return result
}

func checkSnapshots(t *testing.T, snapshots []childSnapshot) {
	t.Helper()
	for _, snapshot := range snapshots {
		if !slices.Equal(snapshot.borrowed, snapshot.expected) || !slices.Equal(snapshot.node.Children(), snapshot.expected) {
			t.Fatal("source node or previously returned child slice changed")
		}
	}
}

func TestFlattenOwnershipAndIdempotence(t *testing.T) {
	for _, operator := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		t.Run(operator.String(), func(t *testing.T) {
			a, b, c := reference("a"), reference("b"), reference("c")
			group := func(children ...expressions.Expression) *expressions.LogicalExpression {
				return logical(operator, children...)
			}
			opposite := foundation.Or
			if operator == foundation.Or {
				opposite = foundation.And
			}
			shared := group(a, b)
			boundary := logical(opposite, b, c)
			withBoundary := group(a, boundary)
			truth := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), true))))
			null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(a.Type()))))
			membership := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, a.Type(), []expressions.Expression{a, truth, null}))
			withMembership := group(a, membership, truth)
			for _, tc := range []struct {
				name                       string
				source                     expressions.Expression
				operands                   []expressions.Expression
				visits                     []*expressions.ReferenceExpression
				removed, literals, entries int
			}{
				{"shared matching group", group(shared, shared), []expressions.Expression{a, b, a, b}, []*expressions.ReferenceExpression{a, b, a, b}, 2, 0, 0},
				{"shared opposite boundary", group(withBoundary, withBoundary), []expressions.Expression{a, boundary, a, boundary}, []*expressions.ReferenceExpression{a, b, c, a, b, c}, 2, 0, 0},
				{"already flat shared boundaries", group(boundary, boundary), []expressions.Expression{boundary, boundary}, []*expressions.ReferenceExpression{b, c, b, c}, 0, 0, 0},
				{"shared membership and literals", group(withMembership, withMembership), []expressions.Expression{a, membership, truth, a, membership, truth}, []*expressions.ReferenceExpression{a, a, a, a}, 2, 6, 4},
			} {
				t.Run(tc.name, func(t *testing.T) {
					sourceSnapshots := snapshotChildren(tc.source)
					originalSize := reconstruction.MeasureExpression(tc.source)
					observer := &boundaryObserver{LogicalFlatteningRewriter: logical_flattening_rewriter.NewLogicalFlatteningRewriter()}
					tree := rewriter.NewExpressionTreeRewriter[struct{}](observer)
					result := must(tree.Rewrite(tc.source, struct{}{}))
					if !slices.Equal(result.Children(), tc.operands) {
						t.Fatal("operand order, multiplicity or original instances lost")
					}
					if !slices.Equal(observer.visits, tc.visits) {
						t.Fatal("first pass skipped, repeated or reordered boundary visits")
					}
					if !result.Type().EqualsTo(tc.source.Type()) {
						t.Fatal("result Type changed")
					}
					if tc.removed == 0 && result != tc.source {
						t.Fatal("flat source lost identity")
					}
					checkSnapshots(t, sourceSnapshots)
					size := reconstruction.MeasureExpression(result)
					if originalSize.NodeCount-size.NodeCount != tc.removed || originalSize.LiteralOccurrences != tc.literals || size.LiteralOccurrences != tc.literals || originalSize.MembershipEntries != tc.entries || size.MembershipEntries != tc.entries {
						t.Fatalf("unexpected occurrence counts: before %+v, after %+v", originalSize, size)
					}

					resultSnapshots := snapshotChildren(result)
					observer.visits = nil
					second := must(tree.Rewrite(result, struct{}{}))
					if second != result {
						t.Fatal("second pass lost normalized root identity")
					}
					if !slices.Equal(observer.visits, tc.visits) {
						t.Fatal("second pass skipped, repeated or reordered boundary visits")
					}
					if reconstruction.MeasureExpression(second) != size {
						t.Fatal("second pass changed occurrence counts")
					}
					checkSnapshots(t, sourceSnapshots)
					checkSnapshots(t, resultSnapshots)
				})
			}
		})
	}
}

func TestFlattenIdempotenceAcrossChangedBoundaries(t *testing.T) {
	a, b, c := reference("a"), reference("b"), reference("c")
	// Both occurrences require reconstruction. The contract preserves their
	// operands; it does not require newly reconstructed grouping nodes to be shared.
	shared := logical(foundation.Or, a, logical(foundation.Or, b, c))
	source := logical(foundation.Not, logical(foundation.And, shared, shared))
	snapshots := snapshotChildren(source)
	observer := &boundaryObserver{LogicalFlatteningRewriter: logical_flattening_rewriter.NewLogicalFlatteningRewriter()}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](observer)
	result := must(tree.Rewrite(source, struct{}{}))
	wantVisits := []*expressions.ReferenceExpression{a, b, c, a, b, c}
	if !slices.Equal(observer.visits, wantVisits) {
		t.Fatal("shared changed boundaries were not visited once per occurrence")
	}
	not, ok := result.(*expressions.LogicalExpression)
	if !ok || not.Operator() != foundation.Not {
		t.Fatal("NOT boundary lost")
	}
	and := not.Children()[0].(*expressions.LogicalExpression)
	if and.Operator() != foundation.And || and.ChildCount() != 2 {
		t.Fatal("AND boundary or repeated occurrence lost")
	}
	for _, child := range and.Children() {
		or := child.(*expressions.LogicalExpression)
		if or.Operator() != foundation.Or || !slices.Equal(or.Children(), []expressions.Expression{a, b, c}) {
			t.Fatal("changed OR boundary lost original operands")
		}
	}
	before, after := reconstruction.MeasureExpression(source), reconstruction.MeasureExpression(result)
	if before.NodeCount-after.NodeCount != 2 {
		t.Fatal("shared nested grouping must be removed at both occurrences")
	}
	resultSnapshots := snapshotChildren(result)
	observer.visits = nil
	if second := must(tree.Rewrite(result, struct{}{})); second != result {
		t.Fatal("mixed normalized tree lost identity on second pass")
	}
	if !slices.Equal(observer.visits, wantVisits) {
		t.Fatal("second pass changed occurrence traversal")
	}
	checkSnapshots(t, snapshots)
	checkSnapshots(t, resultSnapshots)
}
