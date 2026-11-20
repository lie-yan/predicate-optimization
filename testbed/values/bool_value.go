package values

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// BoolValue holds a non-null Bool value.
type BoolValue struct {
	nonNull
	typ     *types.BoolType
	boolean bool
}

var _ valueinterfaces.BoolValue = (*BoolValue)(nil)

func (v *BoolValue) Type() types.Type { assertions.NotNilPointer(v); return v.typ }

func (v *BoolValue) Bool() bool { assertions.NotNilPointer(v); return v.boolean }

// NewBool checks the new payload against a valid, non-nil Type without implicit conversions.
func NewBool(t *types.BoolType, value bool) (*BoolValue, error) {
	assertions.NotNilPointer(t)
	return &BoolValue{typ: t, boolean: value}, nil
}

// WithNullability copies the value with new outer nullability.
// The receiver must be valid and non-nil; immutable payloads are shared.
func (v *BoolValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	assertions.NotNilPointer(v)
	t, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	copy := *v
	copy.typ = t.(*types.BoolType)
	return &copy, nil
}

// EqualsTo compares complete typed structural content, not SQL equality.
// Inputs must be valid, immutable and non-nil.
func (v *BoolValue) EqualsTo(other valueinterfaces.Value) bool {
	assertions.NotNilPointer(v)
	assertions.NotNil(other)
	// Bundled peers can compare storage directly; foreign representations use
	// the public payload contract below.
	if o, ok := other.(*BoolValue); ok {
		return v.typ.EqualsTo(o.typ) && v.boolean == o.boolean
	}
	if !v.typ.EqualsTo(other.Type()) || other.IsNull() {
		return false
	}
	return v.boolean == other.(valueinterfaces.BoolValue).Bool()
}
