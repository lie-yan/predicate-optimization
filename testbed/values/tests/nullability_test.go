package values_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// fixtureValue unwraps setup constructors; behavior under test uses explicit checks.
func TestValueWithNullabilityPreservesPayloads(t *testing.T) {
	boolean := fixtureValue(types.NewBoolType(foundation.NonNull))
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	limit := uint64(8)
	text := fixtureValue(types.NewVarCharType(&limit, foundation.NonNull))
	for _, source := range []valueinterfaces.Value{
		fixtureValue(testbedvalues.NewBool(boolean, true)),
		fixtureValue(testbedvalues.NewInteger(integer, -42)),
		fixtureValue(testbedvalues.NewVarChar(text, "hello")),
	} {
		t.Run(source.Type().String(), func(t *testing.T) {
			target := fixtureValue(source.Type().WithNullability(foundation.MayNull))
			adapted, err := source.WithNullability(foundation.MayNull)
			if err != nil {
				t.Fatal(err)
			}
			if !adapted.Type().EqualsTo(target) || adapted.IsNull() {
				t.Fatal("adapted value must match the target type and preserve non-nullness")
			}
			roundTrip, err := adapted.WithNullability(foundation.NonNull)
			if err != nil {
				t.Fatal(err)
			}
			switch original := source.(type) {
			case *testbedvalues.BoolValue:
				if roundTrip.(*testbedvalues.BoolValue).Bool() != original.Bool() {
					t.Fatal("Boolean payload changed")
				}
			case *testbedvalues.IntegerValue:
				if roundTrip.(*testbedvalues.IntegerValue).Integer() != original.Integer() {
					t.Fatal("integer payload changed")
				}
			case *testbedvalues.VarCharValue:
				if roundTrip.(*testbedvalues.VarCharValue).VarChar() != original.VarChar() {
					t.Fatal("string payload changed")
				}
			}
			if source.Type().Nullability() != foundation.NonNull || adapted.Type().Nullability() != foundation.MayNull {
				t.Fatal("adaptation mutated its source")
			}
		})
	}
}

func TestValueWithNullabilitySharesCompositeChildren(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.MayNull))
	child := fixtureValue(testbedvalues.NewNull(integer))
	arrayType := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	array := fixtureValue(testbedvalues.NewArray(arrayType, []valueinterfaces.Value{child}))
	structType := fixtureValue(types.NewStructType([]types.StructField{{Name: "items", Type: arrayType}}, foundation.NonNull))
	structure := fixtureValue(testbedvalues.NewStruct(structType, []valueinterfaces.Value{array}))
	for _, source := range []valueinterfaces.Value{array, structure, fixtureValue(testbedvalues.NewArray(arrayType, nil))} {
		target := fixtureValue(source.Type().WithNullability(foundation.MayNull))
		adapted, err := source.WithNullability(foundation.MayNull)
		if err != nil || !adapted.Type().EqualsTo(target) {
			t.Fatalf("composite adaptation failed: %v", err)
		}
		switch original := source.(type) {
		case *testbedvalues.ArrayValue:
			got := adapted.(*testbedvalues.ArrayValue).Elements()
			if len(got) != len(original.Elements()) {
				t.Fatal("array length changed")
			}
			if len(got) > 0 {
				if got[0] != child {
					t.Fatal("array child was not shared")
				}
				got[0] = nil
				if adapted.(*testbedvalues.ArrayValue).Elements()[0] != child || original.Elements()[0] != child {
					t.Fatal("returned slice mutation escaped")
				}
			}
		case *testbedvalues.StructValue:
			got := adapted.(*testbedvalues.StructValue).Fields()
			if got[0] != array {
				t.Fatal("struct child was not shared")
			}
			got[0] = nil
			if adapted.(*testbedvalues.StructValue).Fields()[0] != array || original.Fields()[0] != array {
				t.Fatal("returned slice mutation escaped")
			}
		}
		if source.Type().Nullability() != foundation.NonNull {
			t.Fatal("source type mutated")
		}
	}
}

func TestValueWithNullabilityNullsAndInvalidTargets(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.MayNull))
	array := fixtureValue(types.NewArrayType(integer, foundation.MayNull))
	structure := fixtureValue(types.NewStructType([]types.StructField{{Name: "x", Type: integer}}, foundation.MayNull))
	for _, typ := range []types.Type{integer, array, structure} {
		source := fixtureValue(testbedvalues.NewNull(typ))
		result, err := source.WithNullability(foundation.MayNull)
		if err != nil || !result.IsNull() || !result.Type().EqualsTo(typ) {
			t.Fatalf("nullable null adaptation failed: %v", err)
		}
		if result, err := source.WithNullability(foundation.NonNull); err == nil || result != nil {
			t.Fatal("known null adapted to NonNull")
		}
	}
	boolean := fixtureValue(types.NewBoolType(foundation.NonNull))
	text := fixtureValue(types.NewVarCharType(nil, foundation.NonNull))
	for _, source := range []valueinterfaces.Value{
		fixtureValue(testbedvalues.NewNull(integer)),
		fixtureValue(testbedvalues.NewBool(boolean, true)),
		fixtureValue(testbedvalues.NewInteger(integer, 1)),
		fixtureValue(testbedvalues.NewVarChar(text, "a")),
		fixtureValue(testbedvalues.NewArray(array, nil)),
		fixtureValue(testbedvalues.NewStruct(structure, []valueinterfaces.Value{fixtureValue(testbedvalues.NewNull(integer))})),
	} {
		if result, err := source.WithNullability(foundation.Nullability(255)); err == nil || result != nil {
			t.Fatal("invalid nullability accepted")
		}
	}
}

func TestValueWithNullabilityNilAssertions(t *testing.T) {
	var nilInterface valueinterfaces.Value
	var nilValue *testbedvalues.IntegerValue
	for name, call := range map[string]func(){
		"nil value":       func() { _, _ = nilInterface.WithNullability(foundation.MayNull) },
		"typed nil value": func() { _, _ = nilValue.WithNullability(foundation.MayNull) },
	} {
		t.Run(name, func(t *testing.T) {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Fatal("expected nil assertion")
				}
			}()
			call()
		})
	}
}
