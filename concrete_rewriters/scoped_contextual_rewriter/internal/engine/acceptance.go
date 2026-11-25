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
	if assertions.IsNil(result.expression) || !result.expression.Type().EqualsTo(a.source.Type()) {
		return absent, invalidAcceptance("missing candidate or changed result Type")
	}
	if err := a.checkRetainedGuard(result.expression); err != nil {
		return absent, err
	}

	// Private result fields can only be populated by engine construction. Subtree
	// visits and manually constructed results never acquire a completed owner.
	if result.owner != a {
		return absent, invalidAcceptance("candidate is not this attempt's completed construction")
	}

	// Truth/provenance failures remain errors even for an unchanged or oversized
	// candidate. Structural benefit is the final gate, never a substitute for proof.
	if !Improves(reconstruction.MeasureExpression(a.source), reconstruction.MeasureExpression(result.expression)) {
		return absent, nil
	}
	return foundation.Some(result.expression), nil
}

// Checks semantic associations of a constructed attempt without duplicating its state.
func (a *RewriteAttempt) checkAttempt() error {
	if a == nil || a.rule == nil || a.source == nil || a.scope == nil || a.premise == nil || a.input == nil {
		return invalidAcceptance("expected a constructed attempt")
	}
	boolean, ok := a.guard.Expression.Type().(*types.BoolType)
	if !ok || boolean.Nullability() != foundation.NonNull || a.guard.Summary.Source() != a.guard.Expression || !a.guard.Summary.Type().EqualsTo(boolean) {
		return invalidAcceptance("guard must be the complete two-valued summary source")
	}
	if !a.scope.EqualsTo(a.premise.Scope()) || !a.scope.EqualsTo(a.input.Scope()) || a.premise.Expression() != a.guard.Expression || !a.input.Operand().EqualsTo(a.guard.Summary.Operand()) {
		return invalidAcceptance("guard, premise, scope and input disagree")
	}
	premises := a.input.Premises()
	if len(premises) != 1 || !premises[0].EqualsTo(a.premise) {
		return invalidAcceptance("input must depend on exactly the retained guard premise")
	}
	return nil
}

// checkRetainedGuard preserves the original AND path and the exact frozen subtree.
// Finding an equal guard elsewhere, or reaching it through OR, is insufficient.
func (a *RewriteAttempt) checkRetainedGuard(candidate expressions.Expression) error {
	if len(a.guard.Path) == 0 {
		return invalidAcceptance("guard must be a conjunct below the owner")
	}
	var source expressions.Expression = a.source
	for depth, index := range a.guard.Path {
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
	if source != a.guard.Expression || candidate != a.guard.Expression {
		return invalidAcceptance("guard occurrence was lost, moved or replaced")
	}
	return nil
}

func invalidAcceptance(format string, args ...any) error {
	return fmt.Errorf("%w: acceptance: %s", rewriter.ErrInvalidReplacement, fmt.Sprintf(format, args...))
}
