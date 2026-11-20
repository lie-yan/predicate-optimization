package interfaces_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// These implementations embed no library value or interface. The slice-backed
// implementation is deliberately non-comparable, even when boxed in Value.
type nativeInteger struct {
	typ    types.Type
	number int64
}

func (v nativeInteger) Type() types.Type { return v.typ }
func (nativeInteger) IsNull() bool       { return false }
func (v nativeInteger) Integer() int64   { return v.number }

func (v nativeInteger) EqualsTo(other valueinterfaces.Value) bool {
	if !v.typ.EqualsTo(other.Type()) || other.IsNull() {
		return false
	}
	if o, ok := other.(nativeInteger); ok {
		return v.number == o.number
	}
	return v.number == other.(valueinterfaces.IntegerValue).Integer()
}

func (v nativeInteger) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	typ, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	v.typ = typ
	return v, nil
}

type sliceInteger struct {
	typ     types.Type
	payload []int64
}

func (v sliceInteger) Type() types.Type { return v.typ }
func (sliceInteger) IsNull() bool       { return false }
func (v sliceInteger) Integer() int64   { return v.payload[0] }

func (v sliceInteger) EqualsTo(other valueinterfaces.Value) bool {
	if !v.typ.EqualsTo(other.Type()) || other.IsNull() {
		return false
	}
	if o, ok := other.(sliceInteger); ok {
		return v.payload[0] == o.payload[0]
	}
	return v.payload[0] == other.(valueinterfaces.IntegerValue).Integer()
}

func (v sliceInteger) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	typ, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	v.typ = typ // The immutable payload can be shared.
	return v, nil
}

type externalNull struct{ typ types.Type }

func (v externalNull) Type() types.Type { return v.typ }
func (externalNull) IsNull() bool       { return true }

func (v externalNull) EqualsTo(other valueinterfaces.Value) bool {
	return other.IsNull() && v.typ.EqualsTo(other.Type())
}

func (v externalNull) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	if n == foundation.NonNull {
		return nil, fmt.Errorf("cannot narrow a known null")
	}
	typ, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	return externalNull{typ: typ}, nil
}

var (
	_ valueinterfaces.IntegerValue = nativeInteger{}
	_ valueinterfaces.IntegerValue = sliceInteger{}
	_ valueinterfaces.Value        = externalNull{}
)

func TestIndependentValueImplementations(t *testing.T) {
	typ := fixtureValue(types.NewIntType(types.Int16, foundation.MayNull))
	one := fixtureValue(testbedvalues.NewInteger(typ, 1))
	null := fixtureValue(testbedvalues.NewNull(typ))
	variants := []valueinterfaces.Value{
		one, nativeInteger{typ: typ, number: 1}, sliceInteger{typ: typ, payload: []int64{1}},
	}
	for _, a := range variants {
		for _, b := range variants {
			if !a.EqualsTo(b) || !b.EqualsTo(a) {
				t.Fatalf("equal values differ: %T, %T", a, b)
			}
		}
		for _, different := range []valueinterfaces.Value{
			nativeInteger{typ: typ, number: 2},
			sliceInteger{typ: fixtureValue(types.NewIntType(types.Int32, foundation.MayNull)), payload: []int64{1}},
			externalNull{typ: typ},
			null,
		} {
			if a.EqualsTo(different) || different.EqualsTo(a) {
				t.Fatalf("different values compare equal: %T, %T", a, different)
			}
		}
	}
	customNull := externalNull{typ: typ}
	if !null.EqualsTo(customNull) || !customNull.EqualsTo(null) {
		t.Fatal("typed null interoperability failed")
	}
	otherNull := externalNull{typ: fixtureValue(types.NewBoolType(foundation.MayNull))}
	if customNull.EqualsTo(otherNull) || otherNull.EqualsTo(customNull) {
		t.Fatal("null type identity lost")
	}
	arrayType := fixtureValue(types.NewArrayType(typ, foundation.NonNull))
	mixed := fixtureValue(testbedvalues.NewArray(arrayType, []valueinterfaces.Value{variants[1], customNull, variants[2]}))
	bundled := fixtureValue(testbedvalues.NewArray(arrayType, []valueinterfaces.Value{one, null, one}))
	recordType := fixtureValue(types.NewStructType([]types.StructField{{Name: "items", Type: arrayType}}, foundation.NonNull))
	a := fixtureValue(testbedvalues.NewStruct(recordType, []valueinterfaces.Value{mixed}))
	b := fixtureValue(testbedvalues.NewStruct(recordType, []valueinterfaces.Value{bundled}))
	if !a.EqualsTo(b) || !b.EqualsTo(a) {
		t.Fatal("nested composites did not dispatch to external children")
	}
}

func TestIndependentValueNullability(t *testing.T) {
	typ := fixtureValue(types.NewIntType(types.Int16, foundation.MayNull))
	for _, original := range []valueinterfaces.Value{
		nativeInteger{typ: typ, number: 7}, sliceInteger{typ: typ, payload: []int64{7}},
	} {
		adapted := fixtureValue(original.WithNullability(foundation.NonNull))
		expected := fixtureValue(types.NewIntType(types.Int16, foundation.NonNull))
		if !adapted.Type().EqualsTo(expected) || adapted.IsNull() || adapted.(valueinterfaces.IntegerValue).Integer() != 7 {
			t.Fatal("adaptation changed payload or type properties")
		}
		if !original.Type().EqualsTo(typ) || original.EqualsTo(adapted) || adapted.EqualsTo(original) {
			t.Fatal("adaptation mutated source or ignored nullability")
		}
		roundtrip := fixtureValue(adapted.WithNullability(foundation.MayNull))
		if !roundtrip.EqualsTo(original) || !original.EqualsTo(roundtrip) {
			t.Fatal("nullability round trip lost typed content")
		}
	}
	null := externalNull{typ: typ}
	if !null.EqualsTo(fixtureValue(null.WithNullability(foundation.MayNull))) {
		t.Fatal("typed null adaptation lost content")
	}
	if result, err := null.WithNullability(foundation.NonNull); err == nil || result != nil {
		t.Fatal("known null narrowing accepted")
	}
	for _, v := range []valueinterfaces.Value{nativeInteger{typ: typ}, sliceInteger{typ: typ, payload: []int64{0}}, null} {
		if result, err := v.WithNullability(foundation.Nullability(99)); err == nil || result != nil {
			t.Fatal("invalid nullability accepted")
		}
	}
}
