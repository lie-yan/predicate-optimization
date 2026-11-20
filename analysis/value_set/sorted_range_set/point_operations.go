package sorted_range_set

import (
	"fmt"
	"math"
	"slices"

	"github.com/lie-yan/predicate-optimization/types"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// WithPoints inserts non-null values, merging singleton ranges with existing
// intervals. Values already covered by the output need no singleton allocation.
// NaN sets its separate flag. Inputs may be unsorted, duplicated or outer-nullable;
// all must be valid and compatible. Nil Values assert; NULL Values and
// incompatible Types return the usual scalar errors before processing ranges.
// Sorting d points costs O(d log(d+1)) comparisons; the merge costs O(d+r)
// boundary checks for r ranges, with O(d+r) storage and no universe enumeration.
func (s *SortedRangeSet) WithPoints(input []valueinterfaces.Value) (*SortedRangeSet, error) {
	s.assertConstructed()
	points, nan, err := s.sortedPoints(input)
	if err != nil {
		return nil, err
	}
	result := make([]*ranges.Range, 0)
	i, j := 0, 0
	for i < len(s.ranges) || j < len(points) {
		takeRange := j == len(points)
		if !takeRange && i < len(s.ranges) {
			order, err := ranges.CompareLowerEndpoints(s.ranges[i].Lower(), points[j])
			if err != nil {
				return nil, err
			}
			takeRange = order <= 0
		}
		if takeRange {
			result, err = appendNormalizedRange(s.factory, result, s.ranges[i])
			if err != nil {
				return nil, err
			}
			i++
			continue
		}
		point := points[j]
		j++
		if len(result) > 0 {
			order, err := ranges.CompareUpperEndpoints(result[len(result)-1].Upper(), point)
			if err != nil {
				return nil, err
			}
			if order >= 0 {
				continue
			}
		}
		singleton, err := ranges.NewRange(s.factory, s.typ, point, point)
		if err != nil {
			return nil, err
		}
		r, present := singleton.Get()
		if !present {
			panic("value_set: expected nonempty singleton")
		}
		result, err = appendNormalizedRange(s.factory, result, r)
		if err != nil {
			return nil, err
		}
	}
	return &SortedRangeSet{factory: s.factory, typ: s.typ, ranges: result, includesNaN: s.includesNaN || nan}, nil
}

// WithoutPoints removes explicit values by walking sorted points and ranges,
// emitting the pieces on either side of each contained point. NaN clears its
// separate flag. Input validation and sorting costs match WithPoints; the scan
// takes O(d+r) boundary checks and O(d+r) storage, without constructing a full
// complement or re-sorting the result. Immutable source Values/ranges are shared.
func (s *SortedRangeSet) WithoutPoints(input []valueinterfaces.Value) (*SortedRangeSet, error) {
	s.assertConstructed()
	points, nan, err := s.sortedPoints(input)
	if err != nil {
		return nil, err
	}
	result := make([]*ranges.Range, 0)
	j := 0
	for _, r := range s.ranges {
		lower := r.Lower()
		changed := false
		for j < len(points) {
			point := points[j]
			order, err := ranges.CompareLowerEndpoints(lower, point)
			if err != nil {
				return nil, err
			}
			if order > 0 {
				j++
				continue
			}
			order, err = ranges.CompareUpperEndpoints(r.Upper(), point)
			if err != nil {
				return nil, err
			}
			if order < 0 {
				break
			}
			excluded, err := flipBoundedEndpoint(point)
			if err != nil {
				return nil, err
			}
			before, err := ranges.NewRange(s.factory, s.typ, lower, excluded)
			if err != nil {
				return nil, err
			}
			if piece, present := before.Get(); present {
				result = append(result, piece)
			}
			lower = excluded
			changed = true
			j++
		}
		if !changed {
			result = append(result, r)
			continue
		}
		tail, err := ranges.NewRange(s.factory, s.typ, lower, r.Upper())
		if err != nil {
			return nil, err
		}
		if piece, present := tail.Get(); present {
			result = append(result, piece)
		}
	}
	return &SortedRangeSet{factory: s.factory, typ: s.typ, ranges: result, includesNaN: s.includesNaN && !nan}, nil
}

// Keep points as bounded inclusive endpoints, even at domain extrema. Normalized
// singleton ranges can have absent bounds and are unsuitable as point cursors.
// All validation precedes sorting. Signed zeros compare equal; result Range
// construction canonicalizes their endpoint payloads without changing inputs.
func (s *SortedRangeSet) sortedPoints(input []valueinterfaces.Value) ([]ranges.RangeEndpoint, bool, error) {
	points := make([]ranges.RangeEndpoint, 0, len(input))
	nan := false
	for _, v := range input {
		endpoint, err := ranges.NewRangeEndpoint(v, true)
		if err != nil {
			return nil, false, err
		}
		if !s.typ.EqualsToIgnoringNullability(v.Type()) {
			return nil, false, fmt.Errorf("%w: range point", value_set.ErrIncompatibleTypes)
		}
		if _, floating := s.typ.(*types.FloatType); floating && math.IsNaN(v.(valueinterfaces.FloatValue).Float()) {
			nan = true
			continue
		}
		points = append(points, endpoint)
	}
	var comparisonErr error
	slices.SortStableFunc(points, func(a, b ranges.RangeEndpoint) int {
		if comparisonErr != nil {
			return 0
		}
		order, err := ranges.CompareLowerEndpoints(a, b)
		if err != nil {
			comparisonErr = err
			return 0
		}
		return order
	})
	if comparisonErr != nil {
		return nil, false, comparisonErr
	}
	return points, nan, nil
}
