package customvalues_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/examples/customvalues"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestFactoryContracts(t *testing.T) {
	factory := customvalues.Factory{}
	integer := must(types.NewIntType(types.Int8, foundation.MayNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	floating := must(types.NewFloatType(types.Float32, foundation.NonNull))
	limit := uint64(2)
	text := must(types.NewVarCharType(&limit, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	record := must(types.NewStructType([]types.StructField{{Name: "x", Type: array}}, foundation.NonNull))
	one := must(factory.NewInteger(integer, 1))
	null := must(factory.NewNull(integer))
	input := []valueinterfaces.Value{one, null}
	a := must(factory.NewArray(array, input))
	r := must(factory.NewStruct(record, []valueinterfaces.Value{a}))
	input[0] = null
	a.(valueinterfaces.ArrayValue).Elements()[0] = null
	r.(valueinterfaces.StructValue).Fields()[0] = null
	if !a.(valueinterfaces.ArrayValue).Elements()[0].EqualsTo(one) || !r.(valueinterfaces.StructValue).Fields()[0].EqualsTo(a) {
		t.Fatal("composite storage was mutable")
	}
	view := a.(valueinterfaces.ArrayValue)
	if view.ElementCount() != 2 || !view.ElementAt(0).EqualsTo(one) || !view.ElementAt(1).EqualsTo(null) {
		t.Fatal("indexed access differs from owned array content")
	}
	pairs := [][2]valueinterfaces.Value{
		{one, must(testbedvalues.NewInteger(integer, 1))},
		{null, must(testbedvalues.NewNull(integer))},
		{must(factory.NewBool(boolean, true)), must(testbedvalues.NewBool(boolean, true))},
		{must(factory.NewFloat(floating, math.NaN())), must(testbedvalues.NewFloat(floating, math.NaN()))},
		{must(factory.NewFloat(floating, math.Copysign(0, -1))), must(testbedvalues.NewFloat(floating, math.Copysign(0, -1)))},
		{must(factory.NewVarChar(text, "a\x00")), must(testbedvalues.NewVarChar(text, "a\x00"))},
		{a, must(testbedvalues.NewArray(array, []valueinterfaces.Value{one, null}))},
		{r, must(testbedvalues.NewStruct(record, []valueinterfaces.Value{a}))},
	}
	for _, pair := range pairs {
		if !pair[0].EqualsTo(pair[1]) || !pair[1].EqualsTo(pair[0]) {
			t.Fatal("cross-representation equality failed")
		}
	}
	for _, construct := range []func() (valueinterfaces.Value, error){
		func() (valueinterfaces.Value, error) { return factory.NewInteger(integer, 128) },
		func() (valueinterfaces.Value, error) { return factory.NewFloat(floating, 0.1) },
		func() (valueinterfaces.Value, error) { return factory.NewVarChar(text, "abc") },
		func() (valueinterfaces.Value, error) { return factory.NewNull(boolean) },
		func() (valueinterfaces.Value, error) { return factory.NewArray(array, []valueinterfaces.Value{a}) },
		func() (valueinterfaces.Value, error) { return factory.NewStruct(record, nil) },
		func() (valueinterfaces.Value, error) { return factory.NewStruct(record, []valueinterfaces.Value{one}) },
		func() (valueinterfaces.Value, error) { return null.WithNullability(foundation.NonNull) },
	} {
		if v, err := construct(); err == nil || v != nil {
			t.Fatal("invalid construction/adaptation accepted")
		}
	}
	adapted := must(r.WithNullability(foundation.MayNull))
	if adapted.EqualsTo(r) || r.Type().Nullability() != foundation.NonNull || !must(adapted.WithNullability(foundation.NonNull)).EqualsTo(r) {
		t.Fatal("nullability adaptation changed content")
	}
}
