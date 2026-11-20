// Package array_exclusion_grouping_rewriter combines compatible array exclusions
// within direct AND groups while preserving exact Types and operand order.
package array_exclusion_grouping_rewriter

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ArrayExclusionGroupingRewriter groups admitted NOT contains/contains-any calls
// in post-order. Array inputs must be references or shared literal nodes, with
// compatible Types and the same safely comparable function context. Callers supply
// stable, successful, effect-free reference lookup and membership semantics.
// Inputs must be valid, immutable and non-nil, established through controlled
// construction or expressions/utils.ValidateForOptimization at entry.
// The rule retains no context or attempt state and performs no evaluation.
// Schedule traversal outside active scoped attempts.
type ArrayExclusionGroupingRewriter struct {
	factory valueinterfaces.ValueFactory
	rewriter.BaseExpressionRewriter[struct{}]
}

var _ rewriter.ExpressionRewriter[struct{}] = (*ArrayExclusionGroupingRewriter)(nil)

// NewArrayExclusionGroupingRewriter retains a non-nil factory for replacement arrays.
// Construct before use; the zero value is invalid.
func NewArrayExclusionGroupingRewriter(factory valueinterfaces.ValueFactory) *ArrayExclusionGroupingRewriter {
	assertions.NotNil(factory)
	return &ArrayExclusionGroupingRewriter{factory: factory}
}

// RewriteLogical visits children once, then considers the rebuilt AND region.
// Returning a present result preserves descendant work without repeating traversal.
func (r *ArrayExclusionGroupingRewriter) RewriteLogical(node *expressions.LogicalExpression, scope struct{}, tree rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(r)
	assertions.NotNil(r.factory)
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	absent := foundation.Optional[expressions.Expression]{}
	rebuilt, err := tree.DefaultRewrite(node, scope)
	if err != nil {
		return absent, err
	}
	result, err := rewriteConjunction(r.factory, rebuilt.(*expressions.LogicalExpression))
	if err != nil {
		return absent, err
	}
	return foundation.Some(result), nil
}
