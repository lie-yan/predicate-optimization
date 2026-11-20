package ranges

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func rangeTestMust[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestVarCharAdjacency(t *testing.T) {
	limit := uint64(1)
	typ := rangeTestMust(types.NewVarCharType(&limit, foundation.NonNull))
	universe := []string{""}
	for b := 0; b < 256; b++ {
		universe = append(universe, string([]byte{byte(b)}))
	}
	for i, a := range universe {
		for j, b := range universe {
			if varCharAdjacent(typ, a, b) != (j == i+1) {
				t.Fatalf("wrong adjacency %q %q", a, b)
			}
		}
	}
	two, huge := uint64(2), uint64(math.MaxUint64)
	for _, tc := range []struct {
		bound *uint64
		a, b  string
		want  bool
	}{
		{nil, "", "\x00", true},
		{nil, "a", "a\x00", true},
		{nil, "a", "b", false},
		{&two, "a\xff", "b", true},
		{&two, "a\xff", "b\x00", false},
		{&two, "a", "b", false},
		{&two, "\xff\xff", "", false},
		{&huge, "a", "a\x00", true},
	} {
		typ := rangeTestMust(types.NewVarCharType(tc.bound, foundation.NonNull))
		if varCharAdjacent(typ, tc.a, tc.b) != tc.want {
			t.Fatal("wrong prefix/carry adjacency", tc)
		}
	}
}

func TestRangeEndpointComparisons(t *testing.T) {
	typ := rangeTestMust(types.NewVarCharType(nil, foundation.NonNull))
	ep := func(s string, inclusive bool) RangeEndpoint {
		return rangeTestMust(NewRangeEndpoint(rangeTestMust(testbedvalues.NewVarChar(typ, s)), inclusive))
	}
	// Arrays specify the expected lower-bound and upper-bound order independently.
	for _, tc := range []struct {
		ordered []RangeEndpoint
		compare func(RangeEndpoint, RangeEndpoint) (int, error)
	}{
		{[]RangeEndpoint{{}, ep("", true), ep("", false), ep("a", true), ep("a", false), ep("a\x00", true)}, CompareLowerEndpoints},
		{[]RangeEndpoint{ep("", false), ep("", true), ep("a", false), ep("a", true), ep("a\x00", false), {}}, CompareUpperEndpoints},
	} {
		for i, a := range tc.ordered {
			for j, b := range tc.ordered {
				got := rangeTestMust(tc.compare(a, b))
				want := 0
				if i < j {
					want = -1
				}
				if i > j {
					want = 1
				}
				if got != want {
					t.Fatalf("endpoint order: got %d want %d", got, want)
				}
			}
		}
	}
	integer := rangeTestMust(types.NewIntType(types.Int64, foundation.NonNull))
	lo := rangeTestMust(testbedvalues.NewInteger(integer, math.MinInt64))
	hi := rangeTestMust(testbedvalues.NewInteger(integer, math.MaxInt64))
	if rangeTestMust(compareRangeValues(lo, hi)) != -1 {
		t.Fatal("integer comparison overflow")
	}
	f := rangeTestMust(testbedvalues.NewFloat(rangeTestMust(types.NewFloatType(types.Float64, foundation.NonNull)), 1))
	if order, err := compareRangeValues(f, f); err != nil || order != 0 {
		t.Fatal("float ordering", order, err)
	}
	limit := uint64(1)
	bounded := rangeTestMust(testbedvalues.NewVarChar(rangeTestMust(types.NewVarCharType(&limit, foundation.NonNull)), "a"))
	if _, err := compareRangeValues(rangeTestMust(testbedvalues.NewVarChar(typ, "a")), bounded); !errors.Is(err, value_set.ErrIncompatibleTypes) {
		t.Fatal("incompatible bounds", err)
	}
}
