package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

func TestMixedOccurrencePathResolution(t *testing.T) {
	root, path, want := resolutionMixedFixture()
	before := expressions.PrettyPrint(root)
	for _, p := range []engine.OccurrencePath{path, {0, 0, 1}} {
		if got, err := p.Resolve(root); err != nil || got != want {
			t.Fatal("mixed/shared occurrence resolved incorrectly", err)
		}
	}
	if got, err := (engine.OccurrencePath{}).Resolve(root); err != nil || got != root {
		t.Fatal("empty path did not preserve root", err)
	}
	if expressions.PrettyPrint(root) != before {
		t.Fatal("resolution mutated source")
	}
}

func TestOccurrencePathDiagnostics(t *testing.T) {
	root, _, _ := resolutionMixedFixture()
	for _, tc := range []struct {
		path    engine.OccurrencePath
		message string
	}{
		{engine.OccurrencePath{-1}, "occurrence path [-1]: index -1 at depth 0 outside 3 children"},
		{engine.OccurrencePath{3}, "occurrence path [3]: index 3 at depth 0 outside 3 children"},
		{engine.OccurrencePath{1, -1}, "occurrence path [1 -1]: index -1 at depth 1 outside 2 children"},
		{engine.OccurrencePath{1, 2}, "occurrence path [1 2]: index 2 at depth 1 outside 2 children"},
		{engine.OccurrencePath{1, 0, 2}, "occurrence path [1 0 2]: index 2 at depth 2 outside 2 children"},
		{engine.OccurrencePath{1, 0, 1, 0}, "occurrence path [1 0 1 0]: index 0 at depth 3 outside 0 children"},
	} {
		got, err := tc.path.Resolve(root)
		want := rewriter.ErrInvalidReplacement.Error() + ": " + tc.message
		if got != nil || !errors.Is(err, rewriter.ErrInvalidReplacement) || err.Error() != want {
			t.Fatalf("resolve(%v): got %v, want %q", tc.path, err, want)
		}
	}
}

func TestOccurrencePathResolutionDoesNotAllocate(t *testing.T) {
	logical, deepPath, leaf := resolutionLogicalFixture(64, 16)
	mixed, mixedPath, mixedLeaf := resolutionMixedFixture()
	for _, tc := range []struct {
		name string
		root expressions.Expression
		path engine.OccurrencePath
		want expressions.Expression
	}{
		{"wide deep shared", logical, deepPath, leaf},
		{"mixed shared", mixed, mixedPath, mixedLeaf},
		{"empty", mixed, nil, mixed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if allocations := testing.AllocsPerRun(100, func() {
				got, err := tc.path.Resolve(tc.root)
				if err != nil || got != tc.want {
					panic("resolution changed")
				}
				resolvedExpressionSink = got
			}); allocations != 0 {
				t.Fatalf("successful resolution allocated %g times", allocations)
			}
		})
	}
}
