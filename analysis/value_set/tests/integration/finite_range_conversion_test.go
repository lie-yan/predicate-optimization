package integration_test

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set/tests/internal/testutil"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func verifyRangeConversion(t *testing.T, source *finite.EquatableValueSet, probes []valueinterfaces.Value, expected []bool) *sets.SortedRangeSet {
	t.Helper()
	before := source.Values()
	polarity := source.IsInclusion()
	converted := testutil.Must(source.ToSortedRangeSet())
	for i, v := range probes {
		if testutil.Must(converted.Contains(v)) != expected[i] {
			t.Fatalf("conversion changed membership at %d", i)
		}
	}
	if !converted.Type().EqualsTo(source.Type()) || !testutil.Must(converted.EqualsTo(source)) || !testutil.Must(source.EqualsTo(converted)) {
		t.Fatal("conversion changed domain or region")
	}
	if testutil.Must(converted.IsEmpty()) != testutil.Must(source.IsEmpty()) || testutil.Must(converted.IsUniversal()) != testutil.Must(source.IsUniversal()) {
		t.Fatal("conversion changed universe query")
	}
	if !slices.Equal(before, source.Values()) || source.IsInclusion() != polarity {
		t.Fatal("conversion modified source")
	}
	// Accessors must not leak mutable storage; immutable endpoint Values can be shared.
	output := converted.Ranges()
	if len(output) > 0 {
		output[0] = nil
		if converted.Ranges()[0] == nil {
			t.Fatal("range slice escaped")
		}
	}
	entries := source.Values()
	if len(entries) > 0 {
		entries[0] = nil
		if source.Values()[0] == nil {
			t.Fatal("entry slice escaped")
		}
	}
	return converted
}

func TestFiniteRangeConversionAndMixedInt8Algebra(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.MayNull))
	universe := make([]valueinterfaces.Value, 256)
	for i := range universe {
		universe[i] = testutil.Must(testbedvalues.NewInteger(typ, int64(i)-128))
	}
	inputs := testutil.EnumerateSets(typ, universe, []int{0, 127, 128, 255})
	mixed := append([]testutil.EnumeratedSet(nil), inputs...)
	for _, input := range inputs {
		converted := verifyRangeConversion(t, input.Set.(*finite.EquatableValueSet), universe, input.Members)
		mixed = append(mixed, testutil.EnumeratedSet{Set: converted, Members: input.Members})
	}
	for _, pair := range [][2]int64{{-128, 0}, {-3, 3}, {0, 127}} {
		r := testutil.RequiredRange(t, typ, testutil.IntEndpoint(typ, pair[0], true), testutil.IntEndpoint(typ, pair[1], true))
		region := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, typ, []*ranges.Range{r}, false))
		members := make([]bool, len(universe))
		for i := range members {
			n := int64(i) - 128
			members[i] = pair[0] <= n && n <= pair[1]
		}
		mixed = append(mixed, testutil.EnumeratedSet{Set: region, Members: members})
	}
	testutil.VerifyBinaryAlgebra(t, universe, mixed)
}

func TestFiniteRangeConversionAndMixedVarCharAlgebra(t *testing.T) {
	for _, limit := range []uint64{0, 1} {
		typ := testutil.Must(types.NewVarCharType(&limit, foundation.NonNull))
		universe := []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(typ, ""))}
		chosen := []int{0}
		if limit == 1 {
			for n := 0; n < 256; n++ {
				universe = append(universe, testutil.Must(testbedvalues.NewVarChar(typ, string([]byte{byte(n)}))))
			}
			chosen = append(chosen, 1, 98, 256)
		}
		inputs := testutil.EnumerateSets(typ, universe, chosen)
		mixed := append([]testutil.EnumeratedSet(nil), inputs...)
		for _, input := range inputs {
			converted := verifyRangeConversion(t, input.Set.(*finite.EquatableValueSet), universe, input.Members)
			mixed = append(mixed, testutil.EnumeratedSet{Set: converted, Members: input.Members})
		}
		testutil.VerifyBinaryAlgebra(t, universe, mixed)
		allMembers := make([]bool, len(universe))
		for i := range allMembers {
			allMembers[i] = true
		}
		verifyRangeConversion(t, testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, universe)), universe, allMembers)
		verifyRangeConversion(t, testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, universe)), universe, make([]bool, len(universe)))
	}
}

func TestFiniteRangeConversionFloatIdentity(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		numbers := []float64{math.Inf(-1), 0, 1, math.Inf(1), math.NaN(), 2}
		universe := make([]valueinterfaces.Value, len(numbers))
		for i, n := range numbers {
			universe[i] = testutil.Must(testbedvalues.NewFloat(typ, n))
		}
		// Every set names only the first five identities. The extra value witnesses
		// the common outside region, making the relation oracle exact for these sets.
		inputs := testutil.EnumerateSets(typ, universe, []int{0, 1, 2, 3, 4})
		mixed := append([]testutil.EnumeratedSet(nil), inputs...)
		for _, input := range inputs {
			converted := verifyRangeConversion(t, input.Set.(*finite.EquatableValueSet), universe, input.Members)
			mixed = append(mixed, testutil.EnumeratedSet{Set: converted, Members: input.Members})
			if testutil.Must(converted.Contains(testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1))))) != input.Members[1] {
				t.Fatal("negative zero membership")
			}
			if converted.IncludesNaN() != input.Members[4] {
				t.Fatal("NaN membership lost")
			}
		}
		testutil.VerifyBinaryAlgebra(t, universe, mixed)
		nanOnly := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{universe[4]}))
		nanRanges := testutil.Must(nanOnly.ToSortedRangeSet())
		if len(nanRanges.Ranges()) != 0 || !nanRanges.IncludesNaN() {
			t.Fatal("NaN became an endpoint")
		}
		withoutNaN := testutil.Must(testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{universe[4]})).ToSortedRangeSet())
		if len(withoutNaN.Ranges()) != 1 || withoutNaN.IncludesNaN() || testutil.Must(withoutNaN.IsUniversal()) {
			t.Fatal("excluded NaN lost ordered universe distinction")
		}
		source := testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1)))
		zeros := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{source, universe[1]}))
		if len(testutil.Must(zeros.ToSortedRangeSet()).Ranges()) != 1 || !math.Signbit(source.Float()) {
			t.Fatal("zero normalization changed source or failed to merge")
		}
	}
}

func TestFiniteRangeConversionLargeDomainsAndSharing(t *testing.T) {
	for _, width := range []types.IntWidth{types.Int16, types.Int32, types.Int64} {
		typ := testutil.Must(types.NewIntType(width, foundation.NonNull))
		max := int64(math.MaxInt64)
		if width != types.Int64 {
			max = int64(1)<<(width-1) - 1
		}
		min := -max - 1
		probes := []valueinterfaces.Value{testutil.Must(testbedvalues.NewInteger(typ, min)), testutil.Must(testbedvalues.NewInteger(typ, min+1)), testutil.Must(testbedvalues.NewInteger(typ, 0)), testutil.Must(testbedvalues.NewInteger(typ, max-1)), testutil.Must(testbedvalues.NewInteger(typ, max))}
		verifyRangeConversion(t, testutil.Must(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{probes[0], probes[4]})), probes, []bool{false, true, true, true, false})
		singleton := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{probes[2]}))
		converted := testutil.Must(singleton.ToSortedRangeSet())
		v, _ := converted.Ranges()[0].Lower().Value().Get()
		if v != probes[2] {
			t.Fatal("immutable Value not shared")
		}
	}
	huge := uint64(math.MaxUint64)
	for _, bound := range []*uint64{nil, &huge} {
		typ := testutil.Must(types.NewVarCharType(bound, foundation.NonNull))
		a := testutil.Must(testbedvalues.NewVarChar(typ, "a"))
		b := testutil.Must(testbedvalues.NewVarChar(typ, "a\x00"))
		c := testutil.Must(testbedvalues.NewVarChar(typ, "b"))
		source := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{b, a, b}))
		converted := verifyRangeConversion(t, source, []valueinterfaces.Value{a, b, c}, []bool{true, true, false})
		if len(converted.Ranges()) != 1 {
			t.Fatal("adjacent points not merged")
		}
	}
}

func TestFiniteRangeConversionBooleanAndErrors(t *testing.T) {
	boolean := testutil.Must(types.NewBoolType(foundation.NonNull))
	universe := []valueinterfaces.Value{testutil.Must(testbedvalues.NewBool(boolean, false)), testutil.Must(testbedvalues.NewBool(boolean, true))}
	for _, input := range testutil.EnumerateSets(boolean, universe, []int{0, 1}) {
		result, err := input.Set.(*finite.EquatableValueSet).ToSortedRangeSet()
		if result != nil || !errors.Is(err, value_set.ErrUnsupported) {
			t.Fatal("Boolean conversion admitted", err)
		}
	}
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	otherType := testutil.Must(types.NewIntType(types.Int16, foundation.NonNull))
	a := testutil.Must(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
	b := testutil.Must(sets.NewSortedRangeSet(testbedvalues.ValueFactory{}, otherType, nil, false))
	for _, pair := range [][2]value_set.ValueSet{{a, b}, {b, a}} {
		for _, op := range []func(value_set.ValueSet) (value_set.ValueSet, error){pair[0].Union, pair[0].Intersect, pair[0].Difference} {
			if result, err := op(pair[1]); result != nil || !errors.Is(err, value_set.ErrIncompatibleTypes) {
				t.Fatal("incompatible mixed operation", err)
			}
		}
		for _, query := range []func(value_set.ValueSet) (bool, error){pair[0].EqualsTo, pair[0].IsSubsetOf, pair[0].IsDisjointFrom} {
			if result, err := query(pair[1]); result || !errors.Is(err, value_set.ErrIncompatibleTypes) {
				t.Fatal("incompatible mixed query", err)
			}
		}
	}
	for _, bad := range []*finite.EquatableValueSet{nil, {}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("missing constructed-set assertion")
				}
			}()
			_, _ = bad.ToSortedRangeSet()
		}()
	}
}
