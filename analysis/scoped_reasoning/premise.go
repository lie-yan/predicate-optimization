package scoped_reasoning

import "github.com/lie-yan/predicate-optimization/expressions"

// Premise identifies one retained expression occurrence, not a proposition's proof.
// Instances share an immutable expression and scope; copying preserves identity.
// Obtain premises through RegisterPremise; the zero value is invalid.
type (
	Premise         struct{ identity *premiseIdentity }
	premiseIdentity struct {
		scope      *Scope
		expression expressions.Expression
	}
)

func (p *Premise) Scope() *Scope { p.assertConstructed(); return p.identity.scope }
func (p *Premise) Expression() expressions.Expression {
	p.assertConstructed()
	return p.identity.expression
}

func (p *Premise) EqualsTo(other *Premise) bool {
	p.assertConstructed()
	other.assertConstructed()
	return p.identity == other.identity
}

func (p *Premise) assertConstructed() {
	if p == nil || p.identity == nil {
		panic("scoped_reasoning: expected constructed Premise")
	}
}
