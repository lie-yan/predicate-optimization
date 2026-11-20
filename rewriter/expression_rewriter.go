// Package rewriter defines expression rewrite callbacks and traversal contracts.
package rewriter

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
)

// ExpressionRewriter supplies general and subtype-specific callbacks, following
// Presto. The tree service dispatches to the subtype method. BaseExpressionRewriter provides
// defaults, so dedicated rules need only implement the methods they need. C is caller-owned
// scope state and may include analysis facts or services. A callback can pass a
// derived C to recursive tree operations. Scope applies to an occurrence, not a
// globally shared expression node; callers are responsible for isolating mutable
// state across branches.
//
// Inputs must be valid and non-nil except that C follows the caller's contract.
// On error the optional result is ignored. An absent optional requests default
// traversal. A present result must contain a valid, non-nil expression (including
// exclusion of typed nils). It finishes this occurrence without traversing the
// replacement. Returning the original node preserves its subtree; a NULL literal
// is a valid replacement. The tree service checks non-nil and exact result Type
// before accepting a replacement. Rules justify semantic equivalence.
type ExpressionRewriter[C any] interface {
	RewriteExpression(node expressions.Expression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error)
	RewriteLiteral(node *expressions.LiteralExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error)
	RewriteReference(node *expressions.ReferenceExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error)
	RewriteCall(node *expressions.CallExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error)
	RewriteLogical(node *expressions.LogicalExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error)
	RewriteSpecialForm(node *expressions.SpecialFormExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error)
	RewriteOpaque(node *expressions.OpaqueExpression, scope C, tree ExpressionTreeRewriter[C]) (foundation.Optional[expressions.Expression], error)
}

// ExpressionTreeRewriter is the recursive traversal service available to rules.
// NewExpressionTreeRewriter constructs its default implementation.
// Scope carries semantic analysis state. Callbacks execute synchronously.
type ExpressionTreeRewriter[C any] interface {
	// Rewrite invokes the callback at this occurrence. Replacements finish the
	// occurrence; an absent optional traverses children and calls WithChildren.
	Rewrite(node expressions.Expression, scope C) (expressions.Expression, error)
	// DefaultRewrite bypasses only this occurrence's callback. Children still
	// use Rewrite, allowing post-order rules without self-recursion.
	DefaultRewrite(node expressions.Expression, scope C) (expressions.Expression, error)
}
