package values

import (
	"fmt"

	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// NullValue is a known null with its nullable semantic type retained.
type NullValue struct{ typ types.Type }

var _ valueinterfaces.Value = (*NullValue)(nil)

func (*NullValue) IsNull() bool { return true }

func (v *NullValue) Type() types.Type { assertions.NotNilPointer(v); return v.typ }

// NewNull constructs a known null, including a null composite with no children.
func NewNull(t types.Type) (*NullValue, error) {
	assertions.NotNil(t)
	if t.Nullability() != foundation.MayNull {
		return nil, fmt.Errorf("null requires a nullable type")
	}
	return &NullValue{typ: t}, nil
}

// WithNullability copies the value with new outer nullability.
// The receiver must be valid and non-nil; immutable payloads are shared.
func (v *NullValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	assertions.NotNilPointer(v)
	if n == foundation.NonNull {
		return nil, fmt.Errorf("cannot adapt null to a non-null type")
	}
	t, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	copy := *v
	copy.typ = t
	return &copy, nil
}

// EqualsTo compares complete typed structural content, not SQL equality.
// Inputs must be valid, immutable and non-nil.
func (v *NullValue) EqualsTo(other valueinterfaces.Value) bool {
	assertions.NotNilPointer(v)
	assertions.NotNil(other)
	return other.IsNull() && v.typ.EqualsTo(other.Type())
}
