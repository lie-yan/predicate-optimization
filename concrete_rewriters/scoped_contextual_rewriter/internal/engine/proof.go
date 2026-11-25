package engine

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/types"
)

// ProveLiteral attempts a constant substitution at an original source occurrence.
// The result is conditional evidence for this attempt, not a published rewrite.
// Absence means no admitted substitution; operational and association errors propagate.
func (a *RewriteAttempt) ProveLiteral(source expressions.Expression, context RewriteContext) (foundation.Optional[RewriteResult], error) {
	absent := foundation.Optional[RewriteResult]{}
	if a == nil || context.Attempt != a || assertions.IsNil(source) {
		return absent, fmt.Errorf("%w: proof at %v: missing source or foreign attempt", rewriter.ErrInvalidReplacement, context.Path)
	}
	original, err := context.Path.Resolve(a.source)
	if err != nil {
		return absent, err
	}
	if original != source {
		return absent, fmt.Errorf("%w: proof at %v: stale source", rewriter.ErrInvalidReplacement, context.Path)
	}
	// Freeze the guard subtree and its AND ancestors. A later visit may rebuild
	// ancestors from accepted child results, but cannot substitute them here.
	if a.guard.Path.IsPrefixOf(context.Path) || context.Path.IsPrefixOf(a.guard.Path) {
		return absent, nil
	}
	boolean, ok := source.Type().(*types.BoolType)
	if !ok {
		return absent, nil
	}

	// Analyze the original predicate, then ask what it means under the attempt's
	// inputs. For x IN [1,2] guarding x == 3, the original true set is {3};
	// disjointness proves False locally without changing that global summary.
	analysis, err := a.rule.extractor.Extract(source)
	if err != nil {
		return absent, err
	}
	if analysis.Status() != fact_extraction.Exact {
		return absent, nil
	}
	summary, _ := analysis.Summary().Get()
	if !summary.Operand().EqualsTo(a.input.Operand()) {
		return absent, nil
	}
	result, err := scoped_reasoning.ProveTruth(summary, a.input)
	if err != nil {
		return absent, err
	}
	conclusion, present := result.Get()
	if !present || conclusion.Kind() != scoped_reasoning.KnownTruth {
		return absent, nil
	}

	// A known value does not alone permit suppressing its computation. Inspect
	// omission eligibility before constructing the replacement; never Evaluate.
	eligible, err := a.rule.cleanup.CanOmit(source)
	if err != nil {
		return absent, err
	}
	if !eligible {
		return absent, nil
	}
	truth, _ := conclusion.Truth().Get()
	literal, err := boolean_cleanup.TruthLiteral(a.rule.factory, boolean, truth)
	if err != nil {
		return absent, err
	}
	step, err := a.NewStep(context, source, source, literal, foundation.Some(conclusion))
	if err != nil {
		return absent, err
	}
	// NewStep owns its path; this fresh slice transfers that ownership to the
	// caller; evidence accessors copy it before exposing it.
	return foundation.Some(RewriteResult{expression: literal, steps: []RewriteStep{step}}), nil
}
