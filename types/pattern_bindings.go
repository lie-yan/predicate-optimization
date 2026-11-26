package types

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// Immutable literal substitutions for one successful argument-pattern match.
// Instances do not retain caller slices; use BindTypePatterns to construct them.
type PatternBindings struct {
	// Parameter values scoped to the matched declaration.
	literals map[string]literalBinding
}

// Finite numeric literal or explicit unbounded marker; no integer is reserved as a sentinel.
type literalBinding struct {
	number    uint64
	unbounded bool
}

// Matches concrete domains exactly except outer nullability, leaving null policies to function binding.
// A mismatch returns (nil, false); successful matches publish no mutable storage to callers.
func BindTypePatterns(patterns []TypePattern, actual []Type) (*PatternBindings, bool) {
	if len(patterns) != len(actual) {
		return nil, false
	}
	bindings := &PatternBindings{literals: make(map[string]literalBinding)}
	for i, pattern := range patterns {
		assertions.NotNil(pattern)
		assertions.NotNil(actual[i])
		if !pattern.bindPattern(actual[i], bindings.literals) {
			return nil, false
		}
	}
	return bindings, true
}

// Substitutes parameters into a pattern while retaining its declared nullability.
// A parameter absent from these bindings is an error; concrete patterns share their original Type.
func (b *PatternBindings) Resolve(pattern TypePattern) (Type, error) {
	assertions.NotNilPointer(b)
	assertions.NotNil(pattern)
	return pattern.resolvePattern(b.literals)
}

// Fixed domains ignore only outer nullability during matching, never nested constraints.
func (p *ConcreteTypePattern) bindPattern(actual Type, _ map[string]literalBinding) bool {
	return p.typ.EqualsToIgnoringNullability(actual)
}

// Concrete patterns require no substitution or descriptor allocation.
func (p *ConcreteTypePattern) resolvePattern(_ map[string]literalBinding) (Type, error) {
	return p.typ, nil
}

// Extracts and unifies a byte bound without widening, including the explicit unbounded case.
func (p *VarCharTypePattern) bindPattern(actual Type, bindings map[string]literalBinding) bool {
	text, ok := actual.(*VarCharType)
	if !ok {
		return false
	}
	bound, bounded := text.MaxBytes()
	value := literalBinding{number: bound, unbounded: !bounded}
	if previous, exists := bindings[p.parameter]; exists {
		return previous == value
	}
	bindings[p.parameter] = value
	return true
}

// Reconstructs the bound VARCHAR domain with declaration-owned nullability.
func (p *VarCharTypePattern) resolvePattern(bindings map[string]literalBinding) (Type, error) {
	value, ok := bindings[p.parameter]
	if !ok {
		return nil, fmt.Errorf("unbound literal parameter %q", p.parameter)
	}
	if value.unbounded {
		return NewVarCharType(nil, p.nullability)
	}
	return NewVarCharType(&value.number, p.nullability)
}
