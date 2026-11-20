package values

import (
	"errors"
	"math"

	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// FloatValue holds a non-null Float32 or Float64 payload in a float64 carrier.
// Float32 finite payloads must be exactly representable. NaNs are canonicalized;
// signed zero is preserved. Use NewFloat; the zero struct is invalid.
type FloatValue struct {
	nonNull
	typ      *types.FloatType
	floating float64
}

var _ valueinterfaces.FloatValue = (*FloatValue)(nil)

func (v *FloatValue) Type() types.Type { assertions.NotNilPointer(v); return v.typ }
func (v *FloatValue) Float() float64   { assertions.NotNilPointer(v); return v.floating }

// NewFloat performs no implicit rounding. Callers may explicitly round through
// float32 before passing a Float32 payload. NaN payload/sign distinctions are
// discarded; infinities and signed zeros are retained.
func NewFloat(t *types.FloatType, value float64) (*FloatValue, error) {
	assertions.NotNilPointer(t)
	if math.IsNaN(value) {
		value = math.NaN()
	} else if t.FloatWidth() == types.Float32 && float64(float32(value)) != value {
		return nil, errors.New("Float payload is not exactly representable at declared width")
	}
	return &FloatValue{typ: t, floating: value}, nil
}

func (v *FloatValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	assertions.NotNilPointer(v)
	t, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	copy := *v
	copy.typ = t.(*types.FloatType)
	return &copy, nil
}

// EqualsTo is typed structural equality: canonical NaNs are equal and signed
// zeros are distinct. SQL comparisons are supplied by FunctionContext.
func (v *FloatValue) EqualsTo(other valueinterfaces.Value) bool {
	assertions.NotNilPointer(v)
	assertions.NotNil(other)
	// Bundled peers can compare storage directly; foreign representations use
	// the public payload contract below.
	if o, ok := other.(*FloatValue); ok {
		return v.typ.EqualsTo(o.typ) && math.Float64bits(v.floating) == math.Float64bits(o.floating)
	}
	if !v.typ.EqualsTo(other.Type()) || other.IsNull() {
		return false
	}
	return math.Float64bits(v.floating) == math.Float64bits(other.(valueinterfaces.FloatValue).Float())
}
