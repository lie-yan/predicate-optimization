// Package scoped_reasoning represents input restrictions and their scoped premises.
package scoped_reasoning

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// Immutable identity for one reasoning scope; copying a constructed token preserves identity.
type Scope struct{ identity *scopeIdentity }

// Nonzero size keeps independently allocated scope identities distinct.
type scopeIdentity struct{ marker byte }

// Creates an independent scope without fact storage or premise inheritance.
func NewScope() *Scope { return &Scope{identity: &scopeIdentity{}} }

// Compares scope identity rather than token addresses.
func (s *Scope) EqualsTo(other *Scope) bool {
	s.assertConstructed()
	other.assertConstructed()
	return s.identity == other.identity
}

func (s *Scope) assertConstructed() {
	if s == nil || s.identity == nil {
		panic("scoped_reasoning: expected constructed Scope")
	}
}

// Records one retained Boolean occurrence in this scope; repeated registrations remain distinct.
func (s *Scope) RegisterPremise(expression expressions.Expression) (*Premise, error) {
	s.assertConstructed()
	assertions.NotNil(expression)
	if _, ok := expression.Type().(*types.BoolType); !ok {
		return nil, fmt.Errorf("scoped_reasoning: premise must have Boolean type")
	}
	return &Premise{identity: &premiseIdentity{scope: s, expression: expression}}, nil
}
