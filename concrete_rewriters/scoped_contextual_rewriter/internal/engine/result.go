package engine

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"

	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/types"
)

// RewriteStep records one local derivation within an attempt's original source.
// Records are read-only after construction; expression and conclusion data are shared.
type RewriteStep struct {
	Path       OccurrencePath                                         // Owned location in the original source snapshot.
	Source     expressions.Expression                                 // Original expression at that location.
	Before     expressions.Expression                                 // Expression before the local change, possibly rebuilt from child results.
	After      expressions.Expression                                 // Replacement with the same full declared Type.
	Conclusion foundation.Optional[scoped_reasoning.ScopedConclusion] // KnownTruth evidence, or absence for a fixed cleanup identity.
}

// RewriteResult pairs a tentative expression with the evidence for retained changes.
// Rejected changes return the original expression without their tentative steps.
type RewriteResult struct {
	// construction binds a complete traversal result to its owner and retained evidence.
	construction *completedRewrite
	Expression   expressions.Expression // Tentative subtree; publication requires later whole-region acceptance.
	Steps        []RewriteStep          // Owned evidence, including independent copies of each source path.
}

// NewRewriteResult copies the step slice and its paths, sharing immutable data.
// Callers supply checked steps for retained changes only. Rebuilding a parent
// preserves child evidence; it does not by itself introduce a local proof step.
func NewRewriteResult(expression expressions.Expression, steps []RewriteStep) RewriteResult {
	assertions.NotNil(expression)
	owned := make([]RewriteStep, len(steps))
	for i, step := range steps {
		owned[i] = step
		owned[i].Path = step.Path.Clone()
	}
	return RewriteResult{Expression: expression, Steps: owned}
}

// NewStep checks source, Type and evidence associations for a local derivation.
// A present conclusion must prove the original source's literal replacement.
// An absent conclusion is reserved for fixed logical cleanup helpers after they
// establish the identity and omission eligibility; these checks alone do not
// prove cleanup equivalence. No public arbitrary-replacement API is provided.
func (a *RewriteAttempt) NewStep(context RewriteContext, source, before, after expressions.Expression, conclusion foundation.Optional[scoped_reasoning.ScopedConclusion]) (RewriteStep, error) {
	invalid := func(reason string) (RewriteStep, error) {
		return RewriteStep{}, fmt.Errorf("%w: step at %v: %s", rewriter.ErrInvalidReplacement, context.Path, reason)
	}
	if a == nil || context.Attempt != a {
		return invalid("foreign or missing attempt")
	}
	if assertions.IsNil(source) || assertions.IsNil(before) || assertions.IsNil(after) {
		return invalid("missing expression")
	}
	original, err := context.Path.Resolve(a.Source)
	if err != nil {
		return RewriteStep{}, err
	}
	if original != source {
		return invalid("source does not identify the original occurrence")
	}
	// Neither the frozen guard subtree nor its protected AND ancestors may
	// acquire a substitution/cleanup step. Ancestors may still rebuild children.
	if a.Guard.Path.IsPrefixOf(context.Path) || context.Path.IsPrefixOf(a.Guard.Path) {
		return invalid("step changes the protected guard path or subtree")
	}
	if !source.Type().EqualsTo(before.Type()) || !before.Type().EqualsTo(after.Type()) {
		return invalid("declared Type changed")
	}
	if proof, present := conclusion.Get(); present {
		if before != source {
			return invalid("truth proof must replace the original source")
		}
		if proof.Kind() != scoped_reasoning.KnownTruth {
			return invalid("conclusion is not KnownTruth")
		}
		if proof.Summary().Source() != source {
			return invalid("conclusion describes a different source")
		}
		if proof.InputDomain() != a.Input || !proof.Scope().EqualsTo(a.Scope) {
			return invalid("conclusion uses a foreign scope or input domain")
		}
		if !proof.Summary().Operand().EqualsTo(a.Input.Operand()) {
			return invalid("conclusion describes a different operand")
		}
		premises := proof.Premises()
		if len(premises) != 1 || !premises[0].EqualsTo(a.Premise) || !premises[0].Scope().EqualsTo(a.Scope) {
			return invalid("conclusion does not depend on this attempt's guard premise")
		}
		truth, present := proof.Truth().Get()
		boolean, ok := source.Type().(*types.BoolType)
		if !present || !ok {
			return invalid("missing Boolean truth")
		}
		expected, err := boolean_cleanup.TruthLiteral(a.Rule.factory, boolean, truth)
		if err != nil {
			return RewriteStep{}, err
		}
		literal, ok := after.(*expressions.LiteralExpression)
		if !ok || !literal.EqualsTo(expected) {
			return invalid("replacement does not match the proved truth")
		}
	} else {
		// Cleanup may act on an original logical group rebuilt from accepted child
		// results. Child order/meaning and their evidence remain the caller's duty.
		originalGroup, ok := source.(*expressions.LogicalExpression)
		rebuilt, rebuiltOK := before.(*expressions.LogicalExpression)
		if !ok || !rebuiltOK || originalGroup.Operator() != rebuilt.Operator() {
			return invalid("cleanup requires the same logical form")
		}
		count := rebuilt.ChildCount()
		if count != originalGroup.ChildCount() {
			return invalid("rebuilt logical arity changed before cleanup")
		}
		for i := 0; i < count; i++ {
			if !rebuilt.ChildAt(i).Type().EqualsTo(originalGroup.ChildAt(i).Type()) {
				return invalid("rebuilt child Type changed")
			}
		}
	}
	return RewriteStep{Path: context.Path.Clone(), Source: source, Before: before, After: after, Conclusion: conclusion}, nil
}

// completedRewrite is produced only after Rewrite finishes successfully. The
// snapshot owns its evidence slices; it is never used to reuse a previous result.
type completedRewrite struct {
	owner    *RewriteAttempt
	snapshot RewriteResult
}
