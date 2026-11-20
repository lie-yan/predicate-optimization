package types_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestFloatTypeIdentityAndNullability(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
			a, err := types.NewFloatType(width, n)
			must(t, a, err)
			b, err := types.NewFloatType(width, n)
			must(t, b, err)
			if !a.EqualsTo(b) || a.FloatWidth() != width || a.String() != fmt.Sprintf("Float%d%s", width, n.Suffix()) {
				t.Fatal("float descriptor contract failed")
			}
			otherWidth := types.Float32
			if width == types.Float32 {
				otherWidth = types.Float64
			}
			other, err := types.NewFloatType(otherWidth, n)
			must(t, other, err)
			integerType := integer(t, types.IntWidth(width), n)
			if a.EqualsTo(other) || a.EqualsTo(integerType) || integerType.EqualsTo(a) {
				t.Fatal("width or numeric type distinction lost")
			}
			changed := foundation.NonNull
			if n == foundation.NonNull {
				changed = foundation.MayNull
			}
			adapted, err := a.WithNullability(changed)
			must(t, adapted, err)
			if a.EqualsTo(adapted) || a.Nullability() != n || adapted.(*types.FloatType).FloatWidth() != width {
				t.Fatal("nullability adaptation failed")
			}
			left := array(t, a, foundation.NonNull)
			right := array(t, b, foundation.NonNull)
			if !left.EqualsTo(right) {
				t.Fatal("nested float equality failed")
			}
		}
	}
}

func TestFloatTypeInvalidConstruction(t *testing.T) {
	for _, width := range []types.FloatWidth{0, 8, 16, 31, 128, 255} {
		if _, err := types.NewFloatType(width, foundation.NonNull); err == nil {
			t.Fatalf("accepted width %d", width)
		}
	}
	if _, err := types.NewFloatType(types.Float32, foundation.Nullability(255)); err == nil {
		t.Fatal("accepted invalid nullability")
	}
	typ, err := types.NewFloatType(types.Float64, foundation.NonNull)
	must(t, typ, err)
	if _, err := typ.WithNullability(foundation.Nullability(255)); err == nil {
		t.Fatal("accepted invalid nullability adaptation")
	}
}
