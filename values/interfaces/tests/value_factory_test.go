package interfaces_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	"github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

func TestValueFactoryConstruction(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int8, foundation.MayNull))
	nonnull := fixtureValue(types.NewIntType(types.Int8, foundation.NonNull))
	boolean := fixtureValue(types.NewBoolType(foundation.NonNull))
	floating := fixtureValue(types.NewFloatType(types.Float32, foundation.NonNull))
	limit := uint64(2)
	text := fixtureValue(types.NewVarCharType(&limit, foundation.NonNull))
	array := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	record := fixtureValue(types.NewStructType([]types.StructField{{Name: "x", Type: integer}}, foundation.NonNull))
	one := nativeInteger{typ: integer, number: 1}
	var factory valueinterfaces.ValueFactory = testbedvalues.ValueFactory{}
	cases := []struct {
		name string
		call func(valueinterfaces.ValueFactory) (valueinterfaces.Value, error)
		want valueinterfaces.Value
	}{
		{"null", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) { return f.NewNull(integer) }, externalNull{typ: integer}},
		{"bool", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) { return f.NewBool(boolean, true) }, fixtureValue(testbedvalues.NewBool(boolean, true))},
		{"integer minimum", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) {
			return f.NewInteger(integer, -128)
		}, nativeInteger{typ: integer, number: -128}},
		{"integer maximum", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) { return f.NewInteger(integer, 127) }, nativeInteger{typ: integer, number: 127}},
		{"float exact", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) { return f.NewFloat(floating, 1.5) }, fixtureValue(testbedvalues.NewFloat(floating, 1.5))},
		{"float negative zero", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) {
			return f.NewFloat(floating, math.Copysign(0, -1))
		}, fixtureValue(testbedvalues.NewFloat(floating, math.Copysign(0, -1)))},
		{"float infinity", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) {
			return f.NewFloat(floating, math.Inf(1))
		}, fixtureValue(testbedvalues.NewFloat(floating, math.Inf(1)))},
		{"float NaN", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) {
			return f.NewFloat(floating, math.Float64frombits(0x7ff8000000001234))
		}, fixtureValue(testbedvalues.NewFloat(floating, math.NaN()))},
		{"varchar bytes", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) {
			return f.NewVarChar(text, "\xff\x00")
		}, fixtureValue(testbedvalues.NewVarChar(text, "\xff\x00"))},
		{"array", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) {
			return f.NewArray(array, []valueinterfaces.Value{one, externalNull{typ: integer}, one})
		}, fixtureValue(testbedvalues.NewArray(array, []valueinterfaces.Value{one, externalNull{typ: integer}, one}))},
		{"struct", func(f valueinterfaces.ValueFactory) (valueinterfaces.Value, error) {
			return f.NewStruct(record, []valueinterfaces.Value{one})
		}, fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{one}))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fixtureValue(tc.call(factory))
			if got == nil || !got.Type().EqualsTo(tc.want.Type()) || !got.EqualsTo(tc.want) || !tc.want.EqualsTo(got) {
				t.Fatal("factory changed typed content")
			}
			failure := errors.New("construction unavailable")
			recorder := &testutil.RecordingFactory{Delegate: factory, Failure: failure}
			if result, err := tc.call(recorder); result != nil || !errors.Is(err, failure) || len(recorder.Requests) != 1 {
				t.Fatal("injected failure lost its cause, returned a value, or missed recording")
			}
		})
	}
	invalid := []func() (valueinterfaces.Value, error){
		func() (valueinterfaces.Value, error) { return factory.NewNull(nonnull) },
		func() (valueinterfaces.Value, error) { return factory.NewInteger(integer, 128) },
		func() (valueinterfaces.Value, error) { return factory.NewInteger(integer, -129) },
		func() (valueinterfaces.Value, error) { return factory.NewFloat(floating, 0.1) },
		func() (valueinterfaces.Value, error) { return factory.NewVarChar(text, "abc") },
		func() (valueinterfaces.Value, error) {
			return factory.NewArray(array, []valueinterfaces.Value{nativeInteger{typ: nonnull, number: 1}})
		},
		func() (valueinterfaces.Value, error) { return factory.NewStruct(record, nil) },
		func() (valueinterfaces.Value, error) {
			return factory.NewStruct(record, []valueinterfaces.Value{nativeInteger{typ: nonnull, number: 1}})
		},
	}
	for i, call := range invalid {
		if result, err := call(); err == nil || result != nil {
			t.Fatalf("invalid case %d accepted or returned a typed nil interface", i)
		}
	}
}

func TestRecordingFactoryExternalValuesAndOwnership(t *testing.T) {
	delegate := testbedvalues.ValueFactory{}
	factory := &testutil.RecordingFactory{Delegate: delegate}
	factory.Integer = func(typ *types.IntType, n int64) (valueinterfaces.Value, error) {
		if _, err := delegate.NewInteger(typ, n); err != nil {
			return nil, err
		}
		return sliceInteger{typ: typ, payload: []int64{n}}, nil
	}
	factory.Null = func(typ types.Type) (valueinterfaces.Value, error) {
		if _, err := delegate.NewNull(typ); err != nil {
			return nil, err
		}
		return externalNull{typ: typ}, nil
	}
	integer := fixtureValue(types.NewIntType(types.Int16, foundation.MayNull))
	one := fixtureValue(factory.NewInteger(integer, 1))
	null := fixtureValue(factory.NewNull(integer))
	if _, ok := one.(sliceInteger); !ok {
		t.Fatal("factory did not return independent integer representation")
	}
	if _, ok := null.(externalNull); !ok {
		t.Fatal("factory did not return independent null representation")
	}
	if factory.Requests[0].Kind != "Integer" || factory.Requests[0].Payload != int64(1) || !factory.Requests[0].Type.EqualsTo(integer) || factory.Requests[1].Kind != "Null" {
		t.Fatal("constructor request not recorded accurately")
	}
	arrayType := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	recordType := fixtureValue(types.NewStructType([]types.StructField{{Name: "x", Type: integer}, {Name: "y", Type: integer}}, foundation.NonNull))
	input := []valueinterfaces.Value{one, null}
	array := fixtureValue(factory.NewArray(arrayType, input)).(valueinterfaces.ArrayValue)
	record := fixtureValue(factory.NewStruct(recordType, input)).(valueinterfaces.StructValue)
	input[0] = null
	array.Elements()[0] = null
	record.Fields()[0] = null
	if !array.Elements()[0].EqualsTo(one) || !record.Fields()[0].EqualsTo(one) {
		t.Fatal("composite storage aliases caller-owned slices")
	}
	for _, request := range factory.Requests[2:] {
		children := request.Payload.([]valueinterfaces.Value)
		if !children[0].EqualsTo(one) || !children[1].EqualsTo(null) {
			t.Fatal("request record aliases caller-owned slice")
		}
		children[0] = null
	}
	if !array.Elements()[0].EqualsTo(one) || !record.Fields()[0].EqualsTo(one) {
		t.Fatal("recorded slice aliases constructed value storage")
	}
}
