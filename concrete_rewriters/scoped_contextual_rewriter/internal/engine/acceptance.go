package engine

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/types"
)

// Accept closes one completed Rewrite result under its retained two-valued guard.
// It validates construction and dependencies before comparing whole-region size.
// A valid unhelpful result returns absence; malformed associations return
// ErrInvalidReplacement without a candidate. No extraction or evaluation is repeated.
func (a *RewriteAttempt) Accept(result RewriteResult) (foundation.Optional[expressions.Expression], error) {
	absent := foundation.Optional[expressions.Expression]{}
	if err := a.checkAttempt(); err != nil {
		return absent, err
	}
	if assertions.IsNil(result.Expression) || !result.Expression.Type().EqualsTo(a.Source.Type()) {
		return absent, invalidAcceptance("missing candidate or changed result Type")
	}
	if err := a.checkRetainedGuard(result.Expression); err != nil {
		return absent, err
	}

	// The traversal receipt covers changes with no separate local step (checked
	// parent rebuilding) as well as steps whose target disappeared during cleanup.
	// A manually assembled or subsequently altered result has no such coverage.
	built := result.construction
	if built == nil || built.owner != a || built.snapshot.Expression != result.Expression || len(built.snapshot.Steps) != len(result.Steps) {
		return absent, invalidAcceptance("candidate is not this attempt's completed construction")
	}
	for i, step := range result.Steps {
		recorded := built.snapshot.Steps[i]
		if !step.Path.EqualsTo(recorded.Path) || step.Source != recorded.Source || step.Before != recorded.Before || step.After != recorded.After {
			return absent, invalidAcceptance("changed construction step %d at %v", i, step.Path)
		}
		proof, present := step.Conclusion.Get()
		originalProof, recordedProof := recorded.Conclusion.Get()
		if present != recordedProof {
			return absent, invalidAcceptance("changed conclusion presence at %v", step.Path)
		}
		// Reuse the existing source/Type/scope/domain/premise checks. Fixed cleanup
		// identities and evaluation eligibility were established during traversal;
		// matching the receipt preserves those decisions without provider queries.
		if _, err := a.NewStep(RewriteContext{Attempt: a, Path: step.Path}, step.Source, step.Before, step.After, step.Conclusion); err != nil {
			return absent, err
		}
		if present && (proof.Summary() != originalProof.Summary() || proof.InputDomain() != originalProof.InputDomain()) {
			return absent, invalidAcceptance("changed conclusion association at %v", step.Path)
		}
	}

	// Truth/provenance failures remain errors even for an unchanged or oversized
	// candidate. Structural benefit is the final gate, never a substitute for proof.
	if !Improves(reconstruction.MeasureExpression(a.Source), reconstruction.MeasureExpression(result.Expression)) {
		return absent, nil
	}
	return foundation.Some(result.Expression), nil
}

// checkAttempt retains the constructor's exact summary-derived restriction and
// occurrence ownership. In particular, a narrower hand-built InputDomain cannot
// retroactively justify the traversal, even if it uses the same premise token.
func (a *RewriteAttempt) checkAttempt() error {
	if a == nil || a.original == nil {
		return invalidAcceptance("expected a constructed attempt")
	}
	original := a.original
	if a.Rule != original.Rule || a.Source != original.Source || a.Scope != original.Scope || a.Premise != original.Premise || a.Input != original.Input || a.Guard.Expression != original.Guard.Expression || a.Guard.Summary != original.Guard.Summary || !a.Guard.Path.EqualsTo(original.Guard.Path) {
		return invalidAcceptance("attempt associations changed after construction")
	}
	boolean, ok := a.Guard.Expression.Type().(*types.BoolType)
	if !ok || boolean.Nullability() != foundation.NonNull || a.Guard.Summary.Source() != a.Guard.Expression || !a.Guard.Summary.Type().EqualsTo(boolean) {
		return invalidAcceptance("guard must be the complete two-valued summary source")
	}
	if !a.Scope.EqualsTo(a.Premise.Scope()) || !a.Scope.EqualsTo(a.Input.Scope()) || a.Premise.Expression() != a.Guard.Expression || !a.Input.Operand().EqualsTo(a.Guard.Summary.Operand()) {
		return invalidAcceptance("guard, premise, scope and input disagree")
	}
	premises := a.Input.Premises()
	if len(premises) != 1 || !premises[0].EqualsTo(a.Premise) {
		return invalidAcceptance("input must depend on exactly the retained guard premise")
	}
	return nil
}

// checkRetainedGuard preserves the original AND path and the exact frozen subtree.
// Finding an equal guard elsewhere, or reaching it through OR, is insufficient.
func (a *RewriteAttempt) checkRetainedGuard(candidate expressions.Expression) error {
	if len(a.Guard.Path) == 0 {
		return invalidAcceptance("guard must be a conjunct below the owner")
	}
	var source expressions.Expression = a.Source
	for depth, index := range a.Guard.Path {
		before, originalAND := source.(*expressions.LogicalExpression)
		after, candidateAND := candidate.(*expressions.LogicalExpression)
		if !originalAND || !candidateAND || before.Operator() != foundation.And || after.Operator() != foundation.And || !before.Type().EqualsTo(after.Type()) || before.ChildCount() != after.ChildCount() {
			return invalidAcceptance("protected AND structure changed at depth %d", depth)
		}
		if index < 0 || index >= before.ChildCount() {
			return invalidAcceptance("invalid guard index %d at depth %d", index, depth)
		}
		source, candidate = before.ChildAt(index), after.ChildAt(index)
	}
	if source != a.Guard.Expression || candidate != a.Guard.Expression {
		return invalidAcceptance("guard occurrence was lost, moved or replaced")
	}
	return nil
}

func invalidAcceptance(format string, args ...any) error {
	return fmt.Errorf("%w: acceptance: %s", rewriter.ErrInvalidReplacement, fmt.Sprintf(format, args...))
}
