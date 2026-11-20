package reconstruction

import (
	"slices"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// ExpressionSize reports structural counts, not execution cost or encoded bytes.
// Shared subtrees are counted at each occurrence rather than once by identity.
type ExpressionSize struct {
	// NodeCount counts all expression node occurrences, including the root.
	NodeCount int
	// LiteralOccurrences counts literal nodes, including those outside IN lists.
	LiteralOccurrences int
	// MembershipEntries counts IN candidate positions, excluding the target operand.
	MembershipEntries int
}

// MeasureExpression reports sizes for either a candidate or its original source.
// It traverses a valid immutable expression without evaluating it or memoizing
// shared nodes. Work is linear in occurrence count; required nil inputs assert.
// The caller compares these counts and decides whether expansion is worthwhile.
func MeasureExpression(expression expressions.Expression) ExpressionSize {
	assertions.NotNil(expression)
	var result ExpressionSize
	pending := []expressions.Expression{expression}
	for len(pending) != 0 {
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		result.NodeCount++
		switch typed := node.(type) {
		case *expressions.LiteralExpression:
			result.LiteralOccurrences++
		case *expressions.SpecialFormExpression:
			if typed.Form() == foundation.SpecialFormIn {
				result.MembershipEntries += typed.ChildCount() - 1
			}
		}
		// Reserve once, preserving the original stack order without a child copy.
		count := node.ChildCount()
		pending = slices.Grow(pending, count)
		for i := 0; i < count; i++ {
			pending = append(pending, node.ChildAt(i))
		}
	}
	return result
}
