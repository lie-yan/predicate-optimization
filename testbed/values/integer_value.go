package values

import (
	"errors"

	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// IntegerValue holds a non-null Integer value.
type IntegerValue struct {
	nonNull
	typ     *types.IntType
	integer int64
}

var _ valueinterfaces.IntegerValue = (*IntegerValue)(nil)

func (v *IntegerValue) Type() types.Type { assertions.NotNilPointer(v); return v.typ }

func (v *IntegerValue) Integer() int64 { assertions.NotNilPointer(v); return v.integer }

func integerFits(width types.IntWidth, value int64) bool {
	if width == types.Int64 {
		return true
	}
	limit := int64(1) << (width - 1)
	return value >= -limit && value < limit
}

// NewInteger checks the new payload against a valid, non-nil Type without implicit conversions.
func NewInteger(t *types.IntType, value int64) (*IntegerValue, error) {
	assertions.NotNilPointer(t)
	if !integerFits(t.IntegerWidth(), value) {
		return nil, errors.New("Integer payload exceeds declared width")
	}
	return &IntegerValue{typ: t, integer: value}, nil
}

// WithNullability copies the value with new outer nullability.
// The receiver must be valid and non-nil; immutable payloads are shared.
func (v *IntegerValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	assertions.NotNilPointer(v)
	t, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	copy := *v
	copy.typ = t.(*types.IntType)
	return &copy, nil
}

// EqualsTo compares complete typed structural content, not SQL equality.
// Inputs must be valid, immutable and non-nil.
func (v *IntegerValue) EqualsTo(other valueinterfaces.Value) bool {
	assertions.NotNilPointer(v)
	assertions.NotNil(other)
	// Bundled peers can compare storage directly; foreign representations use
	// the public payload contract below.
	if o, ok := other.(*IntegerValue); ok {
		return v.typ.EqualsTo(o.typ) && v.integer == o.integer
	}
	if !v.typ.EqualsTo(other.Type()) || other.IsNull() {
		return false
	}
	return v.integer == other.(valueinterfaces.IntegerValue).Integer()
}
