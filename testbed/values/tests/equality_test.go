package values_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestValueStructuralEquality(t *testing.T) {
	integer := fixtureValue(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := fixtureValue(types.NewIntType(types.Int64, foundation.MayNull))
	number := func(n int64) valueinterfaces.Value { return fixtureValue(testbedvalues.NewInteger(integer, n)) }
	boolean := fixtureValue(types.NewBoolType(foundation.MayNull))
	text := fixtureValue(types.NewVarCharType(nil, foundation.NonNull))
	limit := uint64(8)
	bounded := fixtureValue(types.NewVarCharType(&limit, foundation.NonNull))
	array := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
	list := func(ns ...int64) valueinterfaces.Value {
		children := make([]valueinterfaces.Value, len(ns))
		for i, n := range ns {
			children[i] = number(n)
		}
		return fixtureValue(testbedvalues.NewArray(array, children))
	}
	record := fixtureValue(types.NewStructType([]types.StructField{{Name: "x", Type: integer}, {Name: "y", Type: integer}}, foundation.NonNull))
	renamed := fixtureValue(types.NewStructType([]types.StructField{{Name: "y", Type: integer}, {Name: "x", Type: integer}}, foundation.NonNull))
	for _, tc := range []struct {
		name        string
		left, right valueinterfaces.Value
		equal       bool
	}{
		{"integer equal", number(42), number(42), true},
		{"integer differs", number(42), number(43), false},
		{"integer width", number(1), fixtureValue(testbedvalues.NewInteger(fixtureValue(types.NewIntType(types.Int8, foundation.NonNull)), 1)), false},
		{"nullability", number(1), fixtureValue(testbedvalues.NewInteger(nullable, 1)), false},
		{"separate types", number(1), fixtureValue(testbedvalues.NewInteger(fixtureValue(types.NewIntType(types.Int64, foundation.NonNull)), 1)), true},
		{"bool equal", fixtureValue(testbedvalues.NewBool(boolean, true)), fixtureValue(testbedvalues.NewBool(boolean, true)), true},
		{"bool differs", fixtureValue(testbedvalues.NewBool(boolean, true)), fixtureValue(testbedvalues.NewBool(boolean, false)), false},
		{"cross variant", number(1), fixtureValue(testbedvalues.NewBool(boolean, true)), false},
		{"text equal", fixtureValue(testbedvalues.NewVarChar(text, "tsa_01")), fixtureValue(testbedvalues.NewVarChar(text, "tsa_01")), true},
		{"text bytes", fixtureValue(testbedvalues.NewVarChar(text, "é")), fixtureValue(testbedvalues.NewVarChar(text, "e\u0301")), false},
		{"text bound", fixtureValue(testbedvalues.NewVarChar(text, "a")), fixtureValue(testbedvalues.NewVarChar(bounded, "a")), false},
		{"null equal", fixtureValue(testbedvalues.NewNull(nullable)), fixtureValue(testbedvalues.NewNull(fixtureValue(types.NewIntType(types.Int64, foundation.MayNull)))), true},
		{"null types", fixtureValue(testbedvalues.NewNull(nullable)), fixtureValue(testbedvalues.NewNull(boolean)), false},
		{"null versus present", fixtureValue(testbedvalues.NewNull(nullable)), fixtureValue(testbedvalues.NewInteger(nullable, 0)), false},
		{"array equal", list(1, 2), list(1, 2), true},
		{"array order", list(1, 2), list(2, 1), false},
		{"array length", list(1), list(1, 1), false},
		{"empty array", list(), list(), true},
		{"record equal", fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{number(1), number(2)})), fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{number(1), number(2)})), true},
		{"record field order", fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{number(1), number(2)})), fixtureValue(testbedvalues.NewStruct(renamed, []valueinterfaces.Value{number(1), number(2)})), false},
		{"record payload", fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{number(1), number(2)})), fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{number(2), number(1)})), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.left.EqualsTo(tc.left) || !tc.right.EqualsTo(tc.right) {
				t.Fatal("not reflexive")
			}
			if tc.left.EqualsTo(tc.right) != tc.equal || tc.right.EqualsTo(tc.left) != tc.equal {
				t.Fatal("incorrect structural equality")
			}
		})
	}
}

func TestValueEqualityNestedSharedComposites(t *testing.T) {
	build := func(n int64) valueinterfaces.Value {
		integer := fixtureValue(types.NewIntType(types.Int64, foundation.MayNull))
		array := fixtureValue(types.NewArrayType(integer, foundation.NonNull))
		child := fixtureValue(testbedvalues.NewArray(array, []valueinterfaces.Value{fixtureValue(testbedvalues.NewInteger(integer, n)), fixtureValue(testbedvalues.NewNull(integer))}))
		record := fixtureValue(types.NewStructType([]types.StructField{{Name: "items", Type: array}}, foundation.NonNull))
		var result valueinterfaces.Value = fixtureValue(testbedvalues.NewStruct(record, []valueinterfaces.Value{child}))
		// Each level shares its child twice. Direct equality revisits distinct
		// equal child pairs per occurrence while preserving the shared graph.
		for i := 0; i < 16; i++ {
			typ := fixtureValue(types.NewArrayType(result.Type(), foundation.NonNull))
			result = fixtureValue(testbedvalues.NewArray(typ, []valueinterfaces.Value{result, result}))
		}
		return result
	}
	a, b, c := build(1), build(1), build(1)
	before := a.(*testbedvalues.ArrayValue).Elements()
	if !a.EqualsTo(b) || !b.EqualsTo(c) || !a.EqualsTo(c) || a.EqualsTo(build(2)) {
		t.Fatal("nested/shared equality failed")
	}
	after := a.(*testbedvalues.ArrayValue).Elements()
	if before[0] != after[0] || after[0] != after[1] {
		t.Fatal("comparison changed sharing")
	}
	nullType := fixtureValue(a.Type().WithNullability(foundation.MayNull))
	if !fixtureValue(testbedvalues.NewNull(nullType)).EqualsTo(fixtureValue(testbedvalues.NewNull(nullType))) {
		t.Fatal("composite nulls differ")
	}
}

func TestValueEqualityNilAssertions(t *testing.T) {
	valid := fixtureValue(testbedvalues.NewBool(fixtureValue(types.NewBoolType(foundation.NonNull)), false))
	for _, invalid := range []valueinterfaces.Value{nil, (*testbedvalues.NullValue)(nil), (*testbedvalues.BoolValue)(nil), (*testbedvalues.IntegerValue)(nil), (*testbedvalues.VarCharValue)(nil), (*testbedvalues.ArrayValue)(nil), (*testbedvalues.StructValue)(nil)} {
		for _, call := range []func(){func() { valid.EqualsTo(invalid) }, func() { invalid.EqualsTo(valid) }} {
			func() {
				if !assertions.Enabled {
					return
				}
				defer func() {
					if recover() == nil {
						t.Error("nil input did not panic")
					}
				}()
				call()
			}()
		}
	}
}
