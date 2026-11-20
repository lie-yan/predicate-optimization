// Package scoped_reasoning represents input restrictions and their scoped premises.
package scoped_reasoning

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// ScopedFactContext owns an identity domain for scopes and retained occurrences.
// Registration allocates immutable tokens; it neither proves nor evaluates facts.
// Use NewScopedFactContext; the zero value is invalid.
type ScopedFactContext struct{ identity *contextIdentity }

// A nonzero-sized allocation gives independently constructed contexts distinct identities.
type contextIdentity struct{ marker byte }

func NewScopedFactContext() *ScopedFactContext {
	return &ScopedFactContext{identity: &contextIdentity{}}
}

// NewScope creates an independent scope; nesting and premise inheritance are
// deliberately left to later contextual-rewriter design.
func (c *ScopedFactContext) NewScope() *Scope {
	c.assertConstructed()
	return &Scope{identity: &scopeIdentity{owner: c.identity}}
}

// RegisterPremise records one caller-designated retained Boolean occurrence.
// Every registration is distinct, including repeated registration of the same
// expression pointer. The caller establishes its meaning and retains the occurrence.
// Foreign scopes and non-Boolean expressions return errors; nil inputs assert.
func (c *ScopedFactContext) RegisterPremise(scope *Scope, expression expressions.Expression) (*Premise, error) {
	c.assertConstructed()
	scope.assertConstructed()
	assertions.NotNil(expression)
	if scope.identity.owner != c.identity {
		return nil, fmt.Errorf("scoped_reasoning: scope belongs to another context")
	}
	if _, ok := expression.Type().(*types.BoolType); !ok {
		return nil, fmt.Errorf("scoped_reasoning: premise must have Boolean type")
	}
	return &Premise{identity: &premiseIdentity{scope: scope, expression: expression}}, nil
}

func (c *ScopedFactContext) assertConstructed() {
	if c == nil || c.identity == nil {
		panic("scoped_reasoning: expected constructed ScopedFactContext")
	}
}
