package engine

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

// Rewrite derives a tentative complete region under this attempt's one guard.
// It does not create nested owners, accept the candidate or publish a replacement.
func (a *RewriteAttempt) Rewrite() (RewriteResult, error) {
	assertions.NotNilPointer(a)
	// Only constructor-established ownership may produce a completed receipt.
	if err := a.checkAttempt(); err != nil {
		return RewriteResult{}, err
	}
	if err := a.checkRetainedGuard(a.Source); err != nil {
		return RewriteResult{}, err
	}
	result, err := a.Visit(a.Source, RewriteContext{Attempt: a})
	if err != nil {
		return RewriteResult{}, err
	}
	// Bind the complete tree and evidence after all child rollback has finished.
	// Visit and NewRewriteResult alone produce tentative, unpublished subtrees.
	result.construction = &completedRewrite{owner: a, snapshot: NewRewriteResult(result.Expression, result.Steps)}
	return result, nil
}

// Visit traverses original source occurrences in stored order. Evidence travels
// with returned expressions, so rejecting a parent discards its entire tentative
// subtree without mutating sibling results or a shared global evidence ledger.
func (a *RewriteAttempt) Visit(source expressions.Expression, context RewriteContext) (RewriteResult, error) {
	if a == nil || context.Attempt != a || assertions.IsNil(source) {
		return RewriteResult{}, fmt.Errorf("%w: visit at %v: missing source or foreign attempt", rewriter.ErrInvalidReplacement, context.Path)
	}
	original, err := context.Path.Resolve(a.Source)
	if err != nil {
		return RewriteResult{}, err
	}
	if original != source {
		return RewriteResult{}, fmt.Errorf("%w: visit at %v: stale source", rewriter.ErrInvalidReplacement, context.Path)
	}
	// The retained guard is immutable as a whole; shared objects elsewhere are
	// separate occurrences and are still visited under the same conditional inputs.
	if a.Guard.Path.IsPrefixOf(context.Path) {
		return RewriteResult{Expression: source}, nil
	}
	protected := context.Path.IsPrefixOf(a.Guard.Path)
	if !protected {
		result, err := a.ProveLiteral(source, context)
		if err != nil {
			return RewriteResult{}, err
		}
		if proven, present := result.Get(); present {
			return proven, nil
		}
	}

	children := source.Children()
	proposed := make([]expressions.Expression, len(children))
	// A recursive child result is private to this invocation. Transfer the first
	// nonempty evidence buffer instead of copying it; subsequent children append
	// in order. No child result survives this transfer. Paths remain independently
	// owned, and rejecting this parent discards the entire tentative buffer.
	var steps []RewriteStep
	changed := false
	for i, child := range children {
		result, err := a.Visit(child, context.Child(i))
		if err != nil {
			return RewriteResult{}, err
		}
		proposed[i] = result.Expression
		changed = changed || result.Expression != child
		if len(steps) == 0 {
			steps = result.Steps
		} else {
			steps = append(steps, result.Steps...)
		}
	}
	before := source
	if changed {
		admitted, err := a.Rule.cleanup.CanChangeChildren(source, proposed)
		if err != nil {
			return RewriteResult{}, err
		}
		if !admitted {
			// Example: foo(P) cannot consume the tentative P -> True. Restore foo(P)
			// and drop all evidence beneath it; another sibling may still retain a proof.
			return RewriteResult{Expression: source}, nil
		}
		before, err = source.WithChildren(proposed)
		if err != nil {
			return RewriteResult{}, fmt.Errorf("%w: rebuild at %v: %w", rewriter.ErrInvalidReplacement, context.Path, err)
		}
	}
	// Protected AND ancestors may rebuild accepted siblings, but no cleanup may
	// remove/reposition the retained guard. Rebuilding itself needs no new step.
	if protected {
		return RewriteResult{Expression: before, Steps: steps}, nil
	}
	if logical, ok := source.(*expressions.LogicalExpression); ok {
		cleaned, err := a.Rule.cleanup.CleanupLogical(logical, proposed)
		if err != nil {
			return RewriteResult{}, err
		}
		if after, present := cleaned.Get(); present && after != before {
			step, err := a.NewStep(context, source, before, after, foundation.Optional[scoped_reasoning.ScopedConclusion]{})
			if err != nil {
				return RewriteResult{}, err
			}
			steps = append(steps, step)
			return RewriteResult{Expression: after, Steps: steps}, nil
		}
	}
	return RewriteResult{Expression: before, Steps: steps}, nil
}
