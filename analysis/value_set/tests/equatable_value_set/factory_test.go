package equatable_value_set_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	valuefixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

func TestFiniteSetExternalPayloads(t *testing.T) {
	integer := testutil.Must(types.NewIntType(types.Int16, foundation.MayNull))
	boolean := testutil.Must(types.NewBoolType(foundation.MayNull))
	text := testutil.Must(types.NewVarCharType(nil, foundation.MayNull))
	floating := testutil.Must(types.NewFloatType(types.Float64, foundation.MayNull))
	for _, tc := range []struct {
		name                          string
		typ                           types.Type
		x, y, z                       valueinterfaces.Value
		wantA, wantUnion              string
		wantIntersect, wantDifference string
	}{
		{"integer", integer, testutil.Must(testbedvalues.NewInteger(integer, 1)), testutil.Must(testbedvalues.NewInteger(integer, 2)), testutil.Must(testbedvalues.NewInteger(integer, 3)), "{1, 2}", "{1, 2, 3}", "{2}", "{1}"},
		{"bool", boolean, testutil.Must(testbedvalues.NewBool(boolean, false)), testutil.Must(testbedvalues.NewBool(boolean, true)), testutil.Must(testbedvalues.NewBool(boolean, true)), "{false, true}", "{false, true}", "{true}", "{false}"},
		{"varchar", text, testutil.Must(testbedvalues.NewVarChar(text, "a")), testutil.Must(testbedvalues.NewVarChar(text, "b")), testutil.Must(testbedvalues.NewVarChar(text, "c")), "{\"a\", \"b\"}", "{\"a\", \"b\", \"c\"}", "{\"b\"}", "{\"a\"}"},
		{"float", floating, testutil.Must(testbedvalues.NewFloat(floating, math.Copysign(0, -1))), testutil.Must(testbedvalues.NewFloat(floating, 1)), testutil.Must(testbedvalues.NewFloat(floating, math.NaN())), "{0, 1}", "{0, 1, NaN}", "{1}", "{0}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory := testbedvalues.ValueFactory{}
			a := testutil.Must(finite.NewEquatableValueSet(factory, tc.typ, []valueinterfaces.Value{valuefixtures.WrapScalar(tc.y), tc.x, valuefixtures.WrapScalar(tc.x)}))
			b := testutil.Must(finite.NewEquatableValueSet(factory, tc.typ, []valueinterfaces.Value{valuefixtures.WrapScalar(tc.z), tc.y}))
			if a.String() != tc.wantA || testutil.Must(a.Union(b)).String() != tc.wantUnion || testutil.Must(a.Intersect(b)).String() != tc.wantIntersect || testutil.Must(a.Difference(b)).String() != tc.wantDifference {
				t.Fatal("mixed representations changed finite algebra or storage order")
			}
			if !testutil.Must(a.Contains(valuefixtures.WrapScalar(tc.x))) || !testutil.Must(a.Contains(tc.y)) || !testutil.Must(a.IsSubsetOf(testutil.Must(a.Union(b)))) {
				t.Fatal("mixed membership or subset failed")
			}
			for _, entry := range a.Values() {
				if entry.Type().Nullability() != foundation.NonNull {
					t.Fatal("stored type was not normalized")
				}
			}
			if tc.x.Type().Nullability() != foundation.MayNull {
				t.Fatal("input type was mutated")
			}
		})
	}
}

func TestFiniteDerivedSetsRetainReceiverFactory(t *testing.T) {
	typ := testutil.Must(types.NewFloatType(types.Float64, foundation.NonNull))
	zero := testutil.Must(testbedvalues.NewFloat(typ, 0))
	one := testutil.Must(testbedvalues.NewFloat(typ, 1))
	negative := valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1))))
	leftFactory := &valuefixtures.RecordingFactory{Delegate: testbedvalues.ValueFactory{}}
	failure := errors.New("right factory used")
	rightFactory := &valuefixtures.RecordingFactory{Failure: failure}
	for _, excludedA := range []bool{false, true} {
		for _, excludedB := range []bool{false, true} {
			var a value_set.ValueSet = testutil.Must(finite.NewEquatableValueSet(leftFactory, typ, []valueinterfaces.Value{zero, one}))
			var b value_set.ValueSet = testutil.Must(finite.NewEquatableValueSet(rightFactory, typ, []valueinterfaces.Value{one}))
			if excludedA {
				a = testutil.Must(a.Complement())
			}
			if excludedB {
				b = testutil.Must(b.Complement())
			}
			for _, result := range []value_set.ValueSet{
				testutil.Must(a.Union(b)), testutil.Must(a.Intersect(b)), testutil.Must(a.Difference(b)),
				testutil.Must(a.Complement()), testutil.Must(a.Difference(a)),
			} {
				before := len(leftFactory.Requests)
				testutil.Must(result.Contains(negative))
				testutil.Must(testutil.Must(result.Complement()).Contains(negative))
				if len(leftFactory.Requests) != before+2 || len(rightFactory.Requests) != 0 {
					t.Fatal("derived finite result lost receiver factory")
				}
			}
		}
	}
	// Filtering produces a finite result too; its later queries retain the owner.
	a := testutil.Must(finite.NewEquatableValueSet(leftFactory, typ, []valueinterfaces.Value{zero, one}))
	endpoint := testutil.Must(ranges.NewRangeEndpoint(zero, true))
	interval, _ := testutil.Must(ranges.NewRange(testbedvalues.ValueFactory{}, typ, endpoint, endpoint)).Get()
	ordered := testutil.Must(sorted_range_set.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{interval}, false))
	for _, result := range []value_set.ValueSet{testutil.Must(a.Intersect(ordered)), testutil.Must(a.Difference(ordered))} {
		before := len(leftFactory.Requests)
		testutil.Must(result.Contains(negative))
		if len(leftFactory.Requests) != before+1 {
			t.Fatal("filtered result lost factory")
		}
	}
	b := testutil.Must(finite.NewEquatableValueSet(rightFactory, typ, nil))
	if _, err := testutil.Must(b.Union(a)).Contains(negative); !errors.Is(err, failure) {
		t.Fatal("reverse operation did not preserve its receiver factory")
	}
}

func TestFiniteFactoryNormalizationAndErrors(t *testing.T) {
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
		typ := testutil.Must(types.NewFloatType(width, foundation.MayNull))
		negative := valuefixtures.WrapScalar(testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1))))
		input := []valueinterfaces.Value{negative}
		set := testutil.Must(finite.NewEquatableValueSet(factory, typ, input))
		stored := set.Values()[0]
		if _, ok := stored.(valuefixtures.ScalarView); !ok || math.Signbit(stored.(valueinterfaces.FloatValue).Float()) || !math.Signbit(negative.Float()) {
			t.Fatal("factory representation lost or source zero mutated")
		}
		if len(factory.Requests) != 1 || !factory.Requests[0].Type.EqualsTo(set.Type()) || math.Signbit(factory.Requests[0].Payload.(float64)) {
			t.Fatal("factory received wrong normalized type or payload")
		}
		failure := errors.New("normalization failed")
		factory.Failure = failure
		if result, err := finite.NewEquatableValueSet(factory, typ, input); result != nil || !errors.Is(err, failure) {
			t.Fatal("failed normalization published a set or lost the error")
		}
		if _, err := set.Contains(negative); !errors.Is(err, failure) || set.String() != "{0}" || !math.Signbit(negative.Float()) {
			t.Fatal("membership failure lost error or mutated inputs")
		}
		factory.Failure = nil
	}
	if !assertions.Enabled {
		return
	}
	typ := testutil.Must(types.NewBoolType(foundation.NonNull))
	for _, invalid := range []valueinterfaces.ValueFactory{nil, (*testbedvalues.ValueFactory)(nil)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("nil factory accepted for empty finite set")
				}
			}()
			_, _ = finite.NewEquatableValueSet(invalid, typ, nil)
		}()
	}
}
