// Package types defines immutable semantic types for predicate optimization.
// foundation.Nullability is part of type identity. Descriptors contain no evaluation or
// storage facilities, and constructing a type does not imply operator support.
package types

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// Type describes a semantic value domain, including nullability. Implementations
// are package-owned; additional concrete variants can be added as needed.
// Use constructors and Type.EqualsTo rather than pointer identity. Existing
// descriptors must be valid, immutable and non-nil; they are not revalidated.
// Do not overwrite a descriptor after sharing it.
type Type interface {
	// String describes a valid, non-nil type for display, not serialization.
	String() string
	// EqualsTo requires valid, non-nil immutable descriptors.
	// Nil operands trigger a panic assertion.
	EqualsTo(other Type) bool
	// EqualsToIgnoringNullability ignores only outer nullability. Nested element
	// and field nullability remains significant. Inputs must be valid and non-nil.
	EqualsToIgnoringNullability(other Type) bool
	// equalsToIgnoringNullability compares variant properties, excluding only
	// outer nullability. Composite children retain full equality checks.
	equalsToIgnoringNullability(other Type) bool
	// IsOrderable reports ordering capability for non-null values, independent of
	// outer nullability. It requires a valid, non-nil receiver; it does not
	// guarantee that a function provider implements a comparison overload.
	IsOrderable() bool
	Nullability() foundation.Nullability
	// WithNullability returns a related descriptor without converting a value
	// or proving that an expression can be narrowed. The receiver must be valid
	// and remains unchanged. Nil receivers panic; invalid requested nullability returns an error.
	WithNullability(foundation.Nullability) (Type, error)
	// isType marks package-owned implementations and prevents external types
	// from satisfying Type merely by declaring the public methods. Embedding
	// Type can still satisfy the interface; implementations must honor its contracts.
	isType()
}

// common holds properties shared by every variant.
type common struct {
	nullability foundation.Nullability
}

func (c common) Nullability() foundation.Nullability {
	return c.nullability
}

func (common) isType() {}

func newCommon(n foundation.Nullability) (common, error) {
	if n != foundation.MayNull && n != foundation.NonNull {
		return common{}, fmt.Errorf("invalid nullability: %d", n)
	}
	return common{nullability: n}, nil
}

// equalsToImpl compares structural identity, including nullability at every level.
// Operands must be valid, non-nil immutable descriptors. Nil interfaces and
// typed nil pointers trigger a panic assertion; other invariants are not revalidated.
func equalsToImpl(a, b Type) bool {
	assertions.NotNil(a)
	assertions.NotNil(b)
	if a == b {
		return true
	}
	if a.Nullability() != b.Nullability() {
		return false
	}
	return a.equalsToIgnoringNullability(b)
}

// equalsToIgnoringNullabilityImpl dispatches variant comparison without the outer
// nullability check. Composite implementations use full EqualsTo for children.
func equalsToIgnoringNullabilityImpl(a, b Type) bool {
	assertions.NotNil(a)
	assertions.NotNil(b)
	if a == b {
		return true
	}
	return a.equalsToIgnoringNullability(b)
}
