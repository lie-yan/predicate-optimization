package equatable_value_set

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/internal"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

func (s *EquatableValueSet) Union(other value_set.ValueSet) (value_set.ValueSet, error) {
	s.assertConstructed()
	assertions.NotNil(other)
	if !s.typ.EqualsTo(other.Type()) {
		return nil, fmt.Errorf("%w: union", value_set.ErrIncompatibleTypes)
	}
	switch right := other.(type) {
	case *EquatableValueSet:
		return unionEquatable(s, right), nil
	case *sorted_range_set.SortedRangeSet:
		if s.isInclusion {
			result, err := internal.Apply(s.factory, right, func(view value_set.ValueSet) (value_set.ValueSet, error) {
				result, err := view.(*sorted_range_set.SortedRangeSet).WithPoints(s.elements.values)
				if err != nil {
					return nil, err
				}
				return result, nil
			})
			if err != nil {
				return nil, err
			}
			return result, nil
		}
		converted, err := s.ToSortedRangeSet()
		if err != nil {
			return nil, err
		}
		return converted.Union(right)
	default:
		return nil, fmt.Errorf("%w: union with %T", value_set.ErrUnsupported, other)
	}
}

func (s *EquatableValueSet) Intersect(other value_set.ValueSet) (value_set.ValueSet, error) {
	s.assertConstructed()
	assertions.NotNil(other)
	if !s.typ.EqualsTo(other.Type()) {
		return nil, fmt.Errorf("%w: intersect", value_set.ErrIncompatibleTypes)
	}
	switch right := other.(type) {
	case *EquatableValueSet:
		return intersectEquatable(s, right), nil
	case *sorted_range_set.SortedRangeSet:
		if s.isInclusion {
			return s.filterByRange(right, true)
		}
		result, err := internal.Apply(s.factory, right, func(view value_set.ValueSet) (value_set.ValueSet, error) {
			result, err := view.(*sorted_range_set.SortedRangeSet).WithoutPoints(s.elements.values)
			if err != nil {
				return nil, err
			}
			return result, nil
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%w: intersect with %T", value_set.ErrUnsupported, other)
	}
}

func (s *EquatableValueSet) Difference(other value_set.ValueSet) (value_set.ValueSet, error) {
	s.assertConstructed()
	assertions.NotNil(other)
	if !s.typ.EqualsTo(other.Type()) {
		return nil, fmt.Errorf("%w: difference", value_set.ErrIncompatibleTypes)
	}
	switch right := other.(type) {
	case *EquatableValueSet:
		// A \ B = A intersect complement(B); the temporary view is immutable.
		complement := &EquatableValueSet{factory: s.factory, typ: right.typ, elements: right.elements, isInclusion: !right.isInclusion}
		return intersectEquatable(s, complement), nil
	case *sorted_range_set.SortedRangeSet:
		if s.isInclusion {
			return s.filterByRange(right, false)
		}
		converted, err := s.ToSortedRangeSet()
		if err != nil {
			return nil, err
		}
		return converted.Difference(right)
	default:
		return nil, fmt.Errorf("%w: difference with %T", value_set.ErrUnsupported, other)
	}
}

func (s *EquatableValueSet) IsSubsetOf(other value_set.ValueSet) (bool, error) {
	difference, err := s.Difference(other)
	if err != nil {
		return false, err
	}
	return difference.IsEmpty()
}

func (s *EquatableValueSet) IsDisjointFrom(other value_set.ValueSet) (bool, error) {
	intersection, err := s.Intersect(other)
	if err != nil {
		return false, err
	}
	return intersection.IsEmpty()
}

func (s *EquatableValueSet) EqualsTo(other value_set.ValueSet) (bool, error) {
	leftDifference, err := s.Difference(other)
	if err != nil {
		return false, err
	}
	empty, err := leftDifference.IsEmpty()
	if err != nil || !empty {
		return false, err
	}
	rightDifference, err := internal.Apply(s.factory, other, func(view value_set.ValueSet) (value_set.ValueSet, error) { return view.Difference(s) })
	if err != nil {
		return false, err
	}
	return rightDifference.IsEmpty()
}

// Inputs are constructed, normalized, compatible sets. All results own their
// slices and share immutable Values. No constructor pass or universe enumeration
// is needed: finite helper operations preserve uniqueness and normalization.
func unionEquatable(a, b *EquatableValueSet) *EquatableValueSet {
	result := &EquatableValueSet{factory: a.factory, typ: a.typ}
	switch {
	case a.isInclusion && b.isInclusion:
		result.isInclusion = true
		result.elements = a.elements.union(b.elements)
	case a.isInclusion && !b.isInclusion:
		result.elements = b.elements.difference(a.elements)
	case !a.isInclusion && b.isInclusion:
		result.elements = a.elements.difference(b.elements)
	default:
		result.elements = a.elements.intersect(b.elements)
	}
	return result
}

func intersectEquatable(a, b *EquatableValueSet) *EquatableValueSet {
	result := &EquatableValueSet{factory: a.factory, typ: a.typ, isInclusion: true}
	switch {
	case a.isInclusion && b.isInclusion:
		result.elements = a.elements.intersect(b.elements)
	case a.isInclusion && !b.isInclusion:
		result.elements = a.elements.difference(b.elements)
	case !a.isInclusion && b.isInclusion:
		result.elements = b.elements.difference(a.elements)
	default:
		result.isInclusion = false
		result.elements = a.elements.union(b.elements)
	}
	return result
}
