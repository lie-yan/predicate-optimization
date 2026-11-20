package functions_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type arrayFixture struct {
	context *testbed.Context
	integer *types.IntType
	array   *types.ArrayType
}

func newArrayFixture(t *testing.T) arrayFixture {
	t.Helper()
	provider, err := testbed.NewStandardContext()
	if err != nil {
		t.Fatal(err)
	}
	integer, err := types.NewIntType(types.Int64, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	array, err := types.NewArrayType(integer, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	return arrayFixture{provider, integer, array}
}

func (f arrayFixture) value(t *testing.T, numbers ...int64) valueinterfaces.Value {
	t.Helper()
	elements := make([]valueinterfaces.Value, len(numbers))
	for i, number := range numbers {
		elements[i] = evalInteger(t, f.integer, number)
	}
	v, err := testbedvalues.NewArray(f.array, elements)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f arrayFixture) evaluate(t *testing.T, name shared.BuiltInFunctionName, array, candidate valueinterfaces.Value) foundation.TruthValue {
	t.Helper()
	handle, err := f.context.LookupBuiltInFunction(name, []types.Type{array.Type(), candidate.Type()})
	if err != nil {
		t.Fatal(err)
	}
	result := evaluateStandard(t, f.context, handle, array, candidate)
	if result.Type().Nullability() != array.Type().Nullability() {
		t.Fatal("incorrect effective nullability")
	}
	if result.IsNull() {
		return foundation.Null
	}
	if result.(*testbedvalues.BoolValue).Bool() {
		return foundation.True
	}
	return foundation.False
}

func TestStandardArrayMembershipFixtures(t *testing.T) {
	f := newArrayFixture(t)
	for _, tc := range []struct {
		name       string
		array      []int64
		candidates []int64
		any        bool
		want       foundation.TruthValue
	}{
		{"contains match", []int64{8791003, 8791004}, []int64{8791003}, false, foundation.True},
		{"contains miss", []int64{8791003, 8791004}, []int64{8791005}, false, foundation.False},
		{"contains empty", nil, []int64{8791003}, false, foundation.False},
		{"contains duplicate", []int64{8791003, 8791003}, []int64{8791003}, false, foundation.True},
		{"any match", []int64{8791003, 8791004}, []int64{8791005, 8791004}, true, foundation.True},
		{"any miss", []int64{8791003, 8791004}, []int64{8791005}, true, foundation.False},
		{"any empty input", nil, []int64{8791003}, true, foundation.False},
		{"any empty candidates", []int64{8791003}, nil, true, foundation.False},
		{"any both empty", nil, nil, true, foundation.False},
		{"any duplicates", []int64{8791003}, []int64{8791003, 8791003}, true, foundation.True},
		{"extreme match", []int64{math.MinInt64, math.MaxInt64}, []int64{math.MaxInt64}, true, foundation.True},
		{"negative miss", []int64{-1}, []int64{1}, false, foundation.False},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := shared.BuiltInArrayContains
			var candidate valueinterfaces.Value
			if tc.any {
				name = shared.BuiltInArrayContainsAny
				candidate = f.value(t, tc.candidates...)
			} else {
				candidate = evalInteger(t, f.integer, tc.candidates[0])
			}
			for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				array, err := f.value(t, tc.array...).WithNullability(n)
				if err != nil {
					t.Fatal(err)
				}
				if got := f.evaluate(t, name, array, candidate); got != tc.want {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
				if n == foundation.MayNull {
					null, err := testbedvalues.NewNull(array.Type())
					if err != nil {
						t.Fatal(err)
					}
					if got := f.evaluate(t, name, null, candidate); got != foundation.Null {
						t.Fatal("null array must propagate, including empty candidates")
					}
				}
			}
		})
	}
}

func TestStandardArrayTouchstoneRelationships(t *testing.T) {
	f := newArrayFixture(t)
	tags := []int64{8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009}
	inputs := [][]int64{nil, {42}, {8791003, 8791003}, {8791004, 8791005}}
	for _, tag := range tags {
		inputs = append(inputs, []int64{tag})
	}
	for _, numbers := range inputs {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			array, err := f.value(t, numbers...).WithNullability(n)
			if err != nil {
				t.Fatal(err)
			}
			checkTagUnion(t, f, array, tags)
		}
	}
	nullable, err := f.array.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	null, err := testbedvalues.NewNull(nullable)
	if err != nil {
		t.Fatal(err)
	}
	checkTagUnion(t, f, null, tags)
	for _, id := range []int64{8792001, 8793001} {
		for _, array := range []valueinterfaces.Value{f.value(t), f.value(t, 42), f.value(t, id), f.value(t, id, id), null} {
			contains := f.evaluate(t, shared.BuiltInArrayContains, array, evalInteger(t, f.integer, id))
			any := f.evaluate(t, shared.BuiltInArrayContainsAny, array, f.value(t, id))
			if contains != any {
				t.Fatal("singleton contains-any differs from contains")
			}
		}
	}
}

func checkTagUnion(t *testing.T, f arrayFixture, array valueinterfaces.Value, tags []int64) {
	t.Helper()
	// Independent three-valued OR oracle: true dominates null, then false.
	union := foundation.False
	for _, tag := range tags {
		result := f.evaluate(t, shared.BuiltInArrayContains, array, evalInteger(t, f.integer, tag))
		if result == foundation.True {
			union = foundation.True
		} else if result == foundation.Null && union != foundation.True {
			union = foundation.Null
		}
	}
	if got := f.evaluate(t, shared.BuiltInArrayContainsAny, array, f.value(t, tags...)); got != union {
		t.Fatalf("contains-any %v differs from OR %v", got, union)
	}
}

func TestStandardArrayUnsupportedInputs(t *testing.T) {
	f := newArrayFixture(t)
	nullableInteger, err := f.integer.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	nullableArray, err := f.array.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	nullableElements, err := types.NewArrayType(nullableInteger, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	nested, err := types.NewArrayType(f.array, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := types.NewIntType(types.Int8, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name shared.BuiltInFunctionName
		args []types.Type
	}{
		{shared.BuiltInArrayContains, []types.Type{f.array, nullableInteger}},
		{shared.BuiltInArrayContainsAny, []types.Type{f.array, nullableArray}},
		{shared.BuiltInArrayContains, []types.Type{nullableElements, f.integer}},
		{shared.BuiltInArrayContainsAny, []types.Type{f.array, nullableElements}},
		{shared.BuiltInArrayContains, []types.Type{nested, f.array}},
		{shared.BuiltInArrayContains, []types.Type{f.array, narrow}},
		{shared.BuiltInArrayContainsAll, []types.Type{f.array, f.array}},
	} {
		if _, err := f.context.LookupBuiltInFunction(tc.name, tc.args); !errors.Is(err, shared.ErrUnsupportedLookup) {
			t.Fatalf("%s %v: got %v", tc.name, tc.args, err)
		}
	}
}
