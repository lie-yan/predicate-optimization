// Package array_membership_normalization_rewriter normalizes singleton
// literal contains-any calls to contains for compatible array-membership overloads.
package array_membership_normalization_rewriter

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

// ArrayMembershipNormalizationRewriter normalizes singleton literal candidates
// in post-order, retaining exact Types and input-array evaluation.
// Replacements are resolved through each call's paired context;
// callers supply stable, correct, effect-free and successful admitted membership
// implementations, including their specified null behavior.
// Inputs must be valid, non-nil and immutable. Callers establish validity with
// expressions/utils.ValidateForOptimization at entry, or controlled construction
// preserving graph-wide identity/Type consistency. This pass does not validate the
// whole graph; membership and replacement checks remain its own responsibility.
// Schedule traversal outside active
// scoped rewrite attempts so source occurrence associations remain valid.
type ArrayMembershipNormalizationRewriter struct {
	rewriter.BaseExpressionRewriter[struct{}]
}

var _ rewriter.ExpressionRewriter[struct{}] = (*ArrayMembershipNormalizationRewriter)(nil)

// NewArrayMembershipNormalizationRewriter constructs the stateless rule
// without querying a function context or evaluating expressions.
func NewArrayMembershipNormalizationRewriter() *ArrayMembershipNormalizationRewriter {
	return &ArrayMembershipNormalizationRewriter{}
}

// RewriteCall processes children once, then attempts the local normalization.
// A present rebuilt result on a local miss preserves accepted child changes
// without requesting another default traversal of those same occurrences.
func (r *ArrayMembershipNormalizationRewriter) RewriteCall(node *expressions.CallExpression, scope struct{}, tree rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	assertions.NotNilPointer(r)
	assertions.NotNilPointer(node)
	assertions.NotNil(tree)
	absent := foundation.Optional[expressions.Expression]{}
	rebuilt, err := tree.DefaultRewrite(node, scope)
	if err != nil {
		return absent, err
	}
	candidate, err := normalizeSingleton(rebuilt.(*expressions.CallExpression))
	if err != nil {
		return absent, err
	}
	if _, present := candidate.Get(); present {
		return candidate, nil
	}
	return foundation.Some(rebuilt), nil
}
