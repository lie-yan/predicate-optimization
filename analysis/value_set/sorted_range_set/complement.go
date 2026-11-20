package sorted_range_set

import (
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Complement constructs the universe-relative gaps before, between and after
// stored ranges. A bounded bordering cut flips inclusion; an unbounded side
// suppresses the outside gap. NewRange omits empty gaps at domain extrema.
// Float NaN membership flips independently; SQL NULL is outside this universe.
// Work is O(n) boundary checks with at most n+1 result ranges. Results own their
// slices and share immutable Types/Values, without materializing the universe.
func (s *SortedRangeSet) Complement() (value_set.ValueSet, error) {
	return s.complement(s.factory)
}

// complement synthesizes with the operation owner's factory without allocating
// a rebound receiver wrapper. The source set and its factory remain unchanged.
func (s *SortedRangeSet) complement(factory valueinterfaces.ValueFactory) (value_set.ValueSet, error) {
	s.assertConstructed()
	result := &SortedRangeSet{factory: factory, typ: s.typ, ranges: make([]*ranges.Range, 0)}
	if _, floating := s.typ.(*types.FloatType); floating {
		result.includesNaN = !s.includesNaN
	}
	lower := ranges.RangeEndpoint{}
	for _, r := range s.ranges {
		if !r.Lower().IsUnbounded() {
			upper, err := flipBoundedEndpoint(r.Lower())
			if err != nil {
				return nil, err
			}
			gap, err := ranges.NewRange(factory, s.typ, lower, upper)
			if err != nil {
				return nil, err
			}
			if interval, present := gap.Get(); present {
				result.ranges = append(result.ranges, interval)
			}
		}
		if r.Upper().IsUnbounded() {
			return result, nil
		}
		var err error
		lower, err = flipBoundedEndpoint(r.Upper())
		if err != nil {
			return nil, err
		}
	}
	tail, err := ranges.NewRange(factory, s.typ, lower, ranges.RangeEndpoint{})
	if err != nil {
		return nil, err
	}
	if interval, present := tail.Get(); present {
		result.ranges = append(result.ranges, interval)
	}
	return result, nil
}

// Only bounded endpoints have an opposite inclusion; absence denotes a domain
// boundary whose side must be handled by the caller.
func flipBoundedEndpoint(endpoint ranges.RangeEndpoint) (ranges.RangeEndpoint, error) {
	value, present := endpoint.Value().Get()
	if !present {
		panic("value_set: expected bounded endpoint")
	}
	return ranges.NewRangeEndpoint(value, !endpoint.IsInclusive())
}
