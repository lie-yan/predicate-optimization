package functions_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func boolEqualityFixture[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestBooleanEqualityNamesAndScope(t *testing.T) {
	provider := boolEqualityFixture(testbed.NewStandardContext())
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		typ := boolEqualityFixture(types.NewBoolType(n))
		handle := boolEqualityFixture(provider.ComparisonFunction(foundation.Equal, typ, typ))
		named := boolEqualityFixture(provider.LookupBuiltInFunction(shared.BuiltInBoolEqual, []types.Type{typ, typ}))
		if !handle.EqualsTo(named) || !boolEqualityFixture(provider.IsComparisonFunction(handle)) {
			t.Fatal("Boolean equality identity/classification wrong")
		}
		metadata := boolEqualityFixture(provider.Metadata(handle))
		sig := boolEqualityFixture(shared.BindFunctionSignature(metadata, []types.Type{typ, typ}))
		for _, a := range []bool{false, true} {
			for _, b := range []bool{false, true} {
				left, right := boolEqualityFixture(testbedvalues.NewBool(typ, a)), boolEqualityFixture(testbedvalues.NewBool(typ, b))
				got := boolEqualityFixture(provider.Evaluate(handle, sig, []valueinterfaces.Value{left, right}))
				if got.(*testbedvalues.BoolValue).Bool() != (a == b) || got.Type().Nullability() != n {
					t.Fatal("Boolean equality value/type wrong")
				}
			}
		}
		for _, op := range []foundation.OperatorType{foundation.NotEqual, foundation.LessThan, foundation.LessThanOrEqual, foundation.GreaterThan, foundation.GreaterThanOrEqual} {
			if _, err := provider.ComparisonFunction(op, typ, typ); !errors.Is(err, shared.ErrUnsupportedLookup) {
				t.Fatal("unrequested Boolean comparison admitted", err)
			}
		}
	}
}
