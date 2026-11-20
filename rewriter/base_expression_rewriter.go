package rewriter

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// BaseExpressionRewriter provides default methods for dedicated rewriter types
// to embed. Each subtype method delegates to RewriteExpression, which calls the
// optional dedicated Fallback or returns an absent optional for default traversal.
//
// Go embedding is not virtual inheritance: overriding RewriteExpression on an
// outer struct does not redirect inherited methods. Set Fallback to supply
// common behavior for those methods. Configure it before traversal and avoid
// cyclic fallback chains. A zero base requests default traversal for every node.
type BaseExpressionRewriter[C any] struct {
	Fallback ExpressionRewriter[C]
}

// RewriteExpression invokes the general fallback or requests default traversal.
// It does not dispatch by subtype; that belongs to the tree rewriter.
func (a BaseExpressionRewriter[C]) RewriteExpression(node expressions.Expression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNil(node)
	assertions.NotNil(tree)
	if a.Fallback != nil {
		assertions.NotNil(a.Fallback)
		return a.Fallback.RewriteExpression(node, scope, tree)
	}
	return foundation.Optional[expressions.Expression]{}, nil
}

func (a BaseExpressionRewriter[C]) RewriteLiteral(node *expressions.LiteralExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	return a.RewriteExpression(node, scope, tree)
}

func (a BaseExpressionRewriter[C]) RewriteReference(node *expressions.ReferenceExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	return a.RewriteExpression(node, scope, tree)
}

func (a BaseExpressionRewriter[C]) RewriteCall(node *expressions.CallExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	return a.RewriteExpression(node, scope, tree)
}

func (a BaseExpressionRewriter[C]) RewriteLogical(node *expressions.LogicalExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	return a.RewriteExpression(node, scope, tree)
}

func (a BaseExpressionRewriter[C]) RewriteSpecialForm(node *expressions.SpecialFormExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	return a.RewriteExpression(node, scope, tree)
}

func (a BaseExpressionRewriter[C]) RewriteOpaque(node *expressions.OpaqueExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	return a.RewriteExpression(node, scope, tree)
}
