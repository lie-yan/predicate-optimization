package values_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestScalarValidation(t *testing.T) {
	for _, w := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		typ, _ := types.NewIntType(w, foundation.NonNull)
		low, high := int64(math.MinInt64), int64(math.MaxInt64)
		if w != types.Int64 {
			high = (int64(1) << (w - 1)) - 1
			low = -high - 1
		}
		for _, n := range []int64{low, 0, high} {
			v, err := testbedvalues.NewInteger(typ, n)
			if err != nil {
				t.Fatal(err)
			}
			if got := v.Integer(); got != n {
				t.Fatal("integer payload lost")
			}
		}
		if w != types.Int64 {
			for _, n := range []int64{low - 1, high + 1} {
				if _, err := testbedvalues.NewInteger(typ, n); err == nil {
					t.Fatal("overflow accepted")
				}
			}
		}
		if _, err := testbedvalues.NewNull(typ); err == nil {
			t.Fatal("non-nullable null accepted")
		}
	}
	limit := uint64(5)
	str, _ := types.NewVarCharType(&limit, foundation.MayNull)
	if _, err := testbedvalues.NewVarChar(str, "中文"); err == nil {
		t.Fatal("byte limit ignored")
	}
	v, err := testbedvalues.NewVarChar(str, "中")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.VarChar(); got != "中" {
		t.Fatal("string changed")
	}
	b, err := testbedvalues.NewBool(&types.BoolType{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Bool(); got {
		t.Fatal("false confused with missing payload")
	}
}

func TestCompositesAndOwnership(t *testing.T) {
	integer, _ := types.NewIntType(types.Int8, foundation.MayNull)
	array, _ := types.NewArrayType(integer, foundation.MayNull)
	one, _ := testbedvalues.NewInteger(integer, 1)
	null, _ := testbedvalues.NewNull(integer)
	input := []valueinterfaces.Value{one, null, one}
	a, err := testbedvalues.NewArray(array, input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = nil
	children := a.Elements()
	if len(children) != 3 || children[0] != one || !children[1].IsNull() {
		t.Fatal("array contents changed")
	}
	children[0] = nil
	children = a.Elements()
	if children[0] != one {
		t.Fatal("accessor aliases storage")
	}
	empty, err := testbedvalues.NewArray(array, nil)
	if err != nil {
		t.Fatal(err)
	}
	nullArray, err := testbedvalues.NewNull(array)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Elements()) != 0 || empty.IsNull() {
		t.Fatal("empty confused with null")
	}
	if !nullArray.IsNull() {
		t.Fatal("null confused with empty")
	}
	nested, _ := types.NewArrayType(array, foundation.NonNull)
	if _, err := testbedvalues.NewArray(nested, []valueinterfaces.Value{a, empty, nullArray}); err != nil {
		t.Fatal(err)
	}
	record, _ := types.NewStructType([]types.StructField{{Name: "items", Type: nested}}, foundation.NonNull)
	nestedValue, _ := testbedvalues.NewArray(nested, []valueinterfaces.Value{a})
	fields := []valueinterfaces.Value{nestedValue}
	r, err := testbedvalues.NewStruct(record, fields)
	if err != nil {
		t.Fatal(err)
	}
	fields[0] = nil
	got := r.Fields()
	if got[0] != nestedValue {
		t.Fatal("struct aliases input")
	}
	got[0] = nil
	got = r.Fields()
	if got[0] != nestedValue {
		t.Fatal("struct accessor aliases storage")
	}
	if _, err := testbedvalues.NewStruct(record, nil); err == nil {
		t.Fatal("field count ignored")
	}
	nonnull, _ := types.NewIntType(types.Int8, foundation.NonNull)
	other, _ := testbedvalues.NewInteger(nonnull, 1)
	if _, err := testbedvalues.NewArray(array, []valueinterfaces.Value{other}); err == nil {
		t.Fatal("implicit nullability conversion")
	}
	if _, err := testbedvalues.NewStruct(record, []valueinterfaces.Value{one}); err == nil {
		t.Fatal("mismatched field accepted")
	}
}

func TestInterfaceVariants(t *testing.T) {
	boolean := &types.BoolType{}
	concrete, err := testbedvalues.NewBool(boolean, true)
	if err != nil {
		t.Fatal(err)
	}
	var v valueinterfaces.Value = concrete
	if v.IsNull() || !v.Type().EqualsTo(boolean) {
		t.Fatal("interface methods lost value information")
	}
	if payload, ok := v.(*testbedvalues.BoolValue); !ok || !payload.Bool() {
		t.Fatal("concrete payload unavailable")
	}
	null, err := testbedvalues.NewNull(boolean)
	if err != nil {
		t.Fatal(err)
	}
	v = null
	if !v.IsNull() || !v.Type().EqualsTo(boolean) {
		t.Fatal("typed null lost")
	}
}
