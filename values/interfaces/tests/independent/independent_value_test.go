package interfaces_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// This fixture's package imports no parent values package, helper or testbed.
// Its representation and comparison belong entirely to the application.
type integer struct {
	typ     types.Type
	payload []int64
}

func (v integer) Type() types.Type { return v.typ }
func (integer) IsNull() bool       { return false }
func (v integer) Integer() int64   { return v.payload[0] }
func (v integer) EqualsTo(other valueinterfaces.Value) bool {
	return v.typ.EqualsTo(other.Type()) && !other.IsNull() && v.Integer() == other.(valueinterfaces.IntegerValue).Integer()
}

func (v integer) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	typ, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	v.typ = typ
	return v, nil
}

var _ valueinterfaces.IntegerValue = integer{}

func TestValueWithoutBundledDependencies(t *testing.T) {
	typ, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	one := integer{typ: typ, payload: []int64{1}}
	same := integer{typ: typ, payload: []int64{1}}
	other := integer{typ: typ, payload: []int64{2}}
	if !one.EqualsTo(same) || !same.EqualsTo(one) || one.EqualsTo(other) {
		t.Fatal("independent equality failed")
	}
	nullable, err := one.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	if nullable.(valueinterfaces.IntegerValue).Integer() != 1 || nullable.EqualsTo(one) || one.Type().Nullability() != foundation.NonNull {
		t.Fatal("adaptation lost payload or type identity")
	}
	restored, err := nullable.WithNullability(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.EqualsTo(one) || !one.EqualsTo(restored) {
		t.Fatal("roundtrip changed content")
	}
}
