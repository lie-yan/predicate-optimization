package equatable_value_set_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	equatable "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestBooleanSetEnumerationAndOwnership(t *testing.T) {
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		typ := testutil.Must(types.NewBoolType(n))
		no := testutil.Must(testbedvalues.NewBool(typ, false))
		yes := testutil.Must(testbedvalues.NewBool(typ, true))
		for _, tc := range []struct {
			input []valueinterfaces.Value
			want  []bool
		}{
			{nil, []bool{}},
			{[]valueinterfaces.Value{yes, yes}, []bool{true}},
			{[]valueinterfaces.Value{no, no}, []bool{false}},
			{[]valueinterfaces.Value{yes, no, yes}, []bool{true, false}},
			{[]valueinterfaces.Value{no, yes, no}, []bool{false, true}},
		} {
			set := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, tc.input))
			if len(tc.input) > 0 {
				tc.input[0] = nil
			}
			for round := 0; round < 2; round++ {
				result := set.Values()
				got := make([]bool, 0, len(result))
				for _, v := range result {
					if !v.Type().EqualsTo(set.Type()) || v.IsNull() {
						t.Fatal("wrong value type")
					}
					got = append(got, v.(*testbedvalues.BoolValue).Bool())
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
				if len(result) > 0 {
					result[0] = nil
				}
			}
			if set.Type().Nullability() != foundation.NonNull || typ.Nullability() != n {
				t.Fatal("normalization mutated original type")
			}
		}
	}
}

func TestBooleanSetInputErrors(t *testing.T) {
	typ := testutil.Must(types.NewBoolType(foundation.NonNull))
	nullable := testutil.Must(types.NewBoolType(foundation.MayNull))
	integer := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	for _, tc := range []struct {
		typ   types.Type
		input []valueinterfaces.Value
		want  error
	}{
		{typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewNull(nullable))}, value_set.ErrInvalidInput},

		{typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewInteger(integer, 1))}, value_set.ErrIncompatibleTypes},

		{testutil.Must(types.NewArrayType(typ, foundation.NonNull)), nil, value_set.ErrUnsupported},
	} {
		got, err := equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, tc.typ, tc.input)
		if got != nil || !errors.Is(err, tc.want) {
			t.Fatalf("got %v, %v; want %v", got, err, tc.want)
		}
	}
	// A nullable declaration with a concrete non-null payload is admitted here.
	set := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewBool(nullable, true))}))
	if len(set.Values()) != 1 {
		t.Fatal("nullable-typed concrete value lost")
	}
}

func TestBooleanSetPreconditions(t *testing.T) {
	typ := testutil.Must(types.NewBoolType(foundation.NonNull))
	for _, call := range []func(){
		func() { _, _ = equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, nil, nil) },
		func() {
			_, _ = equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, (*types.BoolType)(nil), nil)
		},
		func() {
			_, _ = equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{nil})
		},
		func() {
			_, _ = equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{(*testbedvalues.BoolValue)(nil)})
		},
		func() { var s *equatable.EquatableValueSet; s.Values() },
		func() { (&equatable.EquatableValueSet{}).Type() },
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("expected precondition panic")
				}
			}()
			call()
		}()
	}
}

func TestBooleanSetSharesValuesAndNormalizesBeforeDeduplication(t *testing.T) {
	typ := testutil.Must(types.NewBoolType(foundation.NonNull))
	nullable := testutil.Must(types.NewBoolType(foundation.MayNull))
	first := testutil.Must(testbedvalues.NewBool(typ, true))
	input := []valueinterfaces.Value{first, testutil.Must(testbedvalues.NewBool(nullable, true)), testutil.Must(testbedvalues.NewBool(typ, true))}
	set := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, nullable, input))
	got := set.Values()
	if len(got) != 1 || got[0] != first || set.Values()[0] != first {
		t.Fatal("expected deduplication by normalized value and sharing of immutable values")
	}
}

func TestIntegerSetConstruction(t *testing.T) {
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		typ := testutil.Must(types.NewIntType(width, foundation.NonNull))
		nullable := testutil.Must(types.NewIntType(width, foundation.MayNull))
		maximum := int64(1)<<(width-1) - 1
		minimum := -maximum - 1
		input := []valueinterfaces.Value{
			testutil.Must(testbedvalues.NewInteger(typ, maximum)),
			testutil.Must(testbedvalues.NewInteger(nullable, minimum)),
			testutil.Must(testbedvalues.NewInteger(nullable, maximum)),
			testutil.Must(testbedvalues.NewInteger(testutil.Must(types.NewIntType(width, foundation.NonNull)), minimum)),
		}
		got := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, nullable, input)).Values()
		if len(got) != 2 || got[0].(*testbedvalues.IntegerValue).Integer() != minimum || got[1].(*testbedvalues.IntegerValue).Integer() != maximum {
			t.Fatalf("wrong enumeration for width %d: %v", width, got)
		}
		for _, v := range got {
			if !v.Type().EqualsTo(typ) {
				t.Fatal("wrong normalized type")
			}
		}
	}
}

func TestVarCharSetConstruction(t *testing.T) {
	limit := uint64(2)
	for _, bound := range []*uint64{nil, &limit} {
		typ := testutil.Must(types.NewVarCharType(bound, foundation.NonNull))
		nullable := testutil.Must(types.NewVarCharType(bound, foundation.MayNull))
		payloads := []string{"", "a", "a\x00", "ab", "\xff"}
		var input []valueinterfaces.Value
		for _, payload := range payloads {
			input = append(input, testutil.Must(testbedvalues.NewVarChar(typ, payload)), testutil.Must(testbedvalues.NewVarChar(nullable, payload)), testutil.Must(testbedvalues.NewVarChar(testutil.Must(types.NewVarCharType(bound, foundation.NonNull)), payload)))
		}
		got := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, nullable, input)).Values()
		if len(got) != len(payloads) {
			t.Fatal("wrong distinct count")
		}
		for i, v := range got {
			if v.(*testbedvalues.VarCharValue).VarChar() != payloads[i] || !v.Type().EqualsTo(typ) {
				t.Fatal("wrong normalized enumeration")
			}
		}
	}
}

func TestScalarSetAdmissionErrors(t *testing.T) {
	integer := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	limit, otherLimit := uint64(2), uint64(3)
	varchar := testutil.Must(types.NewVarCharType(&limit, foundation.NonNull))
	for _, tc := range []struct {
		typ   types.Type
		value valueinterfaces.Value
		want  error
	}{
		{integer, testutil.Must(testbedvalues.NewInteger(testutil.Must(types.NewIntType(types.Int16, foundation.NonNull)), 1)), value_set.ErrIncompatibleTypes},
		{integer, testutil.Must(testbedvalues.NewVarChar(varchar, "a")), value_set.ErrIncompatibleTypes},
		{varchar, testutil.Must(testbedvalues.NewInteger(integer, 1)), value_set.ErrIncompatibleTypes},
		{varchar, testutil.Must(testbedvalues.NewVarChar(testutil.Must(types.NewVarCharType(nil, foundation.NonNull)), "a")), value_set.ErrIncompatibleTypes},
		{varchar, testutil.Must(testbedvalues.NewVarChar(testutil.Must(types.NewVarCharType(&otherLimit, foundation.NonNull)), "a")), value_set.ErrIncompatibleTypes},

		{integer, testutil.Must(testbedvalues.NewNull(testutil.Must(integer.WithNullability(foundation.MayNull)))), value_set.ErrInvalidInput},
		{varchar, testutil.Must(testbedvalues.NewNull(testutil.Must(varchar.WithNullability(foundation.MayNull)))), value_set.ErrInvalidInput},
	} {
		if _, err := equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, tc.typ, []valueinterfaces.Value{tc.value}); !errors.Is(err, tc.want) {
			t.Fatalf("got %v, want %v", err, tc.want)
		}
	}
}
