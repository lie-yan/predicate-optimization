package equatable_value_set_test

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestOrderedStorageConstructionAndOwnership(t *testing.T) {
	intType := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	textType := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	cases := []struct {
		typ     types.Type
		entries []valueinterfaces.Value
	}{
		{intType, []valueinterfaces.Value{testutil.Must(testbedvalues.NewInteger(intType, math.MaxInt64)), testutil.Must(testbedvalues.NewInteger(intType, 0)), testutil.Must(testbedvalues.NewInteger(intType, math.MinInt64))}},
		{textType, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(textType, "\xff")), testutil.Must(testbedvalues.NewVarChar(textType, "a\x00")), testutil.Must(testbedvalues.NewVarChar(textType, "a")), testutil.Must(testbedvalues.NewVarChar(textType, ""))}},
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		entries := []valueinterfaces.Value{}
		for _, x := range []float64{math.NaN(), math.Inf(1), math.SmallestNonzeroFloat32, 0, -1, math.Inf(-1)} {
			entries = append(entries, testutil.Must(testbedvalues.NewFloat(typ, x)))
		}
		cases = append(cases, struct {
			typ     types.Type
			entries []valueinterfaces.Value
		}{typ, entries})
	}
	for _, tc := range cases {
		t.Run(tc.typ.String(), func(t *testing.T) {
			input := append(slices.Clone(tc.entries), tc.entries...)
			before := slices.Clone(input)
			set := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, tc.typ, input))
			if !slices.Equal(input, before) {
				t.Fatal("constructor sorted caller slice")
			}
			testutil.AssertOrderedFiniteStorage(t, set)
			got := set.Values()
			if len(got) != len(tc.entries) {
				t.Fatal("duplicate entries retained")
			}
			for _, entry := range tc.entries {
				if !testutil.Must(set.Contains(entry)) {
					t.Fatal("stored entry missing")
				}
			}
			for i, entry := range got {
				if entry != tc.entries[len(tc.entries)-1-i] {
					t.Fatal("ordering lost shared value")
				}
			}
			reverse := slices.Clone(tc.entries)
			slices.Reverse(reverse)
			if !testutil.Must(set.EqualsTo(testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, tc.typ, reverse)))) {
				t.Fatal("permutation changed membership")
			}
			excluded := testutil.Must(set.Complement())
			testutil.AssertOrderedFiniteStorage(t, excluded)
			for _, entry := range tc.entries {
				if testutil.Must(excluded.Contains(entry)) {
					t.Fatal("exclusion lost")
				}
			}
			input[0] = nil
			got[0] = nil
			if set.Values()[0] == nil || excluded.(*finite.EquatableValueSet).Values()[0] == nil {
				t.Fatal("exposed mutable storage")
			}
		})
	}
}

func TestOrderedFloatAndStringBinaryAlgebra(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		universe := []valueinterfaces.Value{}
		// Include a witness outside all selected entries to distinguish finite sets
		// from exclusions in the full float domain. NaN precedes ordinary values in
		// input so construction must move it to the storage-order tail.
		for _, x := range []float64{math.NaN(), 0, math.Inf(-1), math.Inf(1), -1, 1} {
			universe = append(universe, testutil.Must(testbedvalues.NewFloat(typ, x)))
		}
		testutil.VerifyBinaryAlgebra(t, universe, testutil.EnumerateSets(typ, universe, []int{0, 1, 2, 3, 4}))
	}
	typ := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	universe := []valueinterfaces.Value{}
	for _, s := range []string{"\xff", "a\x00", "", "a", "\x00", "z"} {
		universe = append(universe, testutil.Must(testbedvalues.NewVarChar(typ, s)))
	}
	testutil.VerifyBinaryAlgebra(t, universe, testutil.EnumerateSets(typ, universe, []int{0, 1, 2, 3, 4}))
}

func TestOrderedConstructionRejectsLateInvalidInput(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	one := testutil.Must(testbedvalues.NewInteger(typ, 1))
	for _, tc := range []struct {
		value valueinterfaces.Value
		err   error
	}{
		{testutil.Must(testbedvalues.NewNull(testutil.Must(typ.WithNullability(foundation.MayNull)))), value_set.ErrInvalidInput},
		{testutil.Must(testbedvalues.NewInteger(testutil.Must(types.NewIntType(types.Int8, foundation.NonNull)), 1)), value_set.ErrIncompatibleTypes},
	} {
		input := []valueinterfaces.Value{one, one, tc.value}
		before := slices.Clone(input)
		if result, err := finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, input); result != nil || !errors.Is(err, tc.err) {
			t.Fatalf("unexpected result/error: %v, %v", result, err)
		}
		if !slices.Equal(input, before) {
			t.Fatal("failed construction mutated input")
		}
	}
}
