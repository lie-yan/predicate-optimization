package equatable_value_set

import (
	"fmt"
	"math"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ToSortedRangeSet preserves mathematical membership in an ordered domain.
// Non-NaN entries become singleton ranges; NaN sets a separate membership flag.
// Exclusions complement the converted entries over the full non-null universe.
// Boolean conversion returns ErrUnsupported, including for empty/universal sets.
// The source stays immutable. For d entries, conversion uses O(d log d) boundary
// comparisons and O(d) space; stable sorting may move O(d log² d) pointers.
// This is not SQL IN extraction: NaN remains a mathematical set member.
func (s *EquatableValueSet) ToSortedRangeSet() (*sorted_range_set.SortedRangeSet, error) {
	s.assertConstructed()
	if !s.typ.IsOrderable() {
		return nil, fmt.Errorf("%w: range conversion for %s", value_set.ErrUnsupported, s.typ)
	}
	intervals := make([]*ranges.Range, 0, len(s.elements.values))
	includesNaN := false
	for _, value := range s.elements.values {
		if _, floating := s.typ.(*types.FloatType); floating && math.IsNaN(value.(valueinterfaces.FloatValue).Float()) {
			includesNaN = true
			continue
		}
		endpoint, err := ranges.NewRangeEndpoint(value, true)
		if err != nil {
			return nil, err
		}
		interval, err := ranges.NewRange(s.factory, s.typ, endpoint, endpoint)
		if err != nil {
			return nil, err
		}
		singleton, present := interval.Get()
		if !present {
			panic("value_set: a non-NaN set element must form a nonempty singleton")
		}
		intervals = append(intervals, singleton)
	}
	converted, err := sorted_range_set.NewSortedRangeSet(s.factory, s.typ, intervals, includesNaN)
	if err != nil {
		return nil, err
	}
	if s.isInclusion {
		return converted, nil
	}
	complement, err := converted.Complement()
	if err != nil {
		return nil, err
	}
	return complement.(*sorted_range_set.SortedRangeSet), nil
}
