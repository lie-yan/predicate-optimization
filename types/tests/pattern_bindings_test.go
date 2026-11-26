package types_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestPatternBindingIsolationAndUnboundResult(t *testing.T) {
	x, err := types.NewVarCharTypePattern("x", foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	y, err := types.NewVarCharTypePattern("y", foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	a, b := uint64(32), uint64(64)
	first, err := types.NewVarCharType(&a, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	second, err := types.NewVarCharType(&b, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	patterns := []types.TypePattern{x, x}
	actual := []types.Type{first, first}
	bindings, ok := types.BindTypePatterns(patterns, actual)
	if !ok {
		t.Fatal("exact binding failed")
	}
	patterns[0] = y
	actual[0] = second
	if failed, ok := types.BindTypePatterns([]types.TypePattern{x, x}, []types.Type{first, second}); ok || failed != nil {
		t.Fatal("partial conflicting bindings escaped")
	}
	other, ok := types.BindTypePatterns([]types.TypePattern{x}, []types.Type{second})
	if !ok {
		t.Fatal("independent binding failed")
	}
	resolved, err := bindings.Resolve(x)
	if err != nil {
		t.Fatal(err)
	}
	different, err := other.Resolve(x)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.EqualsTo(first) || !different.EqualsTo(second) {
		t.Fatal("binding scopes or caller slices leaked into each other")
	}
	if unresolved, err := bindings.Resolve(y); err == nil || unresolved != nil {
		t.Fatal("unbound result parameter was accepted")
	}
}
