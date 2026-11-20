package scoped_reasoning

import (
	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/foundation"
)

// ConclusionKind identifies the evidence carried by a ScopedConclusion.
// Its zero value is invalid.
type ConclusionKind uint8

const (
	KnownTruth ConclusionKind = iota + 1
	RestrictedDomain
	Unreachable
)

// ScopedConclusion is immutable evidence relative to an input domain and its
// premises. It is not authorization to replace a predicate globally. Obtain it
// through ProveTruth or RestrictToTrue; the zero value is invalid.
type ScopedConclusion struct {
	// kind distinguishes uniform truth, a conditional restriction and unreachable inputs.
	kind ConclusionKind
	// summary retains the exact predicate being analyzed.
	summary *facts.PredicateSummary
	// input retains the original contextual inputs, including their scope.
	input *InputDomain
	// truth is present only for KnownTruth, including a known Null result.
	truth foundation.Optional[foundation.TruthValue]
	// restricted is present only for RestrictedDomain.
	restricted foundation.Optional[*InputDomain]
	// premises records all dependencies, including the condition for restrictions.
	premises []*Premise
}

func (c ScopedConclusion) Kind() ConclusionKind             { c.assertConstructed(); return c.kind }
func (c ScopedConclusion) Summary() *facts.PredicateSummary { c.assertConstructed(); return c.summary }
func (c ScopedConclusion) InputDomain() *InputDomain        { c.assertConstructed(); return c.input }
func (c ScopedConclusion) Scope() *Scope                    { c.assertConstructed(); return c.input.Scope() }
func (c ScopedConclusion) Truth() foundation.Optional[foundation.TruthValue] {
	c.assertConstructed()
	return c.truth
}

func (c ScopedConclusion) RestrictedDomain() foundation.Optional[*InputDomain] {
	c.assertConstructed()
	return c.restricted
}

// Premises returns an owned slice sharing immutable occurrence tokens.
func (c ScopedConclusion) Premises() []*Premise {
	c.assertConstructed()
	return append([]*Premise(nil), c.premises...)
}

func (c ScopedConclusion) assertConstructed() {
	if c.summary == nil {
		panic("scoped_reasoning: expected constructed ScopedConclusion")
	}
}
