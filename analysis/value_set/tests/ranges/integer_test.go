package ranges_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	ranges "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func acceptsInteger(lower, upper ranges.RangeEndpoint, number int64) bool {
	if value, present := lower.Value().Get(); present {
		bound := value.(*testbedvalues.IntegerValue).Integer()
		if number < bound || (number == bound && !lower.IsInclusive()) {
			return false
		}
	}
	if value, present := upper.Value().Get(); present {
		bound := value.(*testbedvalues.IntegerValue).Integer()
		if number > bound || (number == bound && !upper.IsInclusive()) {
			return false
		}
	}
	return true
}

func TestIntegerRangeAgainstInt8Universe(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	endpoints := []ranges.RangeEndpoint{{}}
	for _, number := range []int64{-128, -127, -3, -2, -1, 0, 1, 2, 3, 126, 127} {
		for _, inclusive := range []bool{false, true} {
			endpoints = append(endpoints, testutil.IntEndpoint(typ, number, inclusive))
		}
	}
	for _, lower := range endpoints {
		for _, upper := range endpoints {
			optional := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, lower, upper))
			result, present := optional.Get()
			count := 0
			for n := int64(-128); n <= 127; n++ {
				want := acceptsInteger(lower, upper, n)
				if want {
					count++
				}
				got := present && acceptsInteger(result.Lower(), result.Upper(), n)
				if got != want {
					t.Fatalf("membership differs at %d for lower=%v upper=%v", n, lower, upper)
				}
			}
			if present != (count != 0) {
				t.Fatal("incorrect empty-range result")
			}
			if present {
				for _, bound := range []ranges.RangeEndpoint{result.Lower(), result.Upper()} {
					if value, bounded := bound.Value().Get(); bounded {
						if !bound.IsInclusive() || !value.Type().EqualsTo(result.Type()) {
							t.Fatal("endpoint not normalized")
						}
					}
				}
			}
		}
	}
}

func TestIntegerRangeExtremaAndNormalization(t *testing.T) {
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		typ := testutil.Must(types.NewIntType(width, foundation.MayNull))
		max := int64(math.MaxInt64)
		if width != types.Int64 {
			max = int64(1)<<(width-1) - 1
		}
		min := -max - 1
		unbounded := ranges.RangeEndpoint{}
		for _, tc := range []struct {
			lower, upper                ranges.RangeEndpoint
			present                     bool
			low, high                   int64
			unboundedLow, unboundedHigh bool
		}{
			{unbounded, unbounded, true, 0, 0, true, true},
			{testutil.IntEndpoint(typ, min, true), testutil.IntEndpoint(typ, max, true), true, 0, 0, true, true},
			{testutil.IntEndpoint(typ, min, false), testutil.IntEndpoint(typ, max, false), true, min + 1, max - 1, false, false},
			{testutil.IntEndpoint(typ, max, false), unbounded, false, 0, 0, false, false},
			{unbounded, testutil.IntEndpoint(typ, min, false), false, 0, 0, false, false},
			{testutil.IntEndpoint(typ, max, true), unbounded, true, max, 0, false, true},
			{unbounded, testutil.IntEndpoint(typ, min, true), true, 0, min, true, false},
			{testutil.IntEndpoint(typ, 3, false), testutil.IntEndpoint(typ, 4, false), false, 0, 0, false, false},
			{testutil.IntEndpoint(typ, 3, true), testutil.IntEndpoint(typ, 4, false), true, 3, 3, false, false},
			{testutil.IntEndpoint(typ, 4, true), testutil.IntEndpoint(typ, 3, true), false, 0, 0, false, false},
			{testutil.IntEndpoint(typ, 3, true), testutil.IntEndpoint(typ, 3, true), true, 3, 3, false, false},
		} {
			result, present := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, tc.lower, tc.upper)).Get()
			if present != tc.present {
				t.Fatalf("width=%d presence=%v want=%v", width, present, tc.present)
			}
			if !present {
				continue
			}
			if result.Type().Nullability() != foundation.NonNull || typ.Nullability() != foundation.MayNull {
				t.Fatal("nullability normalization mutated input")
			}
			for _, bound := range []struct {
				endpoint  ranges.RangeEndpoint
				unbounded bool
				number    int64
			}{{result.Lower(), tc.unboundedLow, tc.low}, {result.Upper(), tc.unboundedHigh, tc.high}} {
				if bound.endpoint.IsUnbounded() != bound.unbounded {
					t.Fatal("wrong unbounded side")
				}
				if value, has := bound.endpoint.Value().Get(); has && (value.(*testbedvalues.IntegerValue).Integer() != bound.number || !bound.endpoint.IsInclusive()) {
					t.Fatal("wrong normalized boundary")
				}
			}
		}
	}
}

func TestEndpointOwnershipAndValidation(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	original := testutil.Must(testbedvalues.NewInteger(typ, 3))
	bound := testutil.Must(ranges.NewRangeEndpoint(original, true))
	stored, _ := bound.Value().Get()
	if stored != original {
		t.Fatal("immutable non-null value not shared")
	}
	nullable := testutil.Must(original.WithNullability(foundation.MayNull))
	normalized := testutil.Must(ranges.NewRangeEndpoint(nullable, false))
	stored, _ = normalized.Value().Get()
	if stored.Type().Nullability() != foundation.NonNull || nullable.Type().Nullability() != foundation.MayNull {
		t.Fatal("endpoint normalization")
	}
	rangeValue, _ := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, bound, testutil.IntEndpoint(typ, 4, true))).Get()
	returned := rangeValue.Lower()
	if returned.IsUnbounded() {
		t.Fatal("expected bounded lower side")
	}
	returned = ranges.RangeEndpoint{}
	if !returned.IsUnbounded() || rangeValue.Lower().IsUnbounded() {
		t.Fatal("endpoint escaped by mutable reference")
	}
	if !bound.IsInclusive() {
		t.Fatal("source bound changed")
	}
	for _, value := range []valueinterfaces.Value{testutil.Must(testbedvalues.NewNull(testutil.Must(typ.WithNullability(foundation.MayNull))))} {
		if _, err := ranges.NewRangeEndpoint(value, true); !errors.Is(err, value_set.ErrInvalidInput) {
			t.Fatal("bad endpoint admitted", err)
		}
	}
	if (ranges.RangeEndpoint{}).IsInclusive() {
		t.Fatal("unbounded zero marked inclusive")
	}
}

func TestRangeErrorsAndPreconditions(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	mismatched := testutil.IntEndpoint(testutil.Must(types.NewIntType(types.Int16, foundation.NonNull)), 0, true)
	if optional, err := ranges.NewRange(testbedvalues.ValueFactory{}, typ, testutil.IntEndpoint(typ, 127, false), mismatched); !errors.Is(err, value_set.ErrIncompatibleTypes) {
		t.Fatal("empty shortcut hid type error", err)
	} else if _, present := optional.Get(); present {
		t.Fatal("error returned a range")
	}
	for _, tc := range []struct {
		typ  types.Type
		want error
	}{
		{testutil.Must(types.NewBoolType(foundation.NonNull)), value_set.ErrUnsupported},
		{testutil.Must(types.NewArrayType(typ, foundation.NonNull)), value_set.ErrUnsupported},
	} {
		optional, err := ranges.NewRange(testbedvalues.ValueFactory{}, tc.typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})
		if !errors.Is(err, tc.want) {
			t.Fatalf("got %v want %v", err, tc.want)
		}
		if _, present := optional.Get(); present {
			t.Fatal("range returned on error")
		}
	}
	for _, call := range []func(){
		func() {
			_, _ = ranges.NewRange(testbedvalues.ValueFactory{}, nil, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})
		},
		func() {
			_, _ = ranges.NewRange(testbedvalues.ValueFactory{}, (*types.IntType)(nil), ranges.RangeEndpoint{}, ranges.RangeEndpoint{})
		},
		func() { _, _ = ranges.NewRangeEndpoint(nil, true) },
		func() { _, _ = ranges.NewRangeEndpoint((*testbedvalues.IntegerValue)(nil), true) },
		func() { (&ranges.Range{}).Type() }, func() { (&ranges.Range{}).Lower() }, func() { (&ranges.Range{}).Upper() }, func() { var r *ranges.Range; r.Type() },
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("missing precondition assertion")
				}
			}()
			call()
		}()
	}
}
