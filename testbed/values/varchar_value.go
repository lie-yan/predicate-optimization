package values

import (
	"errors"

	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// VarCharValue holds a non-null VarChar value.
type VarCharValue struct {
	nonNull
	typ  *types.VarCharType
	text string
}

var _ valueinterfaces.VarCharValue = (*VarCharValue)(nil)

func (v *VarCharValue) Type() types.Type { assertions.NotNilPointer(v); return v.typ }

func (v *VarCharValue) VarChar() string { assertions.NotNilPointer(v); return v.text }

// NewVarChar checks the new payload against a valid, non-nil Type without implicit conversions.
func NewVarChar(t *types.VarCharType, value string) (*VarCharValue, error) {
	assertions.NotNilPointer(t)
	if limit, bounded := t.MaxBytes(); bounded && uint64(len(value)) > limit {
		return nil, errors.New("VarChar payload exceeds byte bound")
	}
	return &VarCharValue{typ: t, text: value}, nil
}

// WithNullability copies the value with new outer nullability.
// The receiver must be valid and non-nil; immutable payloads are shared.
func (v *VarCharValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	assertions.NotNilPointer(v)
	t, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	copy := *v
	copy.typ = t.(*types.VarCharType)
	return &copy, nil
}

// EqualsTo compares complete typed structural content, not SQL equality.
// Inputs must be valid, immutable and non-nil.
func (v *VarCharValue) EqualsTo(other valueinterfaces.Value) bool {
	assertions.NotNilPointer(v)
	assertions.NotNil(other)
	// Bundled peers can compare storage directly; foreign representations use
	// the public payload contract below.
	if o, ok := other.(*VarCharValue); ok {
		return v.typ.EqualsTo(o.typ) && v.text == o.text
	}
	if !v.typ.EqualsTo(other.Type()) || other.IsNull() {
		return false
	}
	return v.text == other.(valueinterfaces.VarCharValue).VarChar()
}
