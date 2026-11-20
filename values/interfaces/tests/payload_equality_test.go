package interfaces_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// The embedded value supplies Type/nullability adaptation, while this wrapper
// owns equality for its deliberately different exposed payload.
// The slice makes the wrapper non-comparable; the second accessor ensures that
// semantic Type, rather than accessor matching order, selects the domain.
type integerEqualityView struct {
	valueinterfaces.Value
	payload []int64
}

func (v integerEqualityView) Integer() int64 { return v.payload[0] }

func (integerEqualityView) Bool() bool { panic("integer dispatched as Boolean") }

func (v integerEqualityView) EqualsTo(other valueinterfaces.Value) bool {
	return v.Type().EqualsTo(other.Type()) && !other.IsNull() && v.Integer() == other.(valueinterfaces.IntegerValue).Integer()
}

func (v integerEqualityView) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	base, err := v.Value.WithNullability(n)
	if err != nil {
		return nil, err
	}
	return integerEqualityView{Value: base, payload: v.payload}, nil
}

func TestEqualityUsesPublicPayloadViews(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	one := fixtureValue(testbedvalues.NewInteger(integer, 1))
	two := fixtureValue(testbedvalues.NewInteger(integer, 2))
	view := integerEqualityView{Value: one, payload: []int64{2}}
	for _, pair := range [][2]valueinterfaces.Value{{view, view}, {view, two}, {two, view}} {
		if !pair[0].EqualsTo(pair[1]) {
			t.Fatal("equality did not use semantic Type and public payload")
		}
	}
	if view.EqualsTo(one) || one.EqualsTo(view) {
		t.Fatal("equality used embedded storage instead of the exposed payload")
	}
	array := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	a := fixtureValue(testbedvalues.NewArray(array, []valueinterfaces.Value{view, view}))
	b := fixtureValue(testbedvalues.NewArray(array, []valueinterfaces.Value{two, two}))
	if !a.EqualsTo(b) || !b.EqualsTo(a) {
		t.Fatal("composite children bypassed public payload equality")
	}
	if view.Integer() != 2 || one.Integer() != 1 {
		t.Fatal("comparison mutated input payloads")
	}
}
