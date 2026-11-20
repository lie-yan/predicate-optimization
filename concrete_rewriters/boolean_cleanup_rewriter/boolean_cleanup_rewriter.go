// Package boolean_cleanup_rewriter simplifies Boolean structure without scoped facts.
package boolean_cleanup_rewriter

import (
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// BooleanCleanupRewriter applies fixed Boolean identities in post-order while
// preserving full declared Types and evaluation eligibility. It carries no guard,
// premise or occurrence-path state. Construct it with NewBooleanCleanupRewriter.
type BooleanCleanupRewriter struct {
	rewriter.BaseExpressionRewriter[struct{}]
	cleanup *boolean_cleanup.Cleaner
}

var _ rewriter.ExpressionRewriter[struct{}] = (*BooleanCleanupRewriter)(nil)

// NewBooleanCleanupRewriter shares a required non-nil function context
// without querying or evaluating it. Calls retain their own paired contexts.
// Supported comparisons, Boolean NOT and array membership must evaluate correctly,
// stably, without effects or failures on all valid effective inputs, including
// null behavior. Reference lookup must be stable, successful and effect-free on
// complete inputs. Arbitrary calls, arithmetic and opaque regions remain outside
// omission admission. Callers establish valid input at entry with
// expressions/utils.ValidateForOptimization or controlled construction preserving
// graph-wide identity/Type consistency; this rule does not validate the whole graph.
// Validity alone does not establish the execution guarantees above.
// Expressions and provider semantics stay immutable in use;
// concurrent use requires matching guarantees from the providers.
// The factory is required and retained for synthesized truth literals.
func NewBooleanCleanupRewriter(factory valueinterfaces.ValueFactory, functionContext functioninterfaces.FunctionContext) *BooleanCleanupRewriter {
	return &BooleanCleanupRewriter{cleanup: boolean_cleanup.New(factory, functionContext)}
}

// RewriteLogical visits children first, then cleans the rebuilt logical group.
// Returning that rebuilt group when no identity applies finishes the occurrence
// and avoids a second default traversal. Other variants inherit ordinary descent.
func (r *BooleanCleanupRewriter) RewriteLogical(node *expressions.LogicalExpression, scope struct{}, tree rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	absent := foundation.Optional[expressions.Expression]{}
	if r == nil || r.cleanup == nil {
		panic("boolean_cleanup_rewriter: expected constructed BooleanCleanupRewriter")
	}
	rebuilt, err := tree.DefaultRewrite(node, scope)
	if err != nil {
		return absent, err
	}
	group := rebuilt.(*expressions.LogicalExpression)
	cleaned, err := r.cleanup.CleanupLogical(group, group.Children())
	if err != nil {
		return absent, err
	}
	if _, present := cleaned.Get(); present {
		return cleaned, nil
	}
	return foundation.Some(rebuilt), nil
}
