package ranges_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	ranges "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestVarCharRangesAgainstBoundedUniverses(t *testing.T) {
	for _, limit := range []uint64{0, 1} {
		typ := testutil.Must(types.NewVarCharType(&limit, foundation.NonNull))
		universe := []string{""}
		if limit == 1 {
			for i := 0; i < 256; i++ {
				universe = append(universe, string([]byte{byte(i)}))
			}
		}
		endpoints := []ranges.RangeEndpoint{{}}
		selected := []string{""}
		if limit == 1 {
			selected = append(selected, "\x00", "\x01", "a", "b", "\x7f", "\x80", "\xfe", "\xff")
		}
		for _, text := range selected {
			for _, inclusive := range []bool{true, false} {
				endpoints = append(endpoints, testutil.TextEndpoint(typ, text, inclusive))
			}
		}
		for _, lower := range endpoints {
			for _, upper := range endpoints {
				result, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, lower, upper)).Get()
				count := 0
				for _, text := range universe {
					want := testutil.AcceptsText(lower, upper, text)
					if want {
						count++
					}
					got := present && testutil.AcceptsText(result.Lower(), result.Upper(), text)
					if got != want {
						t.Fatalf("limit=%d text=%q membership differs", limit, text)
					}
				}
				if present != (count > 0) {
					t.Fatal("incorrect bounded emptiness")
				}
			}
		}
	}
}

func TestVarCharGapsAndBounds(t *testing.T) {
	one, two, huge := uint64(1), uint64(2), uint64(math.MaxUint64)
	for _, tc := range []struct {
		bound                                *uint64
		low, high                            string
		lowInclusive, highInclusive, present bool
	}{
		{nil, "a", "a\x00", false, false, false},
		{nil, "a", "a\x00", true, false, true},
		{nil, "a", "a\x00\x00", false, false, true},
		{&one, "a", "b", false, false, false},
		{&two, "a", "b", false, false, true},
		{nil, "a", "b", false, false, true},
		{&two, "a\xff", "b", false, false, false},
		{&two, "a\xfe", "a\xff", false, false, false},
		{&two, "a\xfd", "a\xff", false, false, true},
		{nil, "a\xff", "b", false, false, true},
		{&huge, "a", "b", false, false, true},
		{nil, "\x7f", "\x80", false, false, true},
		{&one, "\x7f", "\x80", false, false, false},
		{nil, "b", "a", true, true, false},
		{nil, "a", "a", true, true, true},
		{nil, "a", "a", false, true, false},
	} {
		typ := testutil.Must(types.NewVarCharType(tc.bound, foundation.MayNull))
		lower, upper := testutil.TextEndpoint(typ, tc.low, tc.lowInclusive), testutil.TextEndpoint(typ, tc.high, tc.highInclusive)
		result, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, lower, upper)).Get()
		if present != tc.present {
			t.Fatalf("(%q,%q) bound=%v present=%v want=%v", tc.low, tc.high, tc.bound, present, tc.present)
		}
		if present {
			if result.Type().Nullability() != foundation.NonNull || typ.Nullability() != foundation.MayNull {
				t.Fatal("wrong normalized type")
			}
			if result.Lower().IsInclusive() != tc.lowInclusive || result.Upper().IsInclusive() != tc.highInclusive {
				t.Fatal("ordinary VARCHAR cuts changed")
			}
		}
	}
	for _, bound := range []*uint64{nil, &two, &huge} {
		typ := testutil.Must(types.NewVarCharType(bound, foundation.NonNull))
		unbounded := ranges.RangeEndpoint{}
		if _, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, unbounded, testutil.TextEndpoint(typ, "", false))).Get(); present {
			t.Fatal("values below minimum")
		}
		all, _ := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, testutil.TextEndpoint(typ, "", true), unbounded)).Get()
		if !all.Lower().IsUnbounded() || !all.Upper().IsUnbounded() {
			t.Fatal("minimum not normalized")
		}
	}
	bounded := testutil.Must(types.NewVarCharType(&two, foundation.NonNull))
	maximum := testutil.TextEndpoint(bounded, "\xff\xff", true)
	full, _ := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, bounded, ranges.RangeEndpoint{}, maximum)).Get()
	if !full.Upper().IsUnbounded() {
		t.Fatal("maximum not normalized")
	}
	if _, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, bounded, testutil.TextEndpoint(bounded, "\xff\xff", false), ranges.RangeEndpoint{})).Get(); present {
		t.Fatal("values above maximum")
	}
	unbounded := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	if _, err := ranges.NewRange(testbedvalues.ValueFactory{}, bounded, testutil.TextEndpoint(bounded, "\xff\xff", false), testutil.TextEndpoint(unbounded, "a", true)); !errors.Is(err, value_set.ErrIncompatibleTypes) {
		t.Fatal("empty side hid incompatible bound", err)
	}
}
