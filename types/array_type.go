package types

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// ArrayType is an immutable Array descriptor. Its zero value is invalid.
type ArrayType struct {
	common
	element Type
}

func NewArrayType(element Type, n foundation.Nullability) (*ArrayType, error) {
	assertions.NotNil(element)
	c, err := newCommon(n)
	if err != nil {
		return nil, err
	}
	return &ArrayType{common: c, element: element}, nil
}

func (t *ArrayType) Element() Type { return t.element }

// WithNullability copies a valid descriptor, sharing immutable child data.
// It asserts a non-nil receiver and checks the requested nullability,
// without revalidating the existing descriptor.
func (t *ArrayType) WithNullability(n foundation.Nullability) (Type, error) {
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
func (t *ArrayType) EqualsTo(other Type) bool { return equalsToImpl(t, other) }

func (t *ArrayType) equalsToIgnoringNullability(other Type) bool {
	o, ok := other.(*ArrayType)
	return ok && t.element.EqualsTo(o.element)
}

// String describes this valid, non-nil type, including nested nullability.
func (t *ArrayType) String() string {
	assertions.NotNilPointer(t)
	return "Array<" + t.element.String() + ">" + t.Nullability().Suffix()
}

// IsOrderable reports the supported ordering capability for non-null values.
func (t *ArrayType) IsOrderable() bool {
	assertions.NotNilPointer(t)
	return false
}

// EqualsToIgnoringNullability compares structural identity except outer nullability.
func (t *ArrayType) EqualsToIgnoringNullability(other Type) bool {
	return equalsToIgnoringNullabilityImpl(t, other)
}
