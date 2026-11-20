package equatable_value_set

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

// Cardinality boundary tests avoid allocating billions of Values just to check
// overflow and coverage calculations for large domains.
func TestUniverseCoverageCardinality(t *testing.T) {
	mustType := func(typ types.Type, err error) types.Type {
		if err != nil {
			t.Fatal(err)
		}
		return typ
	}
	for _, tc := range []struct {
		typ   types.Type
		count uint64
	}{
		{mustType(types.NewBoolType(foundation.NonNull)), 2},
		{mustType(types.NewIntType(types.Int8, foundation.NonNull)), 256},
		{mustType(types.NewIntType(types.Int16, foundation.NonNull)), 65536},
		{mustType(types.NewIntType(types.Int32, foundation.NonNull)), 4294967296},
		{mustType(types.NewFloatType(types.Float32, foundation.NonNull)), 4278190082},
		{mustType(types.NewFloatType(types.Float64, foundation.NonNull)), 18437736874454810626},
	} {
		full, err := coversUniverse(tc.typ, tc.count)
		if err != nil || !full {
			t.Fatalf("%s coverage: %v %v", tc.typ, full, err)
		}
		partial, err := coversUniverse(tc.typ, tc.count-1)
		if err != nil || partial {
			t.Fatal("incomplete universe")
		}
	}
	for _, tc := range []struct {
		limit, count uint64
		want         bool
	}{
		{0, 1, true}, {0, 0, false}, {1, 257, true}, {1, 256, false}, {2, 65793, true}, {7, 72340172838076673, true}, {8, math.MaxUint64, false}, {math.MaxUint64, math.MaxUint64, false},
	} {
		typ := mustType(types.NewVarCharType(&tc.limit, foundation.NonNull))
		got, err := coversUniverse(typ, tc.count)
		if err != nil || got != tc.want {
			t.Fatalf("limit=%d count=%d got=%v err=%v", tc.limit, tc.count, got, err)
		}
	}
	integer := mustType(types.NewIntType(types.Int64, foundation.NonNull))
	if full, err := coversUniverse(integer, math.MaxUint64); err != nil || full {
		t.Fatal("Int64 universe overflow")
	}
	array := mustType(types.NewArrayType(integer, foundation.NonNull))
	if _, err := coversUniverse(array, 0); !errors.Is(err, value_set.ErrUnsupported) {
		t.Fatal("unsupported universe must be explicit")
	}
}
