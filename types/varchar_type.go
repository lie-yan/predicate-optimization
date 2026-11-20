package types

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// VarCharType is an immutable VarChar descriptor. Its zero value represents a nullable unbounded VARCHAR.
type VarCharType struct {
	common
	maxBytes *uint64
}

// NewVarCharType accepts an optional inclusive byte limit, matching Milvus.
// Nil means unbounded; zero permits only empty non-null strings. The limit is copied.
func NewVarCharType(maxBytes *uint64, n foundation.Nullability) (*VarCharType, error) {
	c, err := newCommon(n)
	if err != nil {
		return nil, err
	}
	t := &VarCharType{common: c}
	if maxBytes != nil {
		b := *maxBytes
		t.maxBytes = &b
	}
	return t, nil
}

// MaxBytes returns the byte limit and whether it is present.
func (t *VarCharType) MaxBytes() (uint64, bool) {
	if t.maxBytes == nil {
		return 0, false
	}
	return *t.maxBytes, true
}

// WithNullability copies a valid descriptor, sharing immutable child data.
// It asserts a non-nil receiver and checks the requested nullability,
// without revalidating the existing descriptor.
func (t *VarCharType) WithNullability(n foundation.Nullability) (Type, error) {
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
func (t *VarCharType) EqualsTo(other Type) bool { return equalsToImpl(t, other) }

func (t *VarCharType) equalsToIgnoringNullability(other Type) bool {
	o, ok := other.(*VarCharType)
	if !ok || (t.maxBytes == nil) != (o.maxBytes == nil) {
		return false
	}
	return t.maxBytes == nil || *t.maxBytes == *o.maxBytes
}

// String describes this valid, non-nil type, including nested nullability.
func (t *VarCharType) String() string {
	assertions.NotNilPointer(t)
	if t.maxBytes == nil {
		return "VarChar" + t.Nullability().Suffix()
	}
	return fmt.Sprintf("VarChar(%d bytes)%s", *t.maxBytes, t.Nullability().Suffix())
}

// IsOrderable reports the supported ordering capability for non-null values.
func (t *VarCharType) IsOrderable() bool {
	assertions.NotNilPointer(t)
	return true
}

// EqualsToIgnoringNullability compares structural identity except outer nullability.
func (t *VarCharType) EqualsToIgnoringNullability(other Type) bool {
	return equalsToIgnoringNullabilityImpl(t, other)
}
