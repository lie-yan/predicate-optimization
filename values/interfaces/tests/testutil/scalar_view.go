package testutil

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ScalarView is a non-comparable caller-owned scalar wrapper. All payload
// accessors exist, but only the one selected by Type is valid. Tests use it to
// detect concrete assertions and dispatch based on accessor matching order.
type ScalarView struct {
	source        valueinterfaces.Value
	nonComparable []byte
}

// WrapScalar shares a valid immutable scalar value or typed null.
func WrapScalar(source valueinterfaces.Value) ScalarView { return ScalarView{source: source} }

func (v ScalarView) Type() types.Type { return v.source.Type() }
func (v ScalarView) IsNull() bool     { return v.source.IsNull() }
func (v ScalarView) EqualsTo(other valueinterfaces.Value) bool {
	return v.source.EqualsTo(other)
}

func (v ScalarView) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	source, err := v.source.WithNullability(n)
	if err != nil {
		return nil, err
	}
	return WrapScalar(source), nil
}

func (v ScalarView) Bool() bool {
	return v.source.(valueinterfaces.BoolValue).Bool()
}

func (v ScalarView) Integer() int64 {
	return v.source.(valueinterfaces.IntegerValue).Integer()
}

func (v ScalarView) Float() float64 {
	return v.source.(valueinterfaces.FloatValue).Float()
}

func (v ScalarView) VarChar() string {
	return v.source.(valueinterfaces.VarCharValue).VarChar()
}
