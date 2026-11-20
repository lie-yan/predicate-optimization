package rewriter

import (
	"errors"
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// ErrInvalidReplacement identifies a present nil result or a result-Type change.
var ErrInvalidReplacement = errors.New("invalid expression replacement")

// NewExpressionTreeRewriter binds a dedicated rule to immutable tree traversal.
// The rule must be non-nil. Nodes and replacements must be valid immutable
// expressions. Callers can establish this with expressions/utils.ValidateForOptimization
// once at pipeline entry, or controlled construction preserving graph-wide identity
// and Type consistency. Passes preserve the invariant without whole-graph validation.
// Each occurrence is
// visited independently, even when several edges share one node.
//
// This component runs a single traversal, without fixpoint iteration or semantic
// equivalence checking. Callbacks and reconstruction execute synchronously.
// Thread safety depends on the supplied rule and caller-owned scope.
func NewExpressionTreeRewriter[C any](rule ExpressionRewriter[C]) ExpressionTreeRewriter[C] {
	assertions.NotNil(rule)
	return &expressionTreeRewriter[C]{rule: rule}
}

type expressionTreeRewriter[C any] struct {
	rule ExpressionRewriter[C]
}

func (t *expressionTreeRewriter[C]) Rewrite(node expressions.Expression, scope C) (expressions.Expression, error) {
	assertions.NotNil(node)
	result, err := t.dispatch(node, scope)
	if err != nil {
		return nil, err
	}
	replacement, present := result.Get()
	if !present {
		return t.DefaultRewrite(node, scope)
	}
	if assertions.IsNil(replacement) {
		return nil, fmt.Errorf("%w: present nil expression", ErrInvalidReplacement)
	}
	if !replacement.Type().EqualsTo(node.Type()) {
		return nil, fmt.Errorf("%w: result type changed from %s to %s", ErrInvalidReplacement, node.Type(), replacement.Type())
	}
	return replacement, nil
}

func (t *expressionTreeRewriter[C]) DefaultRewrite(node expressions.Expression, scope C) (expressions.Expression, error) {
	assertions.NotNil(node)
	children := node.Children()
	changed := false
	for i, child := range children {
		rewritten, err := t.Rewrite(child, scope)
		if err != nil {
			return nil, fmt.Errorf("%T child %d: %w", node, i, err)
		}
		if rewritten != child {
			changed = true
			children[i] = rewritten
		}
	}
	if !changed {
		return node, nil
	}
	rebuilt, err := node.WithChildren(children)
	if err != nil {
		return nil, fmt.Errorf("reconstruct %T: %w", node, err)
	}
	return rebuilt, nil
}

func (t *expressionTreeRewriter[C]) dispatch(node expressions.Expression, scope C) (foundation.Optional[expressions.Expression], error) {
	switch n := node.(type) {
	case *expressions.LiteralExpression:
		return t.rule.RewriteLiteral(n, scope, t)
	case *expressions.ReferenceExpression:
		return t.rule.RewriteReference(n, scope, t)
	case *expressions.CallExpression:
		return t.rule.RewriteCall(n, scope, t)
	case *expressions.LogicalExpression:
		return t.rule.RewriteLogical(n, scope, t)
	case *expressions.SpecialFormExpression:
		return t.rule.RewriteSpecialForm(n, scope, t)
	case *expressions.OpaqueExpression:
		return t.rule.RewriteOpaque(n, scope, t)
	default:
		return t.rule.RewriteExpression(node, scope, t)
	}
}
