package sorted_range_set

import (
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/internal"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// ApplyOperation implements the internal cross-package dispatch protocol.
// Copy only the wrapper: immutable payloads remain shared, and the source's
// retained factory never changes (even if the operation fails).
func (s *SortedRangeSet) ApplyOperation(operation internal.Operation) (value_set.ValueSet, error) {
	s.assertConstructed()
	assertions.NotNil(operation.Factory)
	view := *s
	view.factory = operation.Factory
	return operation.Run(&view)
}
