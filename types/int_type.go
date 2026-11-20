package types

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// IntWidth is the bit width of a signed integer type. Zero is invalid.
type IntWidth uint8

const (
	Int8  IntWidth = 8
	Int16 IntWidth = 16
	Int32 IntWidth = 32
	Int64 IntWidth = 64
)

func (w IntWidth) isValid() bool {
	switch w {
	case Int8, Int16, Int32, Int64:
		return true
	default:
		return false
	}
}

// IntType is an immutable Int descriptor. Its zero value is invalid.
type IntType struct {
	common
	width IntWidth
}

func NewIntType(width IntWidth, n foundation.Nullability) (*IntType, error) {
	if !width.isValid() {
		return nil, fmt.Errorf("invalid integer width: %d", width)
	}
	c, err := newCommon(n)
	if err != nil {
		return nil, err
	}
	return &IntType{common: c, width: width}, nil
}

func (t *IntType) IntegerWidth() IntWidth { return t.width }

// WithNullability copies a valid descriptor, sharing immutable child data.
// It asserts a non-nil receiver and checks the requested nullability,
// without revalidating the existing descriptor.
func (t *IntType) WithNullability(n foundation.Nullability) (Type, error) {
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
func (t *IntType) EqualsTo(other Type) bool { return equalsToImpl(t, other) }

func (t *IntType) equalsToIgnoringNullability(other Type) bool {
	o, ok := other.(*IntType)
	return ok && t.width == o.width
}

// String describes this valid, non-nil type, including nested nullability.
func (t *IntType) String() string {
	assertions.NotNilPointer(t)
	return fmt.Sprintf("Int%d%s", t.width, t.Nullability().Suffix())
}

// IsOrderable reports the supported ordering capability for non-null values.
func (t *IntType) IsOrderable() bool {
	assertions.NotNilPointer(t)
	return true
}

// EqualsToIgnoringNullability compares structural identity except outer nullability.
func (t *IntType) EqualsToIgnoringNullability(other Type) bool {
	return equalsToIgnoringNullabilityImpl(t, other)
}
