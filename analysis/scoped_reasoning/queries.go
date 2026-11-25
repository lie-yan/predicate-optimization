package scoped_reasoning

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
)

// ProveTruth proves a uniform predicate result over the supplied possible inputs.
// An absent result means no uniform truth was established, not SQL NULL or an
// unsupported operation. Empty inputs yield Unreachable. Set-operation errors
// propagate. The conclusion depends on every input premise, not on assuming the
// analyzed predicate true. Required inputs must be constructed and immutable.
func ProveTruth(summary *facts.PredicateSummary, input *InputDomain) (foundation.Optional[ScopedConclusion], error) {
	absent := foundation.Optional[ScopedConclusion]{}
	if err := checkOperand(summary, input); err != nil {
		return absent, err
	}
	conclusion := ScopedConclusion{summary: summary, input: input, premises: input.Premises()}
	empty, err := input.PossibleValues().IsEmpty()
	if err != nil {
		return absent, err
	}
	if empty {
		// No non-null inputs: distinguish unreachable inputs from a known null-input outcome.
		if !input.NullPossible() {
			conclusion.kind = Unreachable
		} else {
			conclusion.kind = KnownTruth
			conclusion.truth = summary.NullTruth()
		}
		return foundation.Some(conclusion), nil
	}

	// A total, two-valued non-null summary is uniform exactly on subsets of
	// its true region or regions disjoint from it.
	subset, err := input.PossibleValues().IsSubsetOf(summary.TrueSet())
	if err != nil {
		return absent, err
	}
	truth := foundation.True
	if !subset {
		disjoint, err := input.PossibleValues().IsDisjointFrom(summary.TrueSet())
		if err != nil {
			return absent, err
		}
		if !disjoint {
			return absent, nil
		}
		truth = foundation.False
	}
	// A possible null input must agree with the result for all non-null inputs.
	if nullTruth, _ := summary.NullTruth().Get(); input.NullPossible() && nullTruth != truth {
		return absent, nil
	}
	conclusion.kind = KnownTruth
	conclusion.truth = foundation.Some(truth)
	return foundation.Some(conclusion), nil
}

// RestrictToTrue describes contextual inputs where the predicate is true.
// condition must identify a same-scope occurrence structurally equal to the
// summary source. Its identity is retained even if the restriction is empty or
// unchanged. The caller associates the token with the actual retained occurrence.
// Success always returns a conclusion: RestrictedDomain or Unreachable. This is
// a conditional restriction, not an equivalent replacement for the predicate.
func RestrictToTrue(summary *facts.PredicateSummary, input *InputDomain, condition *Premise) (foundation.Optional[ScopedConclusion], error) {
	absent := foundation.Optional[ScopedConclusion]{}
	if err := checkOperand(summary, input); err != nil {
		return absent, err
	}
	condition.assertConstructed()
	if !input.Scope().EqualsTo(condition.Scope()) {
		return absent, fmt.Errorf("%w: condition premise belongs to another scope", value_set.ErrInvalidInput)
	}
	if !summary.Source().EqualsTo(condition.Expression()) {
		return absent, fmt.Errorf("%w: condition premise must match the summary source", value_set.ErrInvalidInput)
	}

	// Restriction keeps only true inputs; SQL false and null outcomes do not pass.
	possible, err := input.PossibleValues().Intersect(summary.TrueSet())
	if err != nil {
		return absent, err
	}
	nullTruth, _ := summary.NullTruth().Get()
	nullPossible := input.NullPossible() && nullTruth == foundation.True
	premises := append(input.Premises(), condition)
	restricted, err := NewInputDomain(input.Scope(), input.Operand(), possible, nullPossible, premises)
	if err != nil {
		return absent, err
	}
	conclusion := ScopedConclusion{summary: summary, input: input, premises: restricted.Premises()}
	empty, err := possible.IsEmpty()
	if err != nil {
		return absent, err
	}
	// Even unreachability is conditional on retaining the source premise.
	if empty && !nullPossible {
		conclusion.kind = Unreachable
	} else {
		conclusion.kind = RestrictedDomain
		conclusion.restricted = foundation.Some(restricted)
	}
	return foundation.Some(conclusion), nil
}

// checkOperand requires the same column and full declared Type. Set compatibility
// alone cannot establish that two restrictions describe the same input.
func checkOperand(summary *facts.PredicateSummary, input *InputDomain) error {
	operand := summary.Operand() // Also asserts that the summary is constructed.
	input.assertConstructed()
	if !operand.Type().EqualsTo(input.Operand().Type()) {
		return fmt.Errorf("%w: summary and input operand Types differ", value_set.ErrIncompatibleTypes)
	}
	if !operand.EqualsTo(input.Operand()) {
		return fmt.Errorf("%w: summary and input operands differ", value_set.ErrInvalidInput)
	}
	return nil
}
