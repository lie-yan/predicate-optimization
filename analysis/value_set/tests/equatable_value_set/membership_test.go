package equatable_value_set_test

import (
	"errors"
	"math"
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

func TestBooleanPolarityAndComplement(t *testing.T) {
	typ := testutil.Must(types.NewBoolType(foundation.NonNull))
	universe := []valueinterfaces.Value{testutil.Must(testbedvalues.NewBool(typ, false)), testutil.Must(testbedvalues.NewBool(typ, true))}
	for mask := 0; mask < 4; mask++ {
		var entries []valueinterfaces.Value
		for bit, value := range universe {
			if mask&(1<<bit) != 0 {
				entries = append(entries, value)
			}
		}
		for _, isInclusion := range []bool{true, false} {
			var set *equatable.EquatableValueSet
			if isInclusion {
				set = testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, entries))
			} else {
				set = testutil.Must(equatable.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, entries))
			}
			if set.IsInclusion() != isInclusion {
				t.Fatal("wrong polarity")
			}
			var contract value_set.ValueSet = set
			complement := testutil.Must(contract.Complement())
			twice := testutil.Must(complement.Complement())
			if complement.(*equatable.EquatableValueSet).IsInclusion() == isInclusion || twice.(*equatable.EquatableValueSet).IsInclusion() != isInclusion {
				t.Fatal("complement polarity")
			}
			count := 0
			for bit, value := range universe {
				want := (mask&(1<<bit) != 0) == isInclusion
				if testutil.Must(contract.Contains(value)) != want || testutil.Must(complement.Contains(value)) == want || testutil.Must(twice.Contains(value)) != want {
					t.Fatalf("membership mask=%d isInclusion=%v", mask, isInclusion)
				}
				if want {
					count++
				}
			}
			if testutil.Must(contract.IsEmpty()) != (count == 0) || testutil.Must(contract.IsUniversal()) != (count == 2) || testutil.Must(complement.IsEmpty()) != (count == 2) || testutil.Must(complement.IsUniversal()) != (count == 0) {
				t.Fatal("incorrect finite-universe coverage")
			}
			if complement.Type() != set.Type() {
				t.Fatal("immutable type not shared")
			}
			stored := complement.(*equatable.EquatableValueSet).Values()
			if len(stored) != len(entries) {
				t.Fatal("complement enumerated universe")
			}
			if len(stored) > 0 {
				stored[0] = nil
				if set.Values()[0] == nil || complement.(*equatable.EquatableValueSet).Values()[0] == nil {
					t.Fatal("mutable slice escaped")
				}
			}
		}
	}
}

func TestFiniteUniverseCoverage(t *testing.T) {
	integer := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	var integers []valueinterfaces.Value
	for n := int64(-128); n <= 127; n++ {
		integers = append(integers, testutil.Must(testbedvalues.NewInteger(integer, n)))
	}
	zero, one, huge := uint64(0), uint64(1), uint64(math.MaxUint64)
	emptyText := testutil.Must(types.NewVarCharType(&zero, foundation.NonNull))
	byteText := testutil.Must(types.NewVarCharType(&one, foundation.NonNull))
	strings := []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(byteText, ""))}
	for n := 0; n < 256; n++ {
		strings = append(strings, testutil.Must(testbedvalues.NewVarChar(byteText, string([]byte{byte(n)}))))
	}
	for _, tc := range []struct {
		typ     types.Type
		entries []valueinterfaces.Value
	}{
		{integer, integers}, {emptyText, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(emptyText, ""))}}, {byteText, strings},
	} {
		full := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, tc.typ, tc.entries))
		excluded := testutil.Must(equatable.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, tc.typ, tc.entries))
		if !testutil.Must(full.IsUniversal()) || !testutil.Must(excluded.IsEmpty()) {
			t.Fatal("full finite domain missed")
		}
		partial := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, tc.typ, tc.entries[:len(tc.entries)-1]))
		if testutil.Must(partial.IsUniversal()) || testutil.Must(testutil.Must(partial.Complement()).IsEmpty()) {
			t.Fatal("partial domain treated as full")
		}
	}
	for _, typ := range []types.Type{
		testutil.Must(types.NewIntType(types.Int16, foundation.NonNull)), testutil.Must(types.NewIntType(types.Int32, foundation.NonNull)), testutil.Must(types.NewIntType(types.Int64, foundation.NonNull)),
		testutil.Must(types.NewFloatType(types.Float32, foundation.NonNull)), testutil.Must(types.NewFloatType(types.Float64, foundation.NonNull)),
		testutil.Must(types.NewVarCharType(nil, foundation.NonNull)), testutil.Must(types.NewVarCharType(&huge, foundation.NonNull)),
	} {
		empty := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
		all := testutil.Must(empty.Complement())
		if !testutil.Must(empty.IsEmpty()) || testutil.Must(empty.IsUniversal()) || !testutil.Must(all.IsUniversal()) || testutil.Must(all.IsEmpty()) {
			t.Fatal("empty/all on large domain")
		}
	}
	for _, bound := range []*uint64{nil, &huge} {
		typ := testutil.Must(types.NewVarCharType(bound, foundation.NonNull))
		set := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(typ, ""))}))
		if testutil.Must(set.IsUniversal()) || testutil.Must(testutil.Must(set.Complement()).IsEmpty()) {
			t.Fatal("large string domain covered by singleton")
		}
	}
}

func TestMembershipNormalizationAndErrors(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		nullable := testutil.Must(types.NewFloatType(width, foundation.MayNull))
		set := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewFloat(typ, 0)), testutil.Must(testbedvalues.NewFloat(typ, math.NaN())), testutil.Must(testbedvalues.NewFloat(typ, math.Inf(1)))}))
		complement := testutil.Must(set.Complement())
		for _, tc := range []struct {
			number float64
			want   bool
		}{{math.Copysign(0, -1), true}, {math.NaN(), true}, {math.Inf(1), true}, {math.Inf(-1), false}, {1, false}, {math.SmallestNonzeroFloat32, false}} {
			query := testutil.Must(testbedvalues.NewFloat(nullable, tc.number))
			if testutil.Must(set.Contains(query)) != tc.want || testutil.Must(complement.Contains(query)) == tc.want {
				t.Fatal("float membership normalization")
			}
			if query.Type().Nullability() != foundation.MayNull {
				t.Fatal("query mutated")
			}
		}
	}
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	nullable := testutil.Must(types.NewIntType(types.Int8, foundation.MayNull))
	set := testutil.Must(equatable.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewInteger(typ, 1))}))
	if testutil.Must(set.Contains(testutil.Must(testbedvalues.NewInteger(nullable, 1)))) || !testutil.Must(set.Contains(testutil.Must(testbedvalues.NewInteger(nullable, 2)))) {
		t.Fatal("nullable integer lookup")
	}
	for _, tc := range []struct {
		value valueinterfaces.Value
		want  error
	}{
		{testutil.Must(testbedvalues.NewNull(nullable)), value_set.ErrInvalidInput},

		{testutil.Must(testbedvalues.NewInteger(testutil.Must(types.NewIntType(types.Int16, foundation.NonNull)), 1)), value_set.ErrIncompatibleTypes},
		{testutil.Must(testbedvalues.NewBool(testutil.Must(types.NewBoolType(foundation.NonNull)), true)), value_set.ErrIncompatibleTypes},
	} {
		if _, err := set.Contains(tc.value); !errors.Is(err, tc.want) {
			t.Fatalf("got %v want %v", err, tc.want)
		}
	}
	for _, call := range []func(){
		func() { _, _ = set.Contains(nil) }, func() { _, _ = set.Contains((*testbedvalues.IntegerValue)(nil)) },
		func() {
			_, _ = (&equatable.EquatableValueSet{}).Contains(testutil.Must(testbedvalues.NewInteger(typ, 1)))
		},
		func() { _, _ = (&equatable.EquatableValueSet{}).Complement() }, func() { _, _ = (&equatable.EquatableValueSet{}).IsEmpty() }, func() { _, _ = (&equatable.EquatableValueSet{}).IsUniversal() }, func() { (&equatable.EquatableValueSet{}).IsInclusion() },
		func() { var absent *equatable.EquatableValueSet; _, _ = absent.Complement() },
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("expected assertion")
				}
			}()
			call()
		}()
	}
}

func TestVarCharMembershipAndBounds(t *testing.T) {
	limit := uint64(2)
	typ := testutil.Must(types.NewVarCharType(&limit, foundation.NonNull))
	nullable := testutil.Must(types.NewVarCharType(&limit, foundation.MayNull))
	set := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(typ, "a\x00")), testutil.Must(testbedvalues.NewVarChar(typ, "\xff"))}))
	for _, tc := range []struct {
		text string
		want bool
	}{{"a\x00", true}, {"\xff", true}, {"a", false}, {"", false}, {"A", false}} {
		query := testutil.Must(testbedvalues.NewVarChar(nullable, tc.text))
		if testutil.Must(set.Contains(query)) != tc.want || testutil.Must(testutil.Must(set.Complement()).Contains(query)) == tc.want {
			t.Fatal("incorrect bytewise membership")
		}
	}
	unbounded := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	if _, err := set.Contains(testutil.Must(testbedvalues.NewVarChar(unbounded, "a\x00"))); !errors.Is(err, value_set.ErrIncompatibleTypes) {
		t.Fatal("query bounds ignored", err)
	}
	if _, err := equatable.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewNull(nullable))}); !errors.Is(err, value_set.ErrInvalidInput) {
		t.Fatal("null exclusion accepted", err)
	}
}
