// Package scoped_contextual_rewriter provides retained-guard scalar rewriting.
package scoped_contextual_rewriter

import (
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ScopedContextualRewriter simplifies AND regions under a retained two-valued guard.
// Each accepted replacement is valid for the complete region; failed owners leave
// independent nested regions available to the generic tree traversal.
// Construct it with NewScopedContextualRewriter before use.
type ScopedContextualRewriter struct {
	rewriter.BaseExpressionRewriter[struct{}]
	engine *engine.Engine
}

var _ rewriter.ExpressionRewriter[struct{}] = (*ScopedContextualRewriter)(nil)

// NewScopedContextualRewriter shares the function context without
// querying or evaluating it. Nil, including typed nil, violates the precondition
// and panics. Comparison calls retain their own paired contexts.
//
// Providers must supply correct, stable, effect-free and successful evaluation
// for supported comparisons, Boolean NOT and array-membership signatures over
// all valid effective inputs, including the declared null behavior. Reference
// lookup must be stable, successful and effect-free on complete valid inputs.
// These guarantees do not admit arbitrary calls, arithmetic or opaque regions.
//
// Callers establish valid input with expressions/utils.ValidateForOptimization at
// pipeline entry, or controlled construction preserving graph-wide identity/Type
// consistency. This rule does not validate the whole graph; the execution guarantees
// above and proof/guard obligations remain separate.
// Expressions, values and provider semantics remain immutable during rewriting.
// The rule retains no mutable attempt state; concurrent use requires matching
// guarantees from the supplied providers.
// The factory is required and retained by all analysis and cleanup helpers.
func NewScopedContextualRewriter(factory valueinterfaces.ValueFactory, functionContext functioninterfaces.FunctionContext) *ScopedContextualRewriter {
	return &ScopedContextualRewriter{
		engine: engine.New(factory, functionContext),
	}
}

// RewriteLogical tries guards in source order and publishes the first accepted
// complete region. Absence requests ordinary descent into original children,
// without inheriting any failed attempt's assumptions. A present result finishes
// this occurrence; later cleanup and repeated passes belong to orchestration.
func (r *ScopedContextualRewriter) RewriteLogical(node *expressions.LogicalExpression, _ struct{}, _ rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	absent := foundation.Optional[expressions.Expression]{}
	if r == nil || r.engine == nil {
		panic("scoped_contextual_rewriter: expected constructed ScopedContextualRewriter")
	}
	assertions.NotNilPointer(node)
	if node.Operator() != foundation.And {
		return absent, nil
	}
	guards, err := r.engine.SelectGuards(node)
	if err != nil {
		return absent, err
	}
	for _, guard := range guards {
		candidate, err := r.engine.NewAttempt(node, guard)
		if err != nil {
			return absent, err
		}
		attempt, useful := candidate.Get()
		if !useful {
			continue
		}
		result, err := attempt.Rewrite()
		if err != nil {
			return absent, err
		}
		accepted, err := attempt.Accept(result)
		if err != nil {
			return absent, err
		}
		if _, present := accepted.Get(); present {
			return accepted, nil
		}
	}
	return absent, nil
}
