package engine_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestImprovesStructuralPriority(t *testing.T) {
	for _, tc := range []struct {
		name              string
		source, candidate reconstruction.ExpressionSize
		want              bool
	}{
		{
			name:      "fewer nodes outweigh more nonliteral nodes",
			source:    reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 7},
			candidate: reconstruction.ExpressionSize{NodeCount: 9, LiteralOccurrences: 1},
			want:      true,
		},
		{
			name:      "equal nodes with fewer nonliteral nodes",
			source:    reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 4},
			candidate: reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 5},
			want:      true,
		},
		{
			name:      "equal nodes with more nonliteral nodes",
			source:    reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 5},
			candidate: reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 4},
		},
		{
			name:      "identical measures",
			source:    reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 4, MembershipEntries: 2},
			candidate: reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 4, MembershipEntries: 2},
		},
		{
			name:      "growth rejected despite fewer nonliteral nodes",
			source:    reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 4},
			candidate: reconstruction.ExpressionSize{NodeCount: 11, LiteralOccurrences: 8},
		},
		{
			name:      "fewer membership entries alone give no improvement",
			source:    reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 4, MembershipEntries: 3},
			candidate: reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 4, MembershipEntries: 1},
		},
		{
			name:      "membership entries do not override structural improvement",
			source:    reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 4, MembershipEntries: 1},
			candidate: reconstruction.ExpressionSize{NodeCount: 10, LiteralOccurrences: 5, MembershipEntries: 3},
			want:      true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.Improves(tc.source, tc.candidate); got != tc.want {
				t.Fatalf("Improves(%+v, %+v) = %v, want %v", tc.source, tc.candidate, got, tc.want)
			}
		})
	}
}

func TestImprovesCompleteRegions(t *testing.T) {
	provider := membershipEligibilityProvider()
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	guard := membershipExpression(x, guardInteger(1), guardInteger(2))
	target := guardComparison(provider, foundation.Equal, x, 3)
	truth := cleanupLiteral(foundation.True, foundation.NonNull)
	falsity := cleanupLiteral(foundation.False, foundation.NonNull)
	booleanGuard := guardReference("g", truth.Type())

	// These candidates are explicitly constructed to exercise size policy. This
	// helper does not prove equivalence, perform scoped rewriting or accept results.
	for _, tc := range []struct {
		name                      string
		source, candidate         expressions.Expression
		sourceSize, candidateSize reconstruction.ExpressionSize
		want                      bool
	}{
		{
			name:          "guard retained in a smaller complete region",
			source:        guardLogical(foundation.And, guard, guardLogical(foundation.Not, target)),
			candidate:     guardLogical(foundation.And, guard, truth),
			sourceSize:    reconstruction.ExpressionSize{NodeCount: 9, LiteralOccurrences: 3, MembershipEntries: 2},
			candidateSize: reconstruction.ExpressionSize{NodeCount: 6, LiteralOccurrences: 3, MembershipEntries: 2},
			want:          true,
		},
		{
			name:          "reference replacement benefits an equal-size region",
			source:        guardLogical(foundation.And, booleanGuard, booleanGuard),
			candidate:     guardLogical(foundation.And, booleanGuard, truth),
			sourceSize:    reconstruction.ExpressionSize{NodeCount: 3},
			candidateSize: reconstruction.ExpressionSize{NodeCount: 3, LiteralOccurrences: 1},
			want:          true,
		},
		{
			name:          "shared targets count at both occurrences",
			source:        guardLogical(foundation.And, guard, guardLogical(foundation.Or, target, target)),
			candidate:     guardLogical(foundation.And, guard, guardLogical(foundation.Or, falsity, falsity)),
			sourceSize:    reconstruction.ExpressionSize{NodeCount: 12, LiteralOccurrences: 4, MembershipEntries: 2},
			candidateSize: reconstruction.ExpressionSize{NodeCount: 8, LiteralOccurrences: 4, MembershipEntries: 2},
			want:          true,
		},
		{
			name:          "an equal copy is not an improvement",
			source:        guardLogical(foundation.And, guard, truth),
			candidate:     guardLogical(foundation.And, guard, truth),
			sourceSize:    reconstruction.ExpressionSize{NodeCount: 6, LiteralOccurrences: 3, MembershipEntries: 2},
			candidateSize: reconstruction.ExpressionSize{NodeCount: 6, LiteralOccurrences: 3, MembershipEntries: 2},
		},
		{
			name:          "a local literal substitution does not excuse complete-region growth",
			source:        guardLogical(foundation.And, booleanGuard, booleanGuard),
			candidate:     guardLogical(foundation.And, booleanGuard, truth, truth),
			sourceSize:    reconstruction.ExpressionSize{NodeCount: 3},
			candidateSize: reconstruction.ExpressionSize{NodeCount: 4, LiteralOccurrences: 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := reconstruction.MeasureExpression(tc.source)
			candidate := reconstruction.MeasureExpression(tc.candidate)
			if source != tc.sourceSize || candidate != tc.candidateSize {
				t.Fatalf("complete-region counts = %+v -> %+v, want %+v -> %+v", source, candidate, tc.sourceSize, tc.candidateSize)
			}
			if got := engine.Improves(source, candidate); got != tc.want {
				t.Fatalf("complete-region improvement = %v, want %v", got, tc.want)
			}
		})
	}
}
