package internal_test

import (
	"math"
	"testing"

	valuesetinternal "github.com/lie-yan/predicate-optimization/analysis/value_set/internal"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	"github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

func TestFormatExternalScalarValues(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	f32 := must(types.NewFloatType(types.Float32, foundation.NonNull))
	f64 := must(types.NewFloatType(types.Float64, foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	for _, value := range []valueinterfaces.Value{
		must(testbedvalues.NewBool(boolean, false)),
		must(testbedvalues.NewInteger(integer, math.MinInt64)),
		must(testbedvalues.NewFloat(f32, float64(float32(1.234567)))),
		must(testbedvalues.NewFloat(f64, math.Copysign(0, -1))),
		must(testbedvalues.NewFloat(f64, math.NaN())),
		must(testbedvalues.NewFloat(f64, math.Inf(1))),
		must(testbedvalues.NewVarChar(text, "é\n\xff")),
	} {
		view := testutil.WrapScalar(value)
		if got, want := valuesetinternal.FormatValue(view), valuesetinternal.FormatValue(value); got != want {
			t.Fatalf("external formatting = %q, want %q", got, want)
		}
		if !view.EqualsTo(value) || !value.EqualsTo(view) {
			t.Fatal("formatting changed typed payload")
		}
	}
}
