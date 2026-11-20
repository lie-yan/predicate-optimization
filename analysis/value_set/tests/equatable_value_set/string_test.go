package equatable_value_set_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

var _ fmt.Stringer = (*finite.EquatableValueSet)(nil)

// Check both polarities, repeated formatting, and ownership of stored Values.
func checkFiniteStrings(t *testing.T, typ types.Type, input []valueinterfaces.Value, want string) {
	t.Helper()
	for _, inclusion := range []bool{true, false} {
		set := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, input))
		expected := want
		if !inclusion {
			set = testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, input))
			expected = "ALL \\ " + want
			if len(input) == 0 {
				expected = "ALL"
			}
		}
		before, originalType := set.Values(), set.Type()
		original := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, before))
		if !inclusion {
			original = testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, before))
		}
		for round := 0; round < 2; round++ {
			if got := set.String(); got != expected {
				t.Fatalf("got %q, want %q", got, expected)
			}
			if got := fmt.Sprint(set); got != expected {
				t.Fatalf("fmt.Sprint: got %q, want %q", got, expected)
			}
			after := set.Values()
			if len(after) != len(before) || set.Type() != originalType || set.IsInclusion() != inclusion {
				t.Fatal("formatting changed representation")
			}
			for i, v := range before {
				if v != after[i] {
					t.Fatal("formatting changed entry identity or order")
				}
			}
			if !testutil.Must(set.EqualsTo(original)) {
				t.Fatal("formatting changed mathematical set")
			}
		}
	}
}

func TestFiniteBooleanStrings(t *testing.T) {
	typ := testutil.Must(types.NewBoolType(foundation.NonNull))
	no, yes := testutil.Must(testbedvalues.NewBool(typ, false)), testutil.Must(testbedvalues.NewBool(typ, true))
	for _, tc := range []struct {
		input []valueinterfaces.Value
		want  string
	}{
		{nil, "{}"},
		{[]valueinterfaces.Value{no}, "{false}"},
		{[]valueinterfaces.Value{yes}, "{true}"},
		{[]valueinterfaces.Value{yes, no, yes}, "{false, true}"},
		{[]valueinterfaces.Value{no, yes, no}, "{false, true}"},
	} {
		checkFiniteStrings(t, typ, tc.input, tc.want)
	}
}

func TestFiniteIntegerStrings(t *testing.T) {
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		typ := testutil.Must(types.NewIntType(width, foundation.MayNull))
		checkFiniteStrings(t, typ, nil, "{}")
		for _, numbers := range [][]int64{{8, 1, 3, 1}, {3, 8, 1, 8}} {
			var input []valueinterfaces.Value
			for _, number := range numbers {
				input = append(input, testutil.Must(testbedvalues.NewInteger(typ, number)))
			}
			checkFiniteStrings(t, typ, input, "{1, 3, 8}")
		}
	}
}

func TestFiniteFloatStrings(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		var input []valueinterfaces.Value
		for _, number := range []float64{math.NaN(), math.Inf(1), math.Copysign(0, -1), -1.5, 0, math.Inf(-1), math.NaN()} {
			input = append(input, testutil.Must(testbedvalues.NewFloat(typ, number)))
		}
		checkFiniteStrings(t, typ, input, "{-Inf, -1.5, 0, +Inf, NaN}")
		checkFiniteStrings(t, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewFloat(typ, math.NaN()))}, "{NaN}")
		checkFiniteStrings(t, typ, nil, "{}")
	}
}

func TestFiniteVarCharStrings(t *testing.T) {
	typ := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	var input []valueinterfaces.Value
	for _, s := range []string{"tsa_02", "tsa_01", "tsa_01", "\xff", "\x00\n", "a\"\\"} {
		input = append(input, testutil.Must(testbedvalues.NewVarChar(typ, s)))
	}
	checkFiniteStrings(t, typ, input, `{"\x00\n", "a\"\\", "tsa_01", "tsa_02", "\xff"}`)
	checkFiniteStrings(t, typ, nil, "{}")
	// A directly stored finite representation is retained even when its domain
	// contains only this one value.
	zero := uint64(0)
	bounded := testutil.Must(types.NewVarCharType(&zero, foundation.NonNull))
	checkFiniteStrings(t, bounded, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(bounded, ""))}, `{""}`)
}

func TestFiniteStringPreconditions(t *testing.T) {
	for _, set := range []*finite.EquatableValueSet{nil, {}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected assertion for unconstructed set")
				}
			}()
			_ = set.String()
		}()
	}
}
