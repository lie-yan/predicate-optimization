package rewriter_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	functions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

type recordingRule struct {
	rewriter.BaseExpressionRewriter[scope]
	visited      []expressions.Expression
	replacements map[expressions.Expression]foundation.Optional[expressions.Expression]
	fail         expressions.Expression
	err          error
}

func (r *recordingRule) RewriteExpression(n expressions.Expression, _ scope, _ rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	r.visited = append(r.visited, n)
	if n == r.fail {
		return foundation.Optional[expressions.Expression]{}, r.err
	}
	return r.replacements[n], nil
}

func treeFor(r *recordingRule) rewriter.ExpressionTreeRewriter[scope] {
	return rewriter.NewExpressionTreeRewriter[scope](rewriter.BaseExpressionRewriter[scope]{Fallback: r})
}

func boolLiteral(v bool) *expressions.LiteralExpression {
	typ := must(types.NewBoolType(foundation.NonNull))
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(typ, v))))
}

func TestTreeTraversalAllVariantsAndSharing(t *testing.T) {
	leaf := literal()
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), leaf.Type(), "x"))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("adapter", "x")), leaf.Type(), "opaque"))
	in := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, leaf.Type(), []expressions.Expression{ref, leaf}))
	provider := must(functions.NewStandardContext())
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	intRef := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "i")), integer, "i"))
	handle := must(provider.ComparisonFunction(foundation.Equal, integer, integer))
	call := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{intRef, intRef}))
	root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{in, call, opaque}))
	r := &recordingRule{}
	got := must(treeFor(r).Rewrite(root, scope{}))
	if got != root {
		t.Fatal("unchanged root not shared")
	}
	want := []expressions.Expression{root, in, ref, leaf, call, intRef, intRef, opaque}
	if !reflect.DeepEqual(r.visited, want) {
		t.Fatal("unexpected dispatch/order or shared occurrence skipped")
	}
	replacement := literal()
	r = &recordingRule{replacements: map[expressions.Expression]foundation.Optional[expressions.Expression]{leaf: foundation.Some[expressions.Expression](replacement)}}
	got = must(treeFor(r).Rewrite(root, scope{}))
	if got == root || got.Children()[0] == in || got.Children()[1] != call || got.Children()[2] != opaque {
		t.Fatal("incorrect reconstruction/sharing")
	}
	if got.Children()[0].Children()[1] != replacement || in.Children()[1] != leaf {
		t.Fatal("replacement or immutability lost")
	}
	if err := utils.ValidateForOptimization(got); err != nil {
		t.Fatal(err)
	}
}

func TestPreserveReplaceAndDefaultBypass(t *testing.T) {
	a, b := boolLiteral(true), boolLiteral(false)
	root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{a, b}))
	for _, replacement := range []expressions.Expression{root, a} {
		r := &recordingRule{replacements: map[expressions.Expression]foundation.Optional[expressions.Expression]{root: foundation.Some(replacement)}}
		got := must(treeFor(r).Rewrite(root, scope{}))
		if got != replacement || len(r.visited) != 1 {
			t.Fatal("replacement recursively revisited or subtree traversed")
		}
	}
	r := &recordingRule{replacements: map[expressions.Expression]foundation.Optional[expressions.Expression]{root: foundation.Some[expressions.Expression](a), b: foundation.Some[expressions.Expression](a)}}
	got := must(treeFor(r).DefaultRewrite(root, scope{}))
	if got == root || got.Children()[1] != a || !reflect.DeepEqual(r.visited, []expressions.Expression{a, b}) {
		t.Fatal("default bypass skipped children or called parent callback")
	}
}

type postOrderRule struct {
	rewriter.BaseExpressionRewriter[scope]
	seen []expressions.Expression
}

func (r *postOrderRule) RewriteLogical(n *expressions.LogicalExpression, s scope, tree rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	rebuilt, err := tree.DefaultRewrite(n, s)
	if err != nil {
		return foundation.Optional[expressions.Expression]{}, err
	}
	r.seen = append(r.seen, n)
	return foundation.Some(rebuilt.Children()[0]), nil
}

func (r *postOrderRule) RewriteLiteral(n *expressions.LiteralExpression, _ scope, _ rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	r.seen = append(r.seen, n)
	return foundation.Optional[expressions.Expression]{}, nil
}

func TestPostOrderRule(t *testing.T) {
	leaf := boolLiteral(true)
	inner := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{leaf, leaf}))
	root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{inner, leaf}))
	r := &postOrderRule{}
	got := must(rewriter.NewExpressionTreeRewriter[scope](r).Rewrite(root, scope{}))
	if got != leaf || !reflect.DeepEqual(r.seen, []expressions.Expression{leaf, leaf, inner, leaf, root}) {
		t.Fatal("post-order contract failed")
	}
}

type occurrenceRule struct {
	rewriter.BaseExpressionRewriter[scope]
	replacement expressions.Expression
	scopes      []scope
}

func (r *occurrenceRule) RewriteLogical(n *expressions.LogicalExpression, s scope, tree rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	children := n.Children()
	for i, child := range children {
		derived := s
		if i == 0 {
			derived.fact = "replace"
		} else {
			derived.fact = "preserve"
		}
		got, err := tree.Rewrite(child, derived)
		if err != nil {
			return foundation.Optional[expressions.Expression]{}, err
		}
		children[i] = got
	}
	got, err := n.WithChildren(children)
	if err != nil {
		return foundation.Optional[expressions.Expression]{}, err
	}
	return foundation.Some(got), nil
}

func (r *occurrenceRule) RewriteReference(n *expressions.ReferenceExpression, s scope, _ rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	r.scopes = append(r.scopes, s)
	if s.fact == "replace" {
		return foundation.Some(r.replacement), nil
	}
	return foundation.Some[expressions.Expression](n), nil
}

func TestSharedNodeUnderDistinctScopes(t *testing.T) {
	leaf := boolLiteral(true)
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), leaf.Type(), "x"))
	root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{ref, ref}))
	r := &occurrenceRule{replacement: leaf}
	parent := scope{fact: "parent"}
	got := must(rewriter.NewExpressionTreeRewriter[scope](r).Rewrite(root, parent))
	if got.Children()[0] != leaf || got.Children()[1] != ref || len(r.scopes) != 2 || parent.fact != "parent" {
		t.Fatal("occurrence scopes leaked")
	}
}

func TestInvalidReplacementsAndErrors(t *testing.T) {
	leaf := boolLiteral(true)
	var typedNil *expressions.LiteralExpression
	for _, replacement := range []expressions.Expression{nil, typedNil, literal()} {
		r := &recordingRule{replacements: map[expressions.Expression]foundation.Optional[expressions.Expression]{leaf: foundation.Some(replacement)}}
		got, err := treeFor(r).Rewrite(leaf, scope{})
		if !errors.Is(err, rewriter.ErrInvalidReplacement) || got != nil {
			t.Fatalf("invalid replacement accepted: %v", err)
		}
	}
	other := boolLiteral(false)
	root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{leaf, other}))
	sentinel := errors.New("rule failure")
	r := &recordingRule{fail: other, err: sentinel, replacements: map[expressions.Expression]foundation.Optional[expressions.Expression]{leaf: foundation.Some[expressions.Expression](other)}}
	got, err := treeFor(r).Rewrite(root, scope{})
	if got != nil || !errors.Is(err, sentinel) || root.Children()[0] != leaf {
		t.Fatal("error or immutable original lost")
	}
}
