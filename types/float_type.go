package types

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// FloatWidth is the bit width of a floating-point type. Zero is invalid.
type FloatWidth uint8

const (
	Float32 FloatWidth = 32
	Float64 FloatWidth = 64
)

func (w FloatWidth) isValid() bool {
	switch w {
	case Float32, Float64:
		return true
	default:
		return false
	}
}

// FloatType is an immutable Float descriptor. Its zero value is invalid.
type FloatType struct {
	common
	width FloatWidth
}

func NewFloatType(width FloatWidth, n foundation.Nullability) (*FloatType, error) {
	if !width.isValid() {
		return nil, fmt.Errorf("invalid float width: %d", width)
	}
	c, err := newCommon(n)
	if err != nil {
		return nil, err
	}
	return &FloatType{common: c, width: width}, nil
}

func (t *FloatType) FloatWidth() FloatWidth { return t.width }

// WithNullability copies a valid descriptor, sharing immutable child data.
// It asserts a non-nil receiver and checks the requested nullability,
// without revalidating the existing descriptor.
func (t *FloatType) WithNullability(n foundation.Nullability) (Type, error) {
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
func (t *FloatType) EqualsTo(other Type) bool { return equalsToImpl(t, other) }

func (t *FloatType) equalsToIgnoringNullability(other Type) bool {
	o, ok := other.(*FloatType)
	return ok && t.width == o.width
}

// String describes this valid, non-nil type, including nested nullability.
func (t *FloatType) String() string {
	assertions.NotNilPointer(t)
	return fmt.Sprintf("Float%d%s", t.width, t.Nullability().Suffix())
}

// IsOrderable reports the supported ordering capability for non-null values.
func (t *FloatType) IsOrderable() bool {
	assertions.NotNilPointer(t)
	return true
}

// EqualsToIgnoringNullability compares structural identity except outer nullability.
func (t *FloatType) EqualsToIgnoringNullability(other Type) bool {
	return equalsToIgnoringNullabilityImpl(t, other)
}
