package rewriter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	functions "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

// An occurrence path and facts belong to caller scope, not expression nodes.
type scope struct {
	path string
	fact string
}

type treeFixture struct {
	calls    int
	scopes   []scope
	defaults int
}

func (f *treeFixture) Rewrite(node expressions.Expression, s scope) (expressions.Expression, error) {
	f.calls++
	f.scopes = append(f.scopes, s)
	return node, nil
}

func (f *treeFixture) DefaultRewrite(node expressions.Expression, s scope) (expressions.Expression, error) {
	f.defaults++
	f.scopes = append(f.scopes, s)
	return node, nil
}

func TestBaseExpressionRewriterAllVariants(t *testing.T) {
	state := scope{path: "root", fact: "initial"}
	tree := &treeFixture{}
	leaf := literal()
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), leaf.Type(), "x"))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("adapter", "x")), leaf.Type(), ""))
	logical := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{leaf}))
	in := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, leaf.Type(), []expressions.Expression{ref, leaf}))
	provider := must(functions.NewStandardContext())
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	intRef := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "i")), integer, "i"))
	handle := must(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer}))
	call := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{intRef, intRef}))

	recorder := &generalRule{}
	var rule rewriter.ExpressionRewriter[scope] = rewriter.BaseExpressionRewriter[scope]{Fallback: recorder}
	for _, node := range []expressions.Expression{leaf, ref, call, logical, in, opaque} {
		result := must(dispatch(rule, node, state, tree))
		got, ok := result.Get()
		if !ok || got != node || recorder.node != node || recorder.scope != state || recorder.tree != tree {
			t.Fatalf("general delegation failed for %T", node)
		}
		if !isAbsent(must(dispatch(rewriter.BaseExpressionRewriter[scope]{}, node, state, tree))) {
			t.Fatalf("zero base did not default for %T", node)
		}
	}
	if recorder.calls != 6 || tree.calls != 0 || tree.defaults != 0 {
		t.Fatal("unexpected calls or traversal")
	}
}

func TestDedicatedOverrideOutcomes(t *testing.T) {
	tree := &treeFixture{}
	node := literal()
	recorder := &generalRule{}
	rule := &literalOverride{BaseExpressionRewriter: rewriter.BaseExpressionRewriter[scope]{Fallback: recorder}}
	// An explicit absent result from an override does not invoke the fallback.
	if !isAbsent(must(rule.RewriteLiteral(node, scope{}, tree))) || recorder.calls != 0 {
		t.Fatal("override fell through")
	}
	replacement := literal()
	rule.result = foundation.Some[expressions.Expression](replacement)
	got, ok := must(rule.RewriteLiteral(node, scope{}, tree)).Get()
	if !ok || got != replacement {
		t.Fatal("replacement lost")
	}
	rule.err = errors.New("rule failed")
	if _, err := rule.RewriteLiteral(node, scope{}, tree); !errors.Is(err, rule.err) || recorder.calls != 0 {
		t.Fatal("override error lost")
	}
	recorder.err = errors.New("fallback failed")
	logical := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{node}))
	if _, err := rule.RewriteLogical(logical, scope{}, tree); !errors.Is(err, recorder.err) {
		t.Fatal("fallback error lost")
	}
}

type literalOverride struct {
	rewriter.BaseExpressionRewriter[scope]
	result foundation.Optional[expressions.Expression]
	err    error
}

func (r *literalOverride) RewriteLiteral(*expressions.LiteralExpression, scope, rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	return r.result, r.err
}

type generalRule struct {
	rewriter.BaseExpressionRewriter[scope]
	calls int

	node  expressions.Expression
	scope scope
	tree  rewriter.ExpressionTreeRewriter[scope]
	err   error
}

func (r *generalRule) RewriteExpression(node expressions.Expression, s scope, tree rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	r.calls++
	r.node, r.scope, r.tree = node, s, tree
	return foundation.Some[expressions.Expression](node), r.err
}

func TestGeneralCallbackWithDerivedScopes(t *testing.T) {
	leaf := literal()
	root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{leaf, leaf}))
	tree := &treeFixture{}
	parent := scope{path: "root", fact: "parent"}
	var rule rewriter.ExpressionRewriter[scope] = scopedRule{}
	decision := must(rule.RewriteLogical(root, parent, tree))
	if got, _ := decision.Get(); got != root {
		t.Fatal("replacement changed")
	}
	if tree.calls != 2 || tree.defaults != 1 || tree.scopes[0].fact != "left" || tree.scopes[1].fact != "right" || tree.scopes[2] != parent {
		t.Fatal("derived occurrence scopes were not preserved")
	}

	if parent.fact != "parent" {
		t.Fatal("parent scope mutated")
	}
}

// Dispatch fixture models the visitor dispatch contract independently of traversal.
func dispatch(rule rewriter.ExpressionRewriter[scope], node expressions.Expression, s scope, tree rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	switch n := node.(type) {
	case *expressions.LiteralExpression:
		return rule.RewriteLiteral(n, s, tree)
	case *expressions.ReferenceExpression:
		return rule.RewriteReference(n, s, tree)
	case *expressions.CallExpression:
		return rule.RewriteCall(n, s, tree)
	case *expressions.LogicalExpression:
		return rule.RewriteLogical(n, s, tree)
	case *expressions.SpecialFormExpression:
		return rule.RewriteSpecialForm(n, s, tree)
	case *expressions.OpaqueExpression:
		return rule.RewriteOpaque(n, s, tree)
	}
	return rule.RewriteExpression(node, s, tree)
}

type literalRule struct {
	rewriter.BaseExpressionRewriter[scope]
}

func (literalRule) RewriteLiteral(node *expressions.LiteralExpression, _ scope, _ rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	return foundation.Some[expressions.Expression](node), nil
}

func TestEmbeddedSubtypeOverride(t *testing.T) {
	tree := &treeFixture{}
	node := literal()
	recorder := &generalRule{}
	var callback rewriter.ExpressionRewriter[scope] = literalRule{
		BaseExpressionRewriter: rewriter.BaseExpressionRewriter[scope]{Fallback: recorder},
	}
	got, ok := must(callback.RewriteLiteral(node, scope{}, tree)).Get()
	if !ok || got != node || recorder.calls != 0 {
		t.Fatal("subtype override not selected")
	}
	logical := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{node}))
	must(callback.RewriteLogical(logical, scope{}, tree))
	if recorder.calls != 1 {
		t.Fatal("inherited method did not delegate")
	}
	// Overriding the general method alone does not override inherited methods.
	var generalOnly rewriter.ExpressionRewriter[scope] = &generalRule{}
	if !isAbsent(must(generalOnly.RewriteLiteral(node, scope{}, tree))) {
		t.Fatal("embedding unexpectedly used virtual dispatch")
	}
	if isAbsent(must(generalOnly.RewriteExpression(node, scope{}, tree))) {
		t.Fatal("direct general override was not called")
	}
}

func isAbsent(result foundation.Optional[expressions.Expression]) bool {
	_, present := result.Get()
	return !present
}

type scopedRule struct {
	rewriter.BaseExpressionRewriter[scope]
}

func (scopedRule) RewriteLogical(node *expressions.LogicalExpression, s scope, tr rewriter.ExpressionTreeRewriter[scope]) (foundation.Optional[expressions.Expression], error) {
	left, right := s, s
	left.path, left.fact = "root/0", "left"
	right.path, right.fact = "root/1", "right"
	if _, err := tr.Rewrite(node.Children()[0], left); err != nil {
		return foundation.Optional[expressions.Expression]{}, err
	}
	if _, err := tr.Rewrite(node.Children()[1], right); err != nil {
		return foundation.Optional[expressions.Expression]{}, err
	}
	result, err := tr.DefaultRewrite(node, s)
	if err != nil {
		return foundation.Optional[expressions.Expression]{}, err
	}
	return foundation.Some[expressions.Expression](result), nil
}
