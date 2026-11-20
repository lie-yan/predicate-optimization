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

func TestExhaustiveBooleanBinaryAlgebra(t *testing.T) {
	typ := testutil.Must(types.NewBoolType(foundation.NonNull))
	universe := []valueinterfaces.Value{testutil.Must(testbedvalues.NewBool(typ, false)), testutil.Must(testbedvalues.NewBool(typ, true))}
	testutil.VerifyBinaryAlgebra(t, universe, testutil.EnumerateSets(typ, universe, []int{0, 1}))
}

func TestIntegerBinaryAlgebraAgainstEntireInt8Universe(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	universe := make([]valueinterfaces.Value, 256)
	for i := range universe {
		universe[i] = testutil.Must(testbedvalues.NewInteger(typ, int64(i)-128))
	}
	// All 16 subsets of four representative values, in both polarities; every
	// pair is checked over all 256 Int8 values, including both difference orders.
	testutil.VerifyBinaryAlgebra(t, universe, testutil.EnumerateSets(typ, universe, []int{0, 127, 128, 255}))
	full := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, universe))
	all := testutil.Must(equatable.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
	oneExcluded := testutil.Must(equatable.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, universe[:1]))
	restInclusion := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, universe[1:]))
	if !testutil.Must(full.EqualsTo(all)) || !testutil.Must(restInclusion.EqualsTo(oneExcluded)) || !testutil.Must(oneExcluded.EqualsTo(restInclusion)) {
		t.Fatal("cross-polarity finite-domain equality")
	}
	if !testutil.Must(testutil.Must(full.Difference(all)).IsEmpty()) {
		t.Fatal("equivalent universal difference")
	}
}

func TestBinaryValueIdentityAndOwnership(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := testutil.Must(types.NewFloatType(width, foundation.NonNull))
		positive := testutil.Must(testbedvalues.NewFloat(typ, 0))
		negative := testutil.Must(testbedvalues.NewFloat(typ, math.Copysign(0, -1)))
		nan := testutil.Must(testbedvalues.NewFloat(typ, math.NaN()))
		infinity := testutil.Must(testbedvalues.NewFloat(typ, math.Inf(1)))
		a := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{negative, nan, nan}))
		b := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{nan, positive}))
		if !testutil.Must(a.EqualsTo(b)) {
			t.Fatal("float normalized equality")
		}
		excluded := testutil.Must(equatable.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{nan}))
		difference := testutil.Must(a.Difference(excluded))
		if !testutil.Must(difference.Contains(nan)) || testutil.Must(difference.Contains(positive)) {
			t.Fatal("NaN set identity confused with predicate equality")
		}
		union := testutil.Must(a.Union(testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{infinity})))).(*equatable.EquatableValueSet)
		exposed := union.Values()
		exposed[0] = nil
		if a.Values()[0] == nil || union.Values()[0] == nil || !math.Signbit(negative.Float()) {
			t.Fatal("mutation through binary result")
		}
	}
	typ := testutil.Must(types.NewVarCharType(nil, foundation.NonNull))
	nullable := testutil.Must(types.NewVarCharType(nil, foundation.MayNull))
	a := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(typ, "a")), testutil.Must(testbedvalues.NewVarChar(typ, "\xff"))}))
	b := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, nullable, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(nullable, "\xff")), testutil.Must(testbedvalues.NewVarChar(nullable, "a")), testutil.Must(testbedvalues.NewVarChar(nullable, "a"))}))
	if !testutil.Must(a.EqualsTo(b)) {
		t.Fatal("entry order/allocation/nullability changed equality")
	}
	if !testutil.Must(testutil.Must(a.Difference(b)).IsEmpty()) {
		t.Fatal("equal string difference")
	}
	// A one-value VARCHAR universe has equivalent opposite-polarity spellings.
	zero := uint64(0)
	emptyType := testutil.Must(types.NewVarCharType(&zero, foundation.NonNull))
	singleton := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, emptyType, []valueinterfaces.Value{testutil.Must(testbedvalues.NewVarChar(emptyType, ""))}))
	if !testutil.Must(singleton.EqualsTo(testutil.Must(equatable.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, emptyType, nil)))) {
		t.Fatal("bounded string universe equality")
	}
}

func TestBinaryOperationErrors(t *testing.T) {
	typ := testutil.Must(types.NewIntType(types.Int8, foundation.NonNull))
	a := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil))
	incompatible := testutil.Must(equatable.NewEquatableValueSet(testbedvalues.ValueFactory{}, testutil.Must(types.NewIntType(types.Int16, foundation.NonNull)), nil))
	for _, operation := range []func(value_set.ValueSet) (value_set.ValueSet, error){a.Union, a.Intersect, a.Difference} {
		if result, err := operation(incompatible); result != nil || !errors.Is(err, value_set.ErrIncompatibleTypes) {
			t.Fatal("wrong operation error", result, err)
		}
	}
	for _, query := range []func(value_set.ValueSet) (bool, error){a.IsSubsetOf, a.IsDisjointFrom, a.EqualsTo} {
		if _, err := query(incompatible); !errors.Is(err, value_set.ErrIncompatibleTypes) {
			t.Fatal("wrong query error", err)
		}
	}
	for _, bad := range []value_set.ValueSet{nil, (*equatable.EquatableValueSet)(nil), &equatable.EquatableValueSet{}} {
		for _, call := range []func(){func() { _, _ = a.Union(bad) }, func() { _, _ = a.Intersect(bad) }, func() { _, _ = a.Difference(bad) }, func() { _, _ = a.IsSubsetOf(bad) }, func() { _, _ = a.IsDisjointFrom(bad) }, func() { _, _ = a.EqualsTo(bad) }} {
			func() {
				if !assertions.Enabled {
					return
				}
				defer func() {
					if recover() == nil {
						t.Error("missing input assertion")
					}
				}()
				call()
			}()
		}
	}
}
