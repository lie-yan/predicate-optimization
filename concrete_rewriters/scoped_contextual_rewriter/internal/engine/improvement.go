package engine

import "github.com/lie-yan/predicate-optimization/analysis/reconstruction"

// Improves compares valid complete-region measurements from MeasureExpression.
// A candidate must not grow and must strictly decrease the lexicographic pair
// (NodeCount, NodeCount - LiteralOccurrences). MembershipEntries is diagnostic
// only. This structural preference establishes neither equivalence nor lower latency;
// acceptance must first validate the candidate's semantics and evidence.
func Improves(source, candidate reconstruction.ExpressionSize) bool {
	if candidate.NodeCount > source.NodeCount {
		return false
	}
	if candidate.NodeCount < source.NodeCount {
		return true
	}
	return candidate.NodeCount-candidate.LiteralOccurrences < source.NodeCount-source.LiteralOccurrences
}
