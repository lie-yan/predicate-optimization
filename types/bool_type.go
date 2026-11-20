package types

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// BoolType is an immutable Bool descriptor. Its zero value represents a nullable Boolean.
type BoolType struct{ common }

func NewBoolType(n foundation.Nullability) (*BoolType, error) {
	c, err := newCommon(n)
	if err != nil {
		return nil, err
	}
	return &BoolType{common: c}, nil
}

// WithNullability copies a valid descriptor, sharing immutable child data.
// It asserts a non-nil receiver and checks the requested nullability,
// without revalidating the existing descriptor.
func (t *BoolType) WithNullability(n foundation.Nullability) (Type, error) {
	if t == nil {
		panic("types: expected non-nil Type")
	}
	c, err := newCommon(n)
	if err != nil {
		return nil, err
	}
	copy := *t
	copy.common = c
	return &copy, nil
}

// EqualsTo compares complete structural identity, including nullability.
func (t *BoolType) EqualsTo(other Type) bool { return equalsToImpl(t, other) }

func (t *BoolType) equalsToIgnoringNullability(other Type) bool {
	o, ok := other.(*BoolType)
	return ok && o != nil
}

// String describes this valid, non-nil type, including nested nullability.
func (t *BoolType) String() string {
	assertions.NotNilPointer(t)
	return "Bool" + t.Nullability().Suffix()
}

// IsOrderable reports the supported ordering capability for non-null values.
func (t *BoolType) IsOrderable() bool {
	assertions.NotNilPointer(t)
	return false
}

// EqualsToIgnoringNullability compares structural identity except outer nullability.
func (t *BoolType) EqualsToIgnoringNullability(other Type) bool {
	return equalsToIgnoringNullabilityImpl(t, other)
}
