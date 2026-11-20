package values

import (
	"fmt"

	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// StructValue holds a non-null Struct value.
type StructValue struct {
	nonNull
	typ    *types.StructType
	fields []valueinterfaces.Value
}

var _ valueinterfaces.StructValue = (*StructValue)(nil)

func (v *StructValue) Type() types.Type { assertions.NotNilPointer(v); return v.typ }

// Fields returns a copy; immutable child values may be shared.
func (v *StructValue) Fields() []valueinterfaces.Value {
	assertions.NotNilPointer(v)
	return append([]valueinterfaces.Value(nil), v.fields...)
}

// NewStruct checks the new payload against a valid, non-nil Type without implicit conversions.
func NewStruct(t *types.StructType, value []valueinterfaces.Value) (*StructValue, error) {
	assertions.NotNilPointer(t)
	declared := t.Fields()
	if len(value) != len(declared) {
		return nil, fmt.Errorf("struct field count mismatch")
	}
	for i, child := range value {
		assertions.NotNil(child)
		if !child.Type().EqualsTo(declared[i].Type) {
			return nil, fmt.Errorf("struct field %d type mismatch", i)
		}
	}
	return &StructValue{typ: t, fields: append([]valueinterfaces.Value(nil), value...)}, nil
}

// WithNullability copies the value with new outer nullability.
// The receiver must be valid and non-nil; immutable payloads are shared.
func (v *StructValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	assertions.NotNilPointer(v)
	t, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	copy := *v
	copy.typ = t.(*types.StructType)
	return &copy, nil
}

// EqualsTo compares complete typed structural content, not SQL equality.
// Inputs must be valid, immutable and non-nil.
func (v *StructValue) EqualsTo(other valueinterfaces.Value) bool {
	assertions.NotNilPointer(v)
	assertions.NotNil(other)
	if !v.typ.EqualsTo(other.Type()) || other.IsNull() {
		return false
	}
	var children []valueinterfaces.Value
	if o, ok := other.(*StructValue); ok {
		if v == o {
			return true
		}
		children = o.fields
	} else {
		children = other.(valueinterfaces.StructValue).Fields()
	}
	if len(v.fields) != len(children) {
		return false
	}
	for i, child := range v.fields {
		if !child.EqualsTo(children[i]) {
			return false
		}
	}
	return true
}
