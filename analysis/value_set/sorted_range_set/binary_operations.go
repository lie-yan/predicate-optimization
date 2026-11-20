package sorted_range_set

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/internal"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Union merges two sorted range streams, coalescing overlapping and adjoining
// intervals. NaN membership is OR. Both sets must be constructed and have the
// same normalized Type, even when empty; incompatible Types return an error.
// Finite operands use specialized point operations where applicable, with
// exact range conversion as fallback for other supported cases.
// After conversion, work is O(m+n) endpoint comparisons/boundary checks,
// with O(k) result storage for k output ranges. Payload costs depend on the domain. Results own their
// slices and share immutable ranges/Values; no re-sorting or enumeration occurs.
func (s *SortedRangeSet) Union(other value_set.ValueSet) (value_set.ValueSet, error) {
	s.assertConstructed()
	assertions.NotNil(other)
	if !s.typ.EqualsTo(other.Type()) {
		return nil, fmt.Errorf("%w: union", value_set.ErrIncompatibleTypes)
	}
	if finite, ok := other.(value_set.FiniteValueSet); ok {
		return internal.Apply(s.factory, finite, func(view value_set.ValueSet) (value_set.ValueSet, error) { return view.Union(s) })
	}
	rightSet, err := asSortedRangeSet(s.factory, other)
	if err != nil {
		return nil, err
	}
	result := make([]*ranges.Range, 0)
	i, j := 0, 0
	for i < len(s.ranges) || j < len(rightSet.ranges) {
		var next *ranges.Range
		switch {
		case i == len(s.ranges):
			next = rightSet.ranges[j]
			j++
		case j == len(rightSet.ranges):
			next = s.ranges[i]
			i++
		default:
			order, err := ranges.CompareLowerEndpoints(s.ranges[i].Lower(), rightSet.ranges[j].Lower())
			if err != nil {
				return nil, err
			}
			if order <= 0 {
				next = s.ranges[i]
				i++
			} else {
				next = rightSet.ranges[j]
				j++
			}
		}
		var err error
		result, err = appendNormalizedRange(s.factory, result, next)
		if err != nil {
			return nil, err
		}
	}
	return &SortedRangeSet{factory: s.factory, typ: s.typ, ranges: result, includesNaN: s.includesNaN || rightSet.includesNaN}, nil
}

// Intersect visits the overlap of each pair of current intervals, advancing the
// interval whose upper endpoint finishes first (both on ties). NaN membership is
// AND. As with Union, input Types must match and nil/zero sets assert.
// After conversion, work is O(m+n) endpoint comparisons/boundary checks,
// with O(k) result storage for k output ranges. Payload costs depend on the domain. Results own their
// slices and share immutable Values; no re-sorting or enumeration occurs.
func (s *SortedRangeSet) Intersect(other value_set.ValueSet) (value_set.ValueSet, error) {
	s.assertConstructed()
	assertions.NotNil(other)
	if !s.typ.EqualsTo(other.Type()) {
		return nil, fmt.Errorf("%w: intersect", value_set.ErrIncompatibleTypes)
	}
	if finite, ok := other.(value_set.FiniteValueSet); ok {
		return internal.Apply(s.factory, finite, func(view value_set.ValueSet) (value_set.ValueSet, error) { return view.Intersect(s) })
	}
	rightSet, err := asSortedRangeSet(s.factory, other)
	if err != nil {
		return nil, err
	}
	result := make([]*ranges.Range, 0)
	for i, j := 0, 0; i < len(s.ranges) && j < len(rightSet.ranges); {
		left, right := s.ranges[i], rightSet.ranges[j]
		lowerOrder, err := ranges.CompareLowerEndpoints(left.Lower(), right.Lower())
		if err != nil {
			return nil, err
		}
		upperOrder, err := ranges.CompareUpperEndpoints(left.Upper(), right.Upper())
		if err != nil {
			return nil, err
		}
		lower, upper := left.Lower(), left.Upper()
		if lowerOrder < 0 {
			lower = right.Lower()
		}
		if upperOrder > 0 {
			upper = right.Upper()
		}
		overlap, err := ranges.NewRange(s.factory, s.typ, lower, upper)
		if err != nil {
			return nil, err
		}
		if r, present := overlap.Get(); present {
			// Each input separates its intervals by a nonempty gap. Their intersections
			// therefore remain sorted, disjoint and nonadjoining without a merging pass.
			result = append(result, r)
		}
		if upperOrder <= 0 {
			i++
		}
		if upperOrder >= 0 {
			j++
		}
	}
	return &SortedRangeSet{factory: s.factory, typ: s.typ, ranges: result, includesNaN: s.includesNaN && rightSet.includesNaN}, nil
}

// Difference computes A intersect complement(B). This splits ranges at excluded
// intervals without enumerating values; float NaN membership is A && !B.
// After any conversion, work is O(m+n) boundary checks/comparisons,
// with O(n+k) temporary/result storage.
func (s *SortedRangeSet) Difference(other value_set.ValueSet) (value_set.ValueSet, error) {
	s.assertConstructed()
	assertions.NotNil(other)
	if !s.typ.EqualsTo(other.Type()) {
		return nil, fmt.Errorf("%w: difference", value_set.ErrIncompatibleTypes)
	}
	if finite, ok := other.(value_set.FiniteValueSet); ok {
		if finite.IsInclusion() {
			result, err := s.WithoutPoints(finite.Values())
			if err != nil {
				return nil, err
			}
			return result, nil
		}
		inclusion, err := internal.Apply(s.factory, finite, func(view value_set.ValueSet) (value_set.ValueSet, error) { return view.Complement() })
		if err != nil {
			return nil, err
		}
		return inclusion.Intersect(s)
	}
	right, err := asSortedRangeSet(s.factory, other)
	if err != nil {
		return nil, err
	}
	complement, err := right.complement(s.factory)
	if err != nil {
		return nil, err
	}
	return s.Intersect(complement)
}

func (s *SortedRangeSet) IsSubsetOf(other value_set.ValueSet) (bool, error) {
	difference, err := s.Difference(other)
	if err != nil {
		return false, err
	}
	return difference.IsEmpty()
}

func (s *SortedRangeSet) IsDisjointFrom(other value_set.ValueSet) (bool, error) {
	intersection, err := s.Intersect(other)
	if err != nil {
		return false, err
	}
	return intersection.IsEmpty()
}

// EqualsTo compares exact regions through both directional differences. Endpoint
// spellings may differ, especially for discrete VARCHAR and floating domains.
func (s *SortedRangeSet) EqualsTo(other value_set.ValueSet) (bool, error) {
	difference, err := s.Difference(other)
	if err != nil {
		return false, err
	}
	empty, err := difference.IsEmpty()
	if err != nil || !empty {
		return false, err
	}
	reverse, err := internal.Apply(s.factory, other, func(view value_set.ValueSet) (value_set.ValueSet, error) { return view.Difference(s) })
	if err != nil {
		return false, err
	}
	return reverse.IsEmpty()
}

// appendNormalizedRange appends a compatible nonempty range whose lower bound
// follows the last stored lower bound, merging the last range when connected.
// The caller owns the slice; the Range instances remain immutable.
func appendNormalizedRange(factory valueinterfaces.ValueFactory, result []*ranges.Range, next *ranges.Range) ([]*ranges.Range, error) {
	if len(result) == 0 {
		return append(result, next), nil
	}
	last := result[len(result)-1]
	connected, err := rangesConnected(factory, last, next)
	if err != nil {
		return nil, err
	}
	if !connected {
		return append(result, next), nil
	}
	order, err := ranges.CompareUpperEndpoints(last.Upper(), next.Upper())
	if err != nil {
		return nil, err
	}
	if order < 0 {
		span, err := ranges.NewRange(factory, last.Type(), last.Lower(), next.Upper())
		if err != nil {
			return nil, err
		}
		merged, present := span.Get()
		if !present {
			panic("value_set: connected ranges must have a nonempty span")
		}
		result[len(result)-1] = merged
	}
	return result, nil
}
