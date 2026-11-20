// Package logical_flattening_rewriter flattens ordered logical groups.
package logical_flattening_rewriter

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

// LogicalFlatteningRewriter is a stateless dedicated rule for logical flattening.
// It flattens maximal AND/OR regions; NOT uses default traversal.
// Operand order, duplicates and full declared Types are preserved.
// Inputs must be valid, non-nil and immutable. Callers establish validity with
// expressions/utils.ValidateForOptimization at entry, or controlled construction
// preserving graph-wide identity/Type consistency. This pass does not validate the
// whole graph. Invocation is explicitly scheduled
// through ExpressionTreeRewriter, outside any active scoped rewrite attempt.
type LogicalFlatteningRewriter struct {
	rewriter.BaseExpressionRewriter[struct{}]
}

var _ rewriter.ExpressionRewriter[struct{}] = (*LogicalFlatteningRewriter)(nil)

// NewLogicalFlatteningRewriter constructs the rule without querying a provider
// or evaluating expressions. No function context or semantic assumptions are needed.
func NewLogicalFlatteningRewriter() *LogicalFlatteningRewriter {
	return &LogicalFlatteningRewriter{}
}

// RewriteLogical collects an entire AND or OR region before rebuilding it once.
// Matching nested groups are expanded directly; boundary occurrences use the
// tree service exactly once, preserving left-to-right traversal and sharing.
func (r *LogicalFlatteningRewriter) RewriteLogical(node *expressions.LogicalExpression, scope struct{}, tree rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(r)
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	absent := foundation.Optional[expressions.Expression]{}
	operator := node.Operator()
	if operator != foundation.And && operator != foundation.Or {
		return absent, nil
	}

	// Children returns an owned slice. Reverse it so stack pops visit operands
	// in source order; matching groups append their children in that same stack order.
	pending := node.Children()
	for i, j := 0, len(pending)-1; i < j; i, j = i+1, j-1 {
		pending[i], pending[j] = pending[j], pending[i]
	}
	operands := make([]expressions.Expression, 0, len(pending))
	changed := false
	for len(pending) > 0 {
		last := len(pending) - 1
		operand := pending[last]
		pending[last] = nil
		pending = pending[:last]
		if nested, ok := operand.(*expressions.LogicalExpression); ok && nested.Operator() == operator {
			changed = true
			children := nested.Children()
			for i := len(children) - 1; i >= 0; i-- {
				pending = append(pending, children[i])
			}
			continue
		}
		rewritten, err := tree.Rewrite(operand, scope)
		if err != nil {
			return absent, err
		}
		changed = changed || rewritten != operand
		operands = append(operands, rewritten)
	}
	// A present original result ends this occurrence. Absence would request a
	// second default traversal of boundaries already processed above.
	if !changed {
		return foundation.Some[expressions.Expression](node), nil
	}
	rebuilt, err := node.WithChildren(operands)
	if err != nil {
		return absent, err
	}
	return foundation.Some(rebuilt), nil
}
