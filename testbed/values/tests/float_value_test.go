package values_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestFloatValues(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := fixtureValue(types.NewFloatType(width, foundation.MayNull))
		payloads := []float64{0, math.Copysign(0, -1), 1.5, math.Inf(1), math.Inf(-1), math.NaN()}
		if width == types.Float32 {
			payloads = append(payloads, math.MaxFloat32, math.SmallestNonzeroFloat32, 1<<24)
		} else {
			payloads = append(payloads, math.MaxFloat64, math.SmallestNonzeroFloat64, 1<<53)
		}
		for _, payload := range payloads {
			v := fixtureValue(testbedvalues.NewFloat(typ, payload))
			if v.IsNull() || v.Type() != typ {
				t.Fatal("invalid float")
			}
			if !math.IsNaN(payload) && math.Float64bits(v.Float()) != math.Float64bits(payload) {
				t.Fatal("payload changed")
			}
			if !v.EqualsTo(fixtureValue(testbedvalues.NewFloat(typ, payload))) {
				t.Fatal("not structurally equal")
			}
			adapted := fixtureValue(v.WithNullability(foundation.NonNull))
			if adapted.Type().Nullability() != foundation.NonNull || v.Type().Nullability() != foundation.MayNull || adapted.EqualsTo(v) {
				t.Fatal("bad adaptation")
			}
			arrayType := fixtureValue(types.NewArrayType(typ, foundation.NonNull))
			a := fixtureValue(testbedvalues.NewArray(arrayType, []valueinterfaces.Value{v, fixtureValue(testbedvalues.NewNull(typ))}))
			if !a.EqualsTo(fixtureValue(testbedvalues.NewArray(arrayType, a.Elements()))) {
				t.Fatal("nested float equality")
			}
		}
		pos := fixtureValue(testbedvalues.NewFloat(typ, 0))
		neg := fixtureValue(testbedvalues.NewFloat(typ, math.Copysign(0, -1)))
		if pos.EqualsTo(neg) {
			t.Fatal("literal zero signs lost")
		}
		n1 := fixtureValue(testbedvalues.NewFloat(typ, math.Float64frombits(0x7ff8000000000001)))
		n2 := fixtureValue(testbedvalues.NewFloat(typ, math.Float64frombits(0xfff8000000000002)))
		if !n1.EqualsTo(n2) {
			t.Fatal("NaNs not canonical")
		}
	}
	typ := fixtureValue(types.NewFloatType(types.Float32, foundation.NonNull))
	for _, payload := range []float64{0.1, math.MaxFloat64, math.SmallestNonzeroFloat64, (1 << 24) + 1} {
		if _, err := testbedvalues.NewFloat(typ, payload); err == nil {
			t.Fatal("implicit Float32 rounding")
		}
	}
	if _, err := testbedvalues.NewFloat(typ, float64(float32(0.1))); err != nil {
		t.Fatal(err)
	}
	f64 := fixtureValue(types.NewFloatType(types.Float64, foundation.NonNull))
	if fixtureValue(testbedvalues.NewFloat(typ, 1)).EqualsTo(fixtureValue(testbedvalues.NewFloat(f64, 1))) {
		t.Fatal("width ignored")
	}
}
