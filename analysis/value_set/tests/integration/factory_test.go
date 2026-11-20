package integration_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	sorted "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

// A factory need not be comparable; dispatch must never compare factory identities.
type nonComparableFactory struct {
	*fixtures.RecordingFactory
	unused []byte
}

func TestMixedOperationsRetainOriginalFactory(t *testing.T) {
	typ := testutil.Must(types.NewFloatType(types.Float64, foundation.NonNull))
	zero := fixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, 0)))
	negative := fixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1))))
	one := fixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, 1)))
	build := func(factory valueinterfaces.ValueFactory, kind string) value_set.ValueSet {
		switch kind {
		case "finite":
			return testutil.Must(finite.NewEquatableValueSet(factory, typ, []valueinterfaces.Value{zero, one}))
		case "excluded":
			return testutil.Must(finite.NewExcludedEquatableValueSet(factory, typ, []valueinterfaces.Value{zero, one}))
		case "empty":
			return testutil.Must(sorted.NewSortedRangeSet(factory, typ, nil, false))
		case "universal":
			return testutil.Must(testutil.Must(sorted.NewSortedRangeSet(factory, typ, nil, false)).Complement())
		default:
			interval := testutil.RequiredRange(t, typ, testutil.FloatEndpoint(typ, 0, true), testutil.FloatEndpoint(typ, 1, true))
			return testutil.Must(sorted.NewSortedRangeSet(factory, typ, []*ranges.Range{interval}, false))
		}
	}
	operations := map[string]func(value_set.ValueSet, value_set.ValueSet) (value_set.ValueSet, error){
		"union": value_set.ValueSet.Union, "intersect": value_set.ValueSet.Intersect, "difference": value_set.ValueSet.Difference,
	}
	kinds := []string{"finite", "excluded", "empty", "universal", "range"}
	for _, leftKind := range kinds {
		for _, rightKind := range kinds {
			for name, operation := range operations {
				t.Run(leftKind+"/"+rightKind+"/"+name, func(t *testing.T) {
					left := &fixtures.RecordingFactory{Delegate: testbedvalues.ValueFactory{}, Float: func(typ *types.FloatType, n float64) (valueinterfaces.Value, error) {
						return fixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, n))), nil
					}}
					right := &fixtures.RecordingFactory{Delegate: testbedvalues.ValueFactory{}}
					a, b := build(nonComparableFactory{RecordingFactory: left}, leftKind), build(right, rightKind)
					left.Requests, right.Requests = nil, nil
					right.Failure = errors.New("used other operand's factory")
					result := testutil.Must(operation(a, b))
					baseline := testutil.Must(operation(build(testbedvalues.ValueFactory{}, leftKind), build(testbedvalues.ValueFactory{}, rightKind)))
					if result.String() != baseline.String() {
						t.Fatalf("got %s, want %s", result, baseline)
					}
					// Includes empty, universal and finite filtering results. Force synthesis
					// after the original operation, even if its result needed no allocation.
					before := len(left.Requests)
					switch r := result.(type) {
					case *finite.EquatableValueSet:
						testutil.Must(r.Contains(negative))
					case *sorted.SortedRangeSet:
						cleared := testutil.Must(r.WithoutPoints([]valueinterfaces.Value{zero}))
						restored := testutil.Must(cleared.WithPoints([]valueinterfaces.Value{negative}))
						if !testutil.Must(restored.Contains(zero)) {
							t.Fatal("lost inserted zero")
						}
					default:
						t.Fatalf("unexpected result %T", result)
					}
					if len(left.Requests) <= before || len(right.Requests) != 0 {
						t.Fatal("result lost original factory")
					}
					// Query-only operations must also retain ownership, including EqualsTo's
					// reverse difference. Sources keep their original factory after dispatch.
					testutil.Must(a.EqualsTo(b))
					testutil.Must(a.IsSubsetOf(b))
					testutil.Must(a.IsDisjointFrom(b))
					if len(right.Requests) != 0 {
						t.Fatal("query used right factory")
					}
					right.Failure = nil
					before = len(right.Requests)
					switch r := b.(type) {
					case *finite.EquatableValueSet:
						testutil.Must(r.Contains(negative))
					case *sorted.SortedRangeSet:
						testutil.Must(testutil.Must(r.WithoutPoints([]valueinterfaces.Value{zero})).WithPoints([]valueinterfaces.Value{negative}))
					}
					if len(right.Requests) <= before {
						t.Fatal("source factory was mutated")
					}
				})
			}
		}
	}
}

func TestSortedFactoryReverseDifferenceAndFailures(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int16, foundation.NonNull))
	interval := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, 2, true), testutil.IntEndpoint(typ, 4, true))
	left := &fixtures.RecordingFactory{Delegate: testbedvalues.ValueFactory{}}
	sentinel := errors.New("factory failed")
	right := &fixtures.RecordingFactory{Failure: sentinel}
	a := testutil.Must(sorted.NewSortedRangeSet(left, typ, []*ranges.Range{interval}, false))
	b := testutil.Must(sorted.NewSortedRangeSet(right, typ, []*ranges.Range{interval}, false))
	if !testutil.Must(a.EqualsTo(b)) || len(left.Requests) == 0 || len(right.Requests) != 0 {
		t.Fatal("reverse difference changed ownership")
	}
	left.Failure = sentinel
	for name, op := range map[string]func() (value_set.ValueSet, error){
		"complement": a.Complement,
		"difference": func() (value_set.ValueSet, error) { return a.Difference(b) },
		"split": func() (value_set.ValueSet, error) {
			r, err := a.WithoutPoints([]valueinterfaces.Value{testutil.Must(testbedvalues.NewInteger(typ, 3))})
			if err != nil {
				return nil, err
			}
			return r, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := op()
			if result != nil || !errors.Is(err, sentinel) {
				t.Fatalf("result=%v error=%v", result, err)
			}
		})
	}
	if a.String() != b.String() {
		t.Fatal("failed operation mutated input")
	}
}

func TestMixedConversionUsesSelectedFactory(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int16, foundation.NonNull))
	interval := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, 2, true), testutil.IntEndpoint(typ, 4, true))
	left := &fixtures.RecordingFactory{Delegate: testbedvalues.ValueFactory{}, Integer: func(typ *types.IntType, n int64) (valueinterfaces.Value, error) {
		return fixtures.WrapScalar(testutil.Must(testbedvalues.NewInteger(typ, n))), nil
	}}
	sentinel := errors.New("selected factory failure")
	right := &fixtures.RecordingFactory{Failure: errors.New("wrong factory")}
	build := func(factory valueinterfaces.ValueFactory, excluded bool) value_set.ValueSet {
		if excluded {
			return testutil.Must(finite.NewExcludedEquatableValueSet(factory, typ, []valueinterfaces.Value{fixtures.WrapScalar(testutil.Must(testbedvalues.NewInteger(typ, 3)))}))
		}
		return testutil.Must(sorted.NewSortedRangeSet(factory, typ, []*ranges.Range{interval}, false))
	}
	for _, excluded := range []bool{false, true} {
		a, b := build(left, excluded), build(right, !excluded)
		for name, operation := range map[string]func(value_set.ValueSet, value_set.ValueSet) (value_set.ValueSet, error){"union": value_set.ValueSet.Union, "intersection": value_set.ValueSet.Intersect, "difference": value_set.ValueSet.Difference} {
			t.Run(name+"/"+a.String(), func(t *testing.T) {
				// Sorted minus excluded finite returns a filtered finite set
				// without synthesis. Convert and complement it to check retention.
				if !excluded && name == "difference" {
					original := operation
					operation = func(a, b value_set.ValueSet) (value_set.ValueSet, error) {
						result, err := original(a, b)
						if err != nil {
							return nil, err
						}
						converted, err := result.(*finite.EquatableValueSet).ToSortedRangeSet()
						if err != nil {
							return nil, err
						}
						return converted.Complement()
					}
				}
				left.Requests = nil
				result := testutil.Must(operation(a, b))
				if len(left.Requests) == 0 || len(right.Requests) != 0 {
					t.Fatal("mixed conversion/split used wrong factory")
				}
				baseline := testutil.Must(operation(build(testbedvalues.ValueFactory{}, excluded), build(testbedvalues.ValueFactory{}, !excluded)))
				if result.String() != baseline.String() {
					t.Fatalf("got %s, want %s", result, baseline)
				}
				left.Failure = sentinel
				failed, err := operation(a, b)
				if failed != nil || !errors.Is(err, sentinel) {
					t.Fatalf("partial result or lost error: %v, %v", failed, err)
				}
				left.Failure = nil
			})
		}
	}
}
