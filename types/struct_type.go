package types

import (
	"fmt"
	"strings"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// StructField identifies an ordered field. Names are compared exactly.
type StructField struct {
	Name string
	Type Type
}

// StructType is an immutable Struct descriptor. Its zero value represents a nullable empty struct.
type StructType struct {
	common
	fields []StructField
}

// NewStructType preserves field order and copies the field list. Empty structs
// are representable; integration layers may impose additional restrictions.
func NewStructType(fields []StructField, n foundation.Nullability) (*StructType, error) {
	c, err := newCommon(n)
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(fields))
	for i, f := range fields {
		if f.Name == "" {
			return nil, fmt.Errorf("empty field name at position %d", i)
		}
		if names[f.Name] {
			return nil, fmt.Errorf("duplicate field name: %q", f.Name)
		}
		assertions.NotNil(f.Type)
		names[f.Name] = true
	}
	return &StructType{common: c, fields: append([]StructField(nil), fields...)}, nil
}

// Fields returns a copy; immutable child descriptors may be shared.
func (t *StructType) Fields() []StructField { return append([]StructField(nil), t.fields...) }

// WithNullability copies a valid descriptor, sharing immutable child data.
// It asserts a non-nil receiver and checks the requested nullability,
// without revalidating the existing descriptor.
func (t *StructType) WithNullability(n foundation.Nullability) (Type, error) {
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
func (t *StructType) EqualsTo(other Type) bool { return equalsToImpl(t, other) }

func (t *StructType) equalsToIgnoringNullability(other Type) bool {
	o, ok := other.(*StructType)
	if !ok || len(t.fields) != len(o.fields) {
		return false
	}
	for i, f := range t.fields {
		if f.Name != o.fields[i].Name || !f.Type.EqualsTo(o.fields[i].Type) {
			return false
		}
	}
	return true
}

// String describes this valid, non-nil type, including nested nullability.
func (t *StructType) String() string {
	assertions.NotNilPointer(t)
	fields := make([]string, len(t.fields))
	for i, f := range t.fields {
		fields[i] = fmt.Sprintf("%q: %s", f.Name, f.Type.String())
	}
	return "Struct<" + strings.Join(fields, ", ") + ">" + t.Nullability().Suffix()
}

// IsOrderable reports the supported ordering capability for non-null values.
func (t *StructType) IsOrderable() bool {
	assertions.NotNilPointer(t)
	return false
}

// EqualsToIgnoringNullability compares structural identity except outer nullability.
func (t *StructType) EqualsToIgnoringNullability(other Type) bool {
	return equalsToIgnoringNullabilityImpl(t, other)
}
