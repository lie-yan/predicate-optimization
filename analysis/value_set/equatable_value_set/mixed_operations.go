package equatable_value_set

import (
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// filterByRange receives compatible constructed sets and an included receiver.
// Filtering preserves uniqueness, normalization and source order, so it needs
// no deduplication or conversion pass. O(d log(r+1)) membership comparisons,
// plus payload checks, for d entries and r ranges; O(k) result storage.
func (s *EquatableValueSet) filterByRange(other *sorted_range_set.SortedRangeSet, keepMembers bool) (value_set.ValueSet, error) {
	result := make([]valueinterfaces.Value, 0)
	for _, v := range s.elements.values {
		present, err := other.Contains(v)
		if err != nil {
			return nil, err
		}
		if present == keepMembers {
			result = append(result, v)
		}
	}
	return &EquatableValueSet{factory: s.factory, typ: s.typ, elements: valueArray{values: result, ordered: s.elements.ordered}, isInclusion: true}, nil
}
