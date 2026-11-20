// Package testutil shares fixtures and independent oracles across value-set test suites.
package testutil

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Must unwraps successful fixture construction and panics on a setup error.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// IntEndpoint constructs an integer endpoint for a test fixture.
func IntEndpoint(typ *types.IntType, number int64, inclusive bool) ranges.RangeEndpoint {
	return Must(ranges.NewRangeEndpoint(Must(testbedvalues.NewInteger(typ, number)), inclusive))
}

// FloatEndpoint constructs a float endpoint for a test fixture.
func FloatEndpoint(typ *types.FloatType, n float64, inclusive bool) ranges.RangeEndpoint {
	return Must(ranges.NewRangeEndpoint(Must(testbedvalues.NewFloat(typ, n)), inclusive))
}

// TextEndpoint constructs a VARCHAR endpoint for a test fixture.
func TextEndpoint(typ *types.VarCharType, text string, inclusive bool) ranges.RangeEndpoint {
	return Must(ranges.NewRangeEndpoint(Must(testbedvalues.NewVarChar(typ, text)), inclusive))
}

// RequiredRange constructs a range and fails the test if it is empty.
func RequiredRange(t *testing.T, typ types.Type, lower, upper ranges.RangeEndpoint) *ranges.Range {
	t.Helper()
	r, present := Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, lower, upper)).Get()
	if !present {
		t.Fatal("expected nonempty range")
	}
	return r
}
