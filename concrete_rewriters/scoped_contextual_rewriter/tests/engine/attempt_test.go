package engine_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func TestOccurrencePathResolution(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	guard := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(boolean, true))))
	other := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(boolean, false))))
	// Both edges use the same node, but only [0, 0] is the designated guard occurrence.
	inner := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{guard, guard}))
	root := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{inner, other}))
	before := expressions.PrettyPrint(root)
	for _, tc := range []struct {
		path engine.OccurrencePath
		want expressions.Expression
	}{
		{nil, root},
		{engine.OccurrencePath{}, root},
		{engine.OccurrencePath{0}, inner},
		{engine.OccurrencePath{0, 0}, guard},
		{engine.OccurrencePath{0, 1}, guard},
		{engine.OccurrencePath{1}, other},
	} {
		got, err := tc.path.Resolve(root)
		if err != nil || got != tc.want {
			t.Fatalf("resolve(%v) = %v, %v; want %v", tc.path, got, err, tc.want)
		}
	}
	for _, path := range []engine.OccurrencePath{{-1}, {2}, {0, -1}, {0, 2}, {1, 0}, {0, 0, 0}} {
		got, err := path.Resolve(root)
		if got != nil || !errors.Is(err, rewriter.ErrInvalidReplacement) {
			t.Fatalf("invalid path %v returned %v, %v", path, got, err)
		}
	}
	if expressions.PrettyPrint(root) != before || root.Children()[0] != inner || inner.Children()[0] != guard || inner.Children()[1] != guard {
		t.Fatal("resolution changed source structure or sharing")
	}
}

func TestOccurrencePathOwnership(t *testing.T) {
	// Spare capacity exposes an unsafe append that would alias sibling paths.
	backing := []int{0, 9, 8, 7}
	parent := engine.OccurrencePath(backing[:1])
	left, right := parent.Extended(0), parent.Extended(1)
	cloned := parent.Clone()
	if !slices.Equal(backing, []int{0, 9, 8, 7}) {
		t.Fatal("child extension modified source storage")
	}
	left[0], left[1], cloned[0] = 3, 4, 5
	if !parent.EqualsTo(engine.OccurrencePath{0}) || !right.EqualsTo(engine.OccurrencePath{0, 1}) {
		t.Fatal("child or clone aliases source/sibling storage")
	}
	parent[0] = 6
	if !left.EqualsTo(engine.OccurrencePath{3, 4}) || !right.EqualsTo(engine.OccurrencePath{0, 1}) || !cloned.EqualsTo(engine.OccurrencePath{5}) {
		t.Fatal("source mutation changed a retained path")
	}
	var root engine.OccurrencePath
	if !root.Clone().EqualsTo(engine.OccurrencePath{}) || !root.Extended(2).EqualsTo(engine.OccurrencePath{2}) {
		t.Fatal("empty root path copy/extension failed")
	}
}

func TestOccurrencePathGuardClassification(t *testing.T) {
	guard := engine.OccurrencePath{0, 1}
	for _, tc := range []struct {
		path    engine.OccurrencePath
		atGuard bool
		onPath  bool
	}{
		{nil, false, true},
		{engine.OccurrencePath{0}, false, true},
		{engine.OccurrencePath{0, 1}, true, true},
		{engine.OccurrencePath{0, 0}, false, false},
		{engine.OccurrencePath{1}, false, false},
		{engine.OccurrencePath{0, 1, 0}, false, false},
	} {
		if tc.path.EqualsTo(guard) != tc.atGuard || tc.path.IsPrefixOf(guard) != tc.onPath {
			t.Errorf("incorrect guard relation for %v", tc.path)
		}
	}
	var root engine.OccurrencePath
	if !root.EqualsTo(engine.OccurrencePath{}) || !root.IsPrefixOf(root) || guard.IsPrefixOf(root) {
		t.Fatal("incorrect root/equality prefix semantics")
	}
	// Prefix matching compares edges, not just depth or expression identity.
	if (engine.OccurrencePath{1}).IsPrefixOf(guard) || (engine.OccurrencePath{0, 0}).EqualsTo(engine.OccurrencePath{0, 1}) {
		t.Fatal("distinct occurrences conflated")
	}
}

func TestOccurrencePathNilSource(t *testing.T) {
	for _, source := range []expressions.Expression{nil, (*expressions.LogicalExpression)(nil)} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("nil source did not assert")
				}
			}()
			_, _ = (engine.OccurrencePath{}).Resolve(source)
		}()
	}
}
