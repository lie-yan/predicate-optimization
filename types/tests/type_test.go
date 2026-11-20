package types_test

import (
	"reflect"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"

	types "github.com/lie-yan/predicate-optimization/types"
)

func must(t *testing.T, typ types.Type, err error) types.Type {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return typ
}

func integer(t *testing.T, width types.IntWidth, n foundation.Nullability) types.Type {
	t.Helper()
	v, err := types.NewIntType(width, n)
	return must(t, v, err)
}

func array(t *testing.T, element types.Type, n foundation.Nullability) types.Type {
	t.Helper()
	v, err := types.NewArrayType(element, n)
	return must(t, v, err)
}

func TestNestedIdentity(t *testing.T) {
	makeRecord := func(n foundation.Nullability) types.Type {
		v, err := types.NewStructType([]types.StructField{
			{Name: "id", Type: integer(t, types.Int64, foundation.NonNull)},
			{Name: "values", Type: array(t, array(t, integer(t, types.Int8, n), foundation.MayNull), foundation.NonNull)},
		}, foundation.NonNull)
		return must(t, v, err)
	}
	a, b := makeRecord(foundation.MayNull), makeRecord(foundation.MayNull)
	if a == b || !a.EqualsTo(b) {
		t.Fatal("independently constructed types must compare structurally")
	}
	if a.EqualsTo(makeRecord(foundation.NonNull)) {
		t.Fatal("nested nullability lost")
	}
	nullable, err := a.WithNullability(foundation.MayNull)
	must(t, nullable, err)
	if a.EqualsTo(nullable) || a.Nullability() != foundation.NonNull {
		t.Fatal("outer nullability must distinguish types without mutation")
	}
	fields := a.(*types.StructType).Fields()
	reordered, err := types.NewStructType([]types.StructField{fields[1], fields[0]}, foundation.NonNull)
	must(t, reordered, err)
	if a.EqualsTo(reordered) {
		t.Fatal("field order lost")
	}
	fields[0].Name = "other"
	renamed, err := types.NewStructType(fields, foundation.NonNull)
	must(t, renamed, err)
	if a.EqualsTo(renamed) {
		t.Fatal("field name lost")
	}
}

func TestOwnership(t *testing.T) {
	bound := uint64(100)
	str, err := types.NewVarCharType(&bound, foundation.MayNull)
	must(t, str, err)
	bound = 200
	got, bounded := str.MaxBytes()
	if !bounded || got != 100 {
		t.Fatal("bound aliases constructor input")
	}
	fields := []types.StructField{{Name: "s", Type: str}}
	record, err := types.NewStructType(fields, foundation.NonNull)
	must(t, record, err)
	fields[0].Name = "changed"
	out := record.Fields()
	if out[0].Name != "s" {
		t.Fatal("fields alias constructor input")
	}
	out[0].Type = integer(t, types.Int8, foundation.NonNull)
	out = record.Fields()
	if !out[0].Type.EqualsTo(str) {
		t.Fatal("fields accessor exposes mutation")
	}
}

func TestInvalidConstruction(t *testing.T) {
	valid := integer(t, types.Int8, foundation.NonNull)
	cases := map[string]func() (types.Type, error){
		"width":               func() (types.Type, error) { return types.NewIntType(types.IntWidth(7), foundation.NonNull) },
		"bool nullability":    func() (types.Type, error) { return types.NewBoolType(99) },
		"int nullability":     func() (types.Type, error) { return types.NewIntType(types.Int8, 99) },
		"varchar nullability": func() (types.Type, error) { return types.NewVarCharType(nil, 99) },

		"array nullability": func() (types.Type, error) { return types.NewArrayType(valid, 99) },
		"duplicate fields": func() (types.Type, error) {
			return types.NewStructType([]types.StructField{{Name: "x", Type: valid}, {Name: "x", Type: valid}}, foundation.NonNull)
		},
		"empty name": func() (types.Type, error) {
			return types.NewStructType([]types.StructField{{Type: valid}}, foundation.NonNull)
		},
		"struct nullability":  func() (types.Type, error) { return types.NewStructType(nil, 99) },
		"invalid replacement": func() (types.Type, error) { return valid.WithNullability(99) },
	}
	for name, construct := range cases {
		t.Run(name, func(t *testing.T) {
			v, err := construct()
			if err == nil || (v != nil && !reflect.ValueOf(v).IsNil()) {
				t.Fatal("expected error and nil descriptor")
			}
		})
	}
}

func TestPrimitiveIdentity(t *testing.T) {
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		v := integer(t, width, foundation.MayNull)
		got := v.(*types.IntType).IntegerWidth()
		if got != width || !v.EqualsTo(integer(t, width, foundation.MayNull)) {
			t.Fatal("integer width not preserved")
		}
		if width != types.Int64 && v.EqualsTo(integer(t, types.Int64, foundation.MayNull)) {
			t.Fatal("different integer widths compare equal")
		}
	}
	boolean, err := types.NewBoolType(foundation.NonNull)
	must(t, boolean, err)
	if boolean.EqualsTo(integer(t, types.Int8, foundation.NonNull)) {
		t.Fatal("different variants compare equal")
	}
	unbounded, err := types.NewVarCharType(nil, foundation.MayNull)
	must(t, unbounded, err)
	zeroLimit := uint64(0)
	zero, err := types.NewVarCharType(&zeroLimit, foundation.MayNull)
	must(t, zero, err)
	limit := uint64(100)
	bounded, err := types.NewVarCharType(&limit, foundation.MayNull)
	must(t, bounded, err)
	if unbounded.EqualsTo(zero) || zero.EqualsTo(bounded) {
		t.Fatal("absent, zero and positive byte bounds must remain distinct")
	}
}

// Every ordered pair exercises both sides of variant dispatch, including Bool.
func TestDifferentVariants(t *testing.T) {
	boolean, _ := types.NewBoolType(foundation.MayNull)
	text, _ := types.NewVarCharType(nil, foundation.MayNull)
	record, _ := types.NewStructType(nil, foundation.MayNull)
	variants := []types.Type{
		boolean, integer(t, types.Int8, foundation.MayNull), text,
		array(t, boolean, foundation.MayNull), record,
	}
	for i, a := range variants {
		for j, b := range variants {
			if got := a.EqualsTo(b); got != (i == j) {
				t.Errorf("SameType(%T, %T) = %v", a, b, got)
			}
		}
	}
}

func TestZeroValues(t *testing.T) {
	boolean, _ := types.NewBoolType(foundation.MayNull)
	text, _ := types.NewVarCharType(nil, foundation.MayNull)
	record, _ := types.NewStructType(nil, foundation.MayNull)
	for _, pair := range [][2]types.Type{
		{&types.BoolType{}, boolean},
		{&types.VarCharType{}, text},
		{&types.StructType{}, record},
	} {
		if pair[0].Nullability() != foundation.MayNull || !pair[0].EqualsTo(pair[1]) {
			t.Errorf("unexpected zero-value semantics for %T", pair[0])
		}
	}
}

func TestWithNullabilityDispatch(t *testing.T) {
	boolean, _ := types.NewBoolType(foundation.NonNull)
	integer, _ := types.NewIntType(types.Int16, foundation.NonNull)
	limit := uint64(42)
	text, _ := types.NewVarCharType(&limit, foundation.NonNull)
	arr, _ := types.NewArrayType(integer, foundation.NonNull)
	record, _ := types.NewStructType([]types.StructField{{Name: "items", Type: arr}}, foundation.NonNull)
	for _, original := range []types.Type{boolean, integer, text, arr, record} {
		nullable, err := original.WithNullability(foundation.MayNull)
		must(t, nullable, err)
		if nullable.Nullability() != foundation.MayNull || original.Nullability() != foundation.NonNull {
			t.Fatalf("incorrect nullability for %T", original)
		}
		restored, err := nullable.WithNullability(foundation.NonNull)
		must(t, restored, err)
		if !original.EqualsTo(restored) {
			t.Errorf("variant data lost for %T", original)
		}
		if v, err := original.WithNullability(99); err == nil || v != nil {
			t.Errorf("invalid nullability accepted for %T", original)
		}
	}
}

func TestNonNilAssertions(t *testing.T) {
	expectPanic := func(f func()) {
		t.Helper()
		if !assertions.Enabled {
			return
		}
		defer func() {
			if recover() == nil {
				t.Error("expected non-nil assertion panic")
			}
		}()
		f()
	}
	valid := &types.BoolType{}
	expectPanic(func() { types.Type(nil).EqualsTo(valid) })
	expectPanic(func() { valid.EqualsTo(nil) })
	expectPanic(func() { types.Type(nil).EqualsTo(nil) })
	for _, nilType := range []types.Type{
		(*types.BoolType)(nil), (*types.IntType)(nil),
		(*types.VarCharType)(nil), (*types.ArrayType)(nil), (*types.StructType)(nil),
	} {
		expectPanic(func() { nilType.WithNullability(foundation.MayNull) })
		expectPanic(func() { nilType.EqualsTo(valid) })
		expectPanic(func() { valid.EqualsTo(nilType) })
		expectPanic(func() { nilType.EqualsTo(nilType) })
	}
}
