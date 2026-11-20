package logical_flattening_rewriter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func logical(op foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
	return must(expressions.NewLogicalExpression(op, children))
}

func reference(name string) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(must(ids.NewColumnID("flattening", name)), must(types.NewBoolType(foundation.MayNull)), name))
}

func TestFlattenLogicalRegions(t *testing.T) {
	for _, operator := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		t.Run(operator.String(), func(t *testing.T) { testFlattenRegions(t, operator) })
	}
}

func testFlattenRegions(t *testing.T, operator foundation.LogicalOperator) {
	a, b, c, d := reference("a"), reference("b"), reference("c"), reference("d")
	group := func(children ...expressions.Expression) *expressions.LogicalExpression {
		return logical(operator, children...)
	}
	opposite := foundation.Or
	if operator == foundation.Or {
		opposite = foundation.And
	}
	boundaryGroup := func(children ...expressions.Expression) *expressions.LogicalExpression {
		return logical(opposite, children...)
	}
	yes := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), true))))
	no := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), false))))
	shared := group(a, b)
	boundary := boundaryGroup(b, c)
	for _, tc := range []struct {
		name         string
		source, want expressions.Expression
		removed      int
	}{
		{"left skew", group(group(group(a, b), c), d), group(a, b, c, d), 2},
		{"right skew", group(a, group(b, group(c, d))), group(a, b, c, d), 2},
		{"balanced", group(group(a, b), group(c, d)), group(a, b, c, d), 2},
		{"n-ary", group(a, group(b, c, d), yes), group(a, b, c, d, yes), 1},
		{"shared occurrences", group(shared, shared), group(a, b, a, b), 2},
		{"constants retained", group(a, group(yes, no, b)), group(a, yes, no, b), 1},
		{"opposite boundary retained", group(group(a, boundary), d), group(a, boundary, d), 1},
		{"matching group below opposite boundary", group(a, boundaryGroup(group(b, group(c, d)), yes)), group(a, boundaryGroup(group(b, c, d), yes)), 1},
		{"matching group below NOT", logical(foundation.Not, group(a, group(b, c))), logical(foundation.Not, group(a, b, c)), 1},
		{"opposite region flattened", group(a, boundaryGroup(b, boundaryGroup(c, d))), group(a, boundaryGroup(b, c, d)), 1},
		{"both regions flattened", group(group(a, b), boundaryGroup(c, boundaryGroup(d, a))), group(a, b, boundaryGroup(c, d, a)), 2},
		{"flat unchanged", group(a, b, c), group(a, b, c), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := expressions.PrettyPrint(tc.source)
			sourceChildren := tc.source.Children()
			tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
			got := must(tree.Rewrite(tc.source, struct{}{}))
			if !got.EqualsTo(tc.want) || !got.Type().EqualsTo(tc.source.Type()) {
				t.Fatalf("unexpected result\n%s", expressions.PrettyPrint(got))
			}
			if tc.removed == 0 && got != tc.source {
				t.Fatal("unchanged input lost identity")
			}
			if expressions.PrettyPrint(tc.source) != before {
				t.Fatal("source mutated")
			}
			for i, child := range tc.source.Children() {
				if child != sourceChildren[i] {
					t.Fatal("source children changed")
				}
			}
			oldSize, newSize := reconstruction.MeasureExpression(tc.source), reconstruction.MeasureExpression(got)
			if oldSize.NodeCount-newSize.NodeCount != tc.removed || oldSize.LiteralOccurrences != newSize.LiteralOccurrences || oldSize.MembershipEntries != newSize.MembershipEntries {
				t.Fatal("unexpected structural counts", oldSize, newSize)
			}
			if tc.name == "shared occurrences" {
				for i, want := range []expressions.Expression{a, b, a, b} {
					if got.Children()[i] != want {
						t.Fatal("flattening copied/dropped a boundary occurrence")
					}
				}
			}
			if tc.name == "opposite boundary retained" && got.Children()[1] != boundary {
				t.Fatal("unchanged opposite boundary lost identity")
			}
		})
	}
}

// Instrument the public traversal: logical-region processing must not cause
// the generic service to visit any boundary occurrence a second time.
type boundaryObserver struct {
	*logical_flattening_rewriter.LogicalFlatteningRewriter
	visits      []*expressions.ReferenceExpression
	failAt      *expressions.ReferenceExpression
	failure     error
	replaceAt   *expressions.ReferenceExpression
	replacement expressions.Expression
}

func (r *boundaryObserver) RewriteReference(node *expressions.ReferenceExpression, _ struct{}, _ rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	r.visits = append(r.visits, node)
	if node == r.failAt {
		return foundation.Optional[expressions.Expression]{}, r.failure
	}
	if node == r.replaceAt {
		return foundation.Some(r.replacement), nil
	}
	return foundation.Optional[expressions.Expression]{}, nil
}

func TestLogicalBoundaryTraversalOncePerOccurrence(t *testing.T) {
	for _, operator := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		t.Run(operator.String(), func(t *testing.T) { testBoundaryTraversal(t, operator) })
	}
}

func testBoundaryTraversal(t *testing.T, operator foundation.LogicalOperator) {
	a, b, c := reference("a"), reference("b"), reference("c")
	opposite := foundation.Or
	if operator == foundation.Or {
		opposite = foundation.And
	}
	shared := logical(operator, a, b)
	for _, root := range []expressions.Expression{
		logical(operator, a, b, a, b, c),
		logical(operator, shared, shared, c),
		logical(operator, logical(opposite, shared, shared), c),
	} {
		rule := &boundaryObserver{LogicalFlatteningRewriter: logical_flattening_rewriter.NewLogicalFlatteningRewriter()}
		must(rewriter.NewExpressionTreeRewriter[struct{}](rule).Rewrite(root, struct{}{}))
		want := []*expressions.ReferenceExpression{a, b, a, b, c}
		if len(rule.visits) != len(want) {
			t.Fatal("incorrect boundary visit count", len(rule.visits))
		}
		for i, v := range want {
			if rule.visits[i] != v {
				t.Fatal("boundary order changed")
			}
		}
	}
}

func TestAndBoundaryChangeAndFailure(t *testing.T) {
	a, b, c, d := reference("a"), reference("b"), reference("c"), reference("d")
	replacement := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(a.Type().(*types.BoolType), true))))
	rule := &boundaryObserver{LogicalFlatteningRewriter: logical_flattening_rewriter.NewLogicalFlatteningRewriter(), replaceAt: a, replacement: replacement}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](rule)
	flat := logical(foundation.And, a, b)
	got := must(tree.Rewrite(flat, struct{}{}))
	if got == flat || got.Children()[0] != replacement || got.Children()[1] != b {
		t.Fatal("changed boundary did not rebuild an otherwise flat region")
	}
	failure := errors.New("boundary rewrite failed")
	source := logical(foundation.And, logical(foundation.And, a, b), c, d)
	before := expressions.PrettyPrint(source)
	rule.visits = nil
	rule.failAt, rule.failure = c, failure
	got, err := tree.Rewrite(source, struct{}{})
	if got != nil || !errors.Is(err, failure) || expressions.PrettyPrint(source) != before {
		t.Fatal("boundary failure returned a partial/mutated tree", err)
	}
	if len(rule.visits) != 3 || rule.visits[0] != a || rule.visits[1] != b || rule.visits[2] != c {
		t.Fatal("wrong traversal order at failure")
	}
}

// Generic traversal rebuilds a call when a Boolean argument changes, while
// preserving its handle and Type and never invoking function evaluation.
func TestFlattenInsideCall(t *testing.T) {
	a, b, c, d := reference("a"), reference("b"), reference("c"), reference("d")
	argument := logical(foundation.And, a, logical(foundation.Or, b, logical(foundation.Or, c, d)))
	provider := &unchangedContext{FunctionContext: must(testbed.NewStandardContext())}
	handle := must(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{argument.Type()}))
	source := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{argument}))
	expectedArgument := logical(foundation.And, a, logical(foundation.Or, b, c, d))
	want := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{expectedArgument}))
	before := expressions.PrettyPrint(source)
	tree := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
	got := must(tree.Rewrite(source, struct{}{}))
	if !got.EqualsTo(want) || !got.Type().EqualsTo(source.Type()) {
		t.Fatalf("unexpected call result\n%s", expressions.PrettyPrint(got))
	}
	if expressions.PrettyPrint(source) != before {
		t.Fatal("source call mutated")
	}
}
