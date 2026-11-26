package types

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// Immutable declaration syntax, deliberately separate from executable Type descriptors.
// Implementations are package-owned; general type variables and nested patterns are deferred.
type TypePattern interface {
	// Returns the declared outer nullability.
	Nullability() foundation.Nullability
	// Returns the literal parameter names referenced by this pattern in a fresh slice.
	Parameters() []string
	equalPattern(TypePattern, map[string]string, map[string]string) bool
	bindPattern(Type, map[string]literalBinding) bool
	resolvePattern(map[string]literalBinding) (Type, error)
}

// Fixed declaration domain sharing an immutable concrete Type, including its nullability.
// Use the constructor; the zero value is invalid.
type ConcreteTypePattern struct {
	// Shared fixed domain, including all nested constraints.
	typ Type
}

// Wraps a valid concrete Type without copying or weakening its constraints.
func NewConcreteTypePattern(typ Type) *ConcreteTypePattern {
	assertions.NotNil(typ)
	return &ConcreteTypePattern{typ: typ}
}

// Returns the shared concrete domain.
func (p *ConcreteTypePattern) Type() Type { assertions.NotNilPointer(p); return p.typ }

// Returns the declared outer nullability.
func (p *ConcreteTypePattern) Nullability() foundation.Nullability { return p.Type().Nullability() }

// Fixed domains introduce no literal parameters.
func (p *ConcreteTypePattern) Parameters() []string { assertions.NotNilPointer(p); return nil }

func (p *ConcreteTypePattern) equalPattern(other TypePattern, _, _ map[string]string) bool {
	o, ok := other.(*ConcreteTypePattern)
	return ok && p.typ.EqualsTo(o.typ)
}

// VARCHAR declaration whose byte bound is a scoped literal parameter, not a concrete Type.
// Use the constructor; the zero value is invalid.
type VarCharTypePattern struct {
	// Parameter name local to the containing declaration.
	parameter string
	// Declared outer nullability, independent of the byte-bound parameter.
	nullability foundation.Nullability
}

// Declares varchar(parameter); empty names and invalid nullability are rejected.
func NewVarCharTypePattern(parameter string, n foundation.Nullability) (*VarCharTypePattern, error) {
	if parameter == "" {
		return nil, fmt.Errorf("VARCHAR pattern requires a literal parameter name")
	}
	if _, err := newCommon(n); err != nil {
		return nil, err
	}
	return &VarCharTypePattern{parameter: parameter, nullability: n}, nil
}

// Returns the scoped byte-bound parameter name.
func (p *VarCharTypePattern) Parameter() string { assertions.NotNilPointer(p); return p.parameter }

// Returns the declared outer nullability.
func (p *VarCharTypePattern) Nullability() foundation.Nullability {
	assertions.NotNilPointer(p)
	return p.nullability
}

// Returns an independent view of this pattern's single literal parameter.
func (p *VarCharTypePattern) Parameters() []string { return []string{p.Parameter()} }

func (p *VarCharTypePattern) equalPattern(other TypePattern, left, right map[string]string) bool {
	o, ok := other.(*VarCharTypePattern)
	if !ok || p.nullability != o.nullability {
		return false
	}
	if name, seen := left[p.parameter]; seen {
		return name == o.parameter
	}
	if _, seen := right[o.parameter]; seen {
		return false
	}
	left[p.parameter] = o.parameter
	right[o.parameter] = p.parameter
	return true
}

// Compares ordered patterns in one shared scope, allowing bijective parameter renaming.
// Concrete Types and nullability remain exact; inputs must be valid immutable patterns.
func EqualTypePatterns(left, right []TypePattern) bool {
	if len(left) != len(right) {
		return false
	}
	forward, reverse := make(map[string]string), make(map[string]string)
	for i, p := range left {
		assertions.NotNil(p)
		assertions.NotNil(right[i])
		if !p.equalPattern(right[i], forward, reverse) {
			return false
		}
	}
	return true
}
