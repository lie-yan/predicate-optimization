// Package folding_rewriter connects partial interpretation to expression rewriting.
// It does not select or iterate optimization rules.
package folding_rewriter

import (
	"errors"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

// FoldingRewriter partially evaluates an entire subtree using the supplied Environment
// as rewrite scope. A present replacement finishes the subtree: the tree service
// must not traverse it again. This preserves interpreter admission/error-order
// decisions and avoids repeatedly interpreting every descendant.
//
// Callers establish valid immutable input with expressions/utils.ValidateForOptimization
// at entry, or controlled construction preserving graph-wide identity/Type consistency.
// The rule does not validate the whole graph; runtime binding/result checks and
// evaluation availability remain interpreter responsibilities.
// Construct with NewFoldingRewriter; the zero value is invalid.
type FoldingRewriter struct {
	evaluator                *interpreter.ExpressionInterpreter
	preserveSemanticFailures bool
}

var _ rewriter.ExpressionRewriter[interfaces.Environment] = (*FoldingRewriter)(nil)

// NewFoldingRewriter chooses the semantic-failure policy explicitly. If preservation is
// enabled, ErrEvaluationFailure returns the original whole subtree, not partially
// folded children. Otherwise it propagates. Contract/provider errors
// always propagate. Unavailable execution follows the interpreter's
// residual behavior. The evaluator must be non-nil.
func NewFoldingRewriter(evaluator *interpreter.ExpressionInterpreter, preserveSemanticFailures bool) *FoldingRewriter {
	if evaluator == nil {
		panic("folding_rewriter: expected non-nil interpreter")
	}
	return &FoldingRewriter{evaluator: evaluator, preserveSemanticFailures: preserveSemanticFailures}
}

func (r *FoldingRewriter) RewriteExpression(node expressions.Expression, environment interfaces.Environment, _ rewriter.ExpressionTreeRewriter[interfaces.Environment]) (foundation.Optional[expressions.Expression], error) {
	if r == nil || r.evaluator == nil {
		panic("folding_rewriter: expected constructed FoldingRewriter")
	}
	result, err := r.evaluator.PartialEvaluate(node, environment)
	if err != nil {
		if r.preserveSemanticFailures && preservableFailure(err) {
			return foundation.Some(node), nil
		}
		return foundation.Optional[expressions.Expression]{}, err
	}
	if value, known := result.Known(); known {
		if literal, ok := node.(*expressions.LiteralExpression); ok && literal.Value().EqualsTo(value) {
			return foundation.Some(node), nil
		}
		literal, err := expressions.NewLiteralExpression(value)
		if err != nil {
			return foundation.Optional[expressions.Expression]{}, err
		}
		return foundation.Some[expressions.Expression](literal), nil
	}
	residual, _ := result.Residual()
	return foundation.Some(residual), nil
}

func preservableFailure(err error) bool {
	if !errors.Is(err, functions.ErrEvaluationFailure) {
		return false
	}
	for _, contract := range []error{
		interpreter.ErrInvalidBinding, interpreter.ErrInvalidResult,
		functions.ErrContractViolation, functions.ErrUnknownHandle, functions.ErrIncompatibleHandle, functions.ErrUnsupportedLookup,
	} {
		if errors.Is(err, contract) {
			return false
		}
	}
	return true
}

func (r *FoldingRewriter) RewriteLiteral(node *expressions.LiteralExpression, environment interfaces.Environment, tree rewriter.ExpressionTreeRewriter[interfaces.Environment]) (foundation.Optional[expressions.Expression], error) {
	return r.RewriteExpression(node, environment, tree)
}

func (r *FoldingRewriter) RewriteReference(node *expressions.ReferenceExpression, environment interfaces.Environment, tree rewriter.ExpressionTreeRewriter[interfaces.Environment]) (foundation.Optional[expressions.Expression], error) {
	return r.RewriteExpression(node, environment, tree)
}

func (r *FoldingRewriter) RewriteCall(node *expressions.CallExpression, environment interfaces.Environment, tree rewriter.ExpressionTreeRewriter[interfaces.Environment]) (foundation.Optional[expressions.Expression], error) {
	return r.RewriteExpression(node, environment, tree)
}

func (r *FoldingRewriter) RewriteLogical(node *expressions.LogicalExpression, environment interfaces.Environment, tree rewriter.ExpressionTreeRewriter[interfaces.Environment]) (foundation.Optional[expressions.Expression], error) {
	return r.RewriteExpression(node, environment, tree)
}

func (r *FoldingRewriter) RewriteSpecialForm(node *expressions.SpecialFormExpression, environment interfaces.Environment, tree rewriter.ExpressionTreeRewriter[interfaces.Environment]) (foundation.Optional[expressions.Expression], error) {
	return r.RewriteExpression(node, environment, tree)
}

func (r *FoldingRewriter) RewriteOpaque(node *expressions.OpaqueExpression, environment interfaces.Environment, tree rewriter.ExpressionTreeRewriter[interfaces.Environment]) (foundation.Optional[expressions.Expression], error) {
	return r.RewriteExpression(node, environment, tree)
}
