package ranges_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	valuefixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

func TestRangeFactoryIntegerNormalization(t *testing.T) {
	delegate := testbedvalues.ValueFactory{}
	factory := &valuefixtures.RecordingFactory{Delegate: delegate}
	factory.Integer = func(typ *types.IntType, n int64) (valueinterfaces.Value, error) {
		value, err := delegate.NewInteger(typ, n)
		if err != nil {
			return nil, err
		}
		return valuefixtures.WrapScalar(value), nil
	}
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.MayNull))
	low := valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewInteger(typ, 3)))
	high := valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewInteger(typ, 8)))
	lower := testutil.Must(ranges.NewRangeEndpoint(low, false))
	upper := testutil.Must(ranges.NewRangeEndpoint(high, false))
	interval, present := testutil.Must(ranges.NewRange(factory, typ, lower, upper)).Get()
	if !present || interval.String() != "[4,7]" || len(factory.Requests) != 2 {
		t.Fatal("exclusive bounds were not normalized through the factory")
	}
	for i, endpoint := range []ranges.RangeEndpoint{interval.Lower(), interval.Upper()} {
		value, _ := endpoint.Value().Get()
		if _, ok := value.(valuefixtures.ScalarView); !ok || !endpoint.IsInclusive() {
			t.Fatal("factory result representation lost")
		}
		request := factory.Requests[i]
		if request.Kind != "Integer" || request.Payload != []int64{4, 7}[i] || !request.Type.EqualsTo(interval.Type()) {
			t.Fatal("factory received wrong payload or normalized type")
		}
	}
	if low.Integer() != 3 || high.Integer() != 8 || low.Type().Nullability() != foundation.MayNull || lower.IsInclusive() {
		t.Fatal("normalization mutated its inputs")
	}
	// Failure after the first bound must not publish a partially built range.
	failure := errors.New("second bound failed")
	factory.Requests = nil
	factory.Integer = func(typ *types.IntType, n int64) (valueinterfaces.Value, error) {
		if n == 7 {
			return nil, failure
		}
		return delegate.NewInteger(typ, n)
	}
	result, err := ranges.NewRange(factory, typ, lower, upper)
	if _, present := result.Get(); present || !errors.Is(err, failure) || len(factory.Requests) != 2 {
		t.Fatal("partial range published or construction failure lost")
	}
}

func TestRangeFactoryFloatNormalization(t *testing.T) {
	delegate := testbedvalues.ValueFactory{}
	factory := &valuefixtures.RecordingFactory{Delegate: delegate}
	factory.Float = func(typ *types.FloatType, n float64) (valueinterfaces.Value, error) {
		value, err := delegate.NewFloat(typ, n)
		if err != nil {
			return nil, err
		}
		return valuefixtures.WrapScalar(value), nil
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		factory.Requests = nil
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		negative := valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1))))
		lower := testutil.Must(ranges.NewRangeEndpoint(negative, true))
		upper := testutil.Must(ranges.NewRangeEndpoint(testutil.Must(testbedvalues.NewFloat(typ, 1)), true))
		interval, present := testutil.Must(ranges.NewRange(factory, typ, lower, upper)).Get()
		if !present || len(factory.Requests) != 1 {
			t.Fatal("zero normalization bypassed factory")
		}
		request := factory.Requests[0]
		if request.Kind != "Float" || !request.Type.EqualsTo(typ) || request.Payload.(float64) != 0 || math.Signbit(request.Payload.(float64)) {
			t.Fatal("factory did not receive positive zero at the declared width")
		}
		value, _ := interval.Lower().Value().Get()
		if _, ok := value.(valuefixtures.ScalarView); !ok || math.Signbit(value.(valueinterfaces.FloatValue).Float()) || !math.Signbit(negative.Float()) {
			t.Fatal("normalization lost factory representation or mutated the source")
		}
		failure := errors.New("zero construction failed")
		factory.Failure = failure
		result, err := ranges.NewRange(factory, typ, lower, upper)
		if _, present := result.Get(); present || !errors.Is(err, failure) {
			t.Fatal("float factory failure did not propagate")
		}
		factory.Failure = nil
	}
}

func TestRangeFactoryExtremaAndPreconditions(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int64, foundation.NonNull))
	factory := &valuefixtures.RecordingFactory{Failure: errors.New("unexpected construction")}
	for _, pair := range [][2]ranges.RangeEndpoint{
		{testutil.IntEndpoint(typ, math.MaxInt64, false), {}},
		{{}, testutil.IntEndpoint(typ, math.MinInt64, false)},
	} {
		result, err := ranges.NewRange(factory, typ, pair[0], pair[1])
		if _, present := result.Get(); present || err != nil || len(factory.Requests) != 0 {
			t.Fatal("empty extrema attempted an overflowing construction")
		}
	}
	interval, present := testutil.Must(ranges.NewRange(factory, typ, testutil.IntEndpoint(typ, math.MinInt64, true), testutil.IntEndpoint(typ, math.MaxInt64, true))).Get()
	if !present || interval.String() != "(-∞,+∞)" || len(factory.Requests) != 0 {
		t.Fatal("unbounded extrema unexpectedly constructed values")
	}
	if !assertions.Enabled {
		return
	}
	for _, invalid := range []valueinterfaces.ValueFactory{nil, (*testbedvalues.ValueFactory)(nil)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("nil factory accepted even on a non-constructing path")
				}
			}()
			_, _ = ranges.NewRange(invalid, typ, ranges.RangeEndpoint{}, ranges.RangeEndpoint{})
		}()
	}
}
