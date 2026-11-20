package sorted_range_set_test

import (
	"errors"
	"math"
	"testing"

	sorted "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

func TestSortedExternalNaNAndZero(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		factory := &fixtures.RecordingFactory{Delegate: testbedvalues.ValueFactory{}, Float: func(typ *types.FloatType, n float64) (valueinterfaces.Value, error) {
			return fixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, n))), nil
		}}
		nan := fixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, math.NaN())))
		negative := fixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1))))
		empty := testutil.Must(sorted.NewSortedRangeSet(factory, typ, nil, false))
		added := testutil.Must(empty.WithPoints([]valueinterfaces.Value{nan, negative}))
		if !testutil.Must(added.Contains(nan)) || !testutil.Must(added.Contains(negative)) || len(factory.Requests) == 0 {
			t.Fatal("external float insertion failed")
		}
		for _, r := range added.Ranges() {
			for _, endpoint := range []ranges.RangeEndpoint{r.Lower(), r.Upper()} {
				v, ok := endpoint.Value().Get()
				if !ok {
					continue
				}
				if _, ok := v.(fixtures.ScalarView); !ok || math.Signbit(v.(valueinterfaces.FloatValue).Float()) {
					t.Fatal("factory representation or positive zero lost")
				}
			}
		}
		removed := testutil.Must(added.WithoutPoints([]valueinterfaces.Value{nan, negative}))
		if !testutil.Must(removed.IsEmpty()) || testutil.Must(testutil.Must(added.Complement()).Contains(nan)) {
			t.Fatal("NaN removal/complement failed")
		}
		sentinel := errors.New("zero construction failed")
		factory.Failure = sentinel
		if result, err := empty.WithPoints([]valueinterfaces.Value{negative}); result != nil || !errors.Is(err, sentinel) {
			t.Fatal("factory failure lost")
		}
	}
}

func TestSortedConstructorFactoryAndPreconditions(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int16, foundation.NonNull))
	a := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, 1, true), testutil.IntEndpoint(typ, 2, true))
	b := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, 3, true), testutil.IntEndpoint(typ, 4, true))
	sentinel := errors.New("gap construction failed")
	factory := &fixtures.RecordingFactory{Failure: sentinel}
	if result, err := sorted.NewSortedRangeSet(factory, typ, []*ranges.Range{a, b}, false); result != nil || !errors.Is(err, sentinel) {
		t.Fatal("constructor did not use supplied factory")
	}
	if !assertions.Enabled {
		return
	}
	for _, invalid := range []valueinterfaces.ValueFactory{nil, (*testbedvalues.ValueFactory)(nil)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("nil factory accepted")
				}
			}()
			_, _ = sorted.NewSortedRangeSet(invalid, typ, nil, false)
		}()
	}
}
