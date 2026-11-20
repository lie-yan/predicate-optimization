package scoped_reasoning

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// InputDomain is an immutable, possibly overapproximated set of inputs available
// in one scope. It is not an exact predicate summary or a rewrite authorization.
// Use NewInputDomain; the zero value is invalid.
type InputDomain struct {
	// scope identifies where this restriction and its premises apply.
	scope *Scope
	// operand identifies the referenced input, retaining its full declared Type.
	operand *expressions.ReferenceExpression
	// possibleValues contains the possible non-null inputs, possibly overapproximated.
	possibleValues value_set.ValueSet
	// nullPossible records whether null is also an available input.
	nullPossible bool
	// premises contains the retained occurrences on which the restriction depends.
	premises []*Premise
}

// NewInputDomain checks representation consistency and copies/deduplicates the
// premise collection in first-occurrence order. All premises must belong to scope.
// The set's Type must match the operand with outer nullability removed; null is
// possible only for a nullable operand. Empty and null-only domains are allowed.
// The caller establishes soundness and supplies all dependencies; construction
// does not infer a restriction from the premises or certify their truth.
// Valid immutable inputs are required; nil/unconstructed tokens assert.
func NewInputDomain(scope *Scope, operand *expressions.ReferenceExpression, possibleValues value_set.ValueSet, nullPossible bool, premises []*Premise) (*InputDomain, error) {
	scope.assertConstructed()
	assertions.NotNilPointer(operand)
	assertions.NotNil(possibleValues)
	if possibleValues.Type().Nullability() != foundation.NonNull || !operand.Type().EqualsToIgnoringNullability(possibleValues.Type()) {
		return nil, fmt.Errorf("%w: input values must match the non-null operand domain", value_set.ErrIncompatibleTypes)
	}
	if nullPossible && operand.Type().Nullability() != foundation.MayNull {
		return nil, fmt.Errorf("scoped_reasoning: non-nullable operand cannot admit null input")
	}
	owned := make([]*Premise, 0, len(premises))
	seen := make(map[*premiseIdentity]struct{}, len(premises))
	for _, premise := range premises {
		premise.assertConstructed()
		if !scope.EqualsTo(premise.Scope()) {
			return nil, fmt.Errorf("scoped_reasoning: premise belongs to another scope or context")
		}
		if _, exists := seen[premise.identity]; !exists {
			seen[premise.identity] = struct{}{}
			owned = append(owned, premise)
		}
	}
	return &InputDomain{scope: scope, operand: operand, possibleValues: possibleValues, nullPossible: nullPossible, premises: owned}, nil
}

func (d *InputDomain) Scope() *Scope { d.assertConstructed(); return d.scope }
func (d *InputDomain) Operand() *expressions.ReferenceExpression {
	d.assertConstructed()
	return d.operand
}

func (d *InputDomain) PossibleValues() value_set.ValueSet {
	d.assertConstructed()
	return d.possibleValues
}
func (d *InputDomain) NullPossible() bool { d.assertConstructed(); return d.nullPossible }

// Premises returns an owned slice sharing immutable occurrence tokens.
func (d *InputDomain) Premises() []*Premise {
	d.assertConstructed()
	return append([]*Premise(nil), d.premises...)
}

func (d *InputDomain) assertConstructed() {
	if d == nil || d.scope == nil {
		panic("scoped_reasoning: expected constructed InputDomain")
	}
}
