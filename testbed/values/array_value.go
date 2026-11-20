package values

import (
	"fmt"

	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// ArrayValue holds a non-null Array value.
type ArrayValue struct {
	nonNull
	typ      *types.ArrayType
	elements []valueinterfaces.Value
}

var _ valueinterfaces.ArrayValue = (*ArrayValue)(nil)

func (v *ArrayValue) Type() types.Type { assertions.NotNilPointer(v); return v.typ }

// Elements returns a copy; immutable child values may be shared.
func (v *ArrayValue) Elements() []valueinterfaces.Value {
	assertions.NotNilPointer(v)
	return append([]valueinterfaces.Value(nil), v.elements...)
}

// ElementCount reads the array length without copying its children.
func (v *ArrayValue) ElementCount() int {
	assertions.NotNilPointer(v)
	return len(v.elements)
}

// ElementAt shares one immutable child. Slice indexing checks the bounds.
func (v *ArrayValue) ElementAt(index int) valueinterfaces.Value {
	assertions.NotNilPointer(v)
	return v.elements[index]
}

// NewArray checks the new payload against a valid, non-nil Type without implicit conversions.
func NewArray(t *types.ArrayType, value []valueinterfaces.Value) (*ArrayValue, error) {
	assertions.NotNilPointer(t)
	for i, child := range value {
		assertions.NotNil(child)
		if !child.Type().EqualsTo(t.Element()) {
			return nil, fmt.Errorf("array element %d type mismatch", i)
		}
	}
	return &ArrayValue{typ: t, elements: append([]valueinterfaces.Value(nil), value...)}, nil
}

// WithNullability copies the value with new outer nullability.
// The receiver must be valid and non-nil; immutable payloads are shared.
func (v *ArrayValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	assertions.NotNilPointer(v)
	t, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	copy := *v
	copy.typ = t.(*types.ArrayType)
	return &copy, nil
}

// EqualsTo compares complete typed structural content, not SQL equality.
// Inputs must be valid, immutable and non-nil.
func (v *ArrayValue) EqualsTo(other valueinterfaces.Value) bool {
	assertions.NotNilPointer(v)
	assertions.NotNil(other)
	if !v.typ.EqualsTo(other.Type()) || other.IsNull() {
		return false
	}
	var children []valueinterfaces.Value
	if o, ok := other.(*ArrayValue); ok {
		if v == o {
			return true
		}
		children = o.elements
	} else {
		children = other.(valueinterfaces.ArrayValue).Elements()
	}
	if len(v.elements) != len(children) {
		return false
	}
	for i, child := range v.elements {
		if !child.EqualsTo(children[i]) {
			return false
		}
	}
	return true
}
