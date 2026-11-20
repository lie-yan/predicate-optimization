package sorted_range_set

import (
	"fmt"
	"math"
	"slices"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	ranges "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// SortedRangeSet owns sorted, nonempty, nonoverlapping and nonadjoining ranges.
// For floats, NaN membership is separate from the ordered ranges. The zero struct
// is invalid. It implements value_set.ValueSet for the admitted ordered domains.
type SortedRangeSet struct {
	value_set.ValueSetMarker
	factory     valueinterfaces.ValueFactory
	typ         types.Type
	ranges      []*ranges.Range
	includesNaN bool
}

var _ value_set.ValueSet = (*SortedRangeSet)(nil)

// NewSortedRangeSet copies and normalizes input, sharing immutable Ranges/Values.
// Ranges are already nonempty; omit absent NewRange results before calling.
// Nil input slices represent no ordered values. Nil range entries and zero ranges.Range
// structs assert. All input domains are checked before sorting or merging.
// includesNaN is valid only for float domains. Construction uses O(n log n)
// boundary comparisons plus O(n) gap checks, without universe enumeration.
func NewSortedRangeSet(factory valueinterfaces.ValueFactory, typ types.Type, input []*ranges.Range, includesNaN bool) (*SortedRangeSet, error) {
	assertions.NotNil(factory)
	assertions.NotNil(typ)
	if !typ.IsOrderable() {
		return nil, fmt.Errorf("%w: range-set type is not orderable", value_set.ErrUnsupported)
	}
	switch typ.(type) {
	case *types.IntType, *types.VarCharType:
		if includesNaN {
			return nil, fmt.Errorf("%w: NaN membership requires float type", value_set.ErrInvalidInput)
		}
	case *types.FloatType:
	default:
		return nil, fmt.Errorf("%w: range-set domain %s", value_set.ErrUnsupported, typ)
	}
	normalized, err := typ.WithNullability(foundation.NonNull)
	if err != nil {
		return nil, err
	}
	owned := make([]*ranges.Range, len(input))
	for i, r := range input {
		if !normalized.EqualsTo(r.Type()) {
			return nil, fmt.Errorf("%w: range %d", value_set.ErrIncompatibleTypes, i)
		}
		owned[i] = r
	}
	var comparisonErr error
	slices.SortStableFunc(owned, func(a, b *ranges.Range) int {
		if comparisonErr != nil {
			return 0
		}
		order, err := ranges.CompareLowerEndpoints(a.Lower(), b.Lower())
		if err != nil {
			comparisonErr = err
			return 0
		}
		return order
	})
	if comparisonErr != nil {
		return nil, comparisonErr
	}
	merged := make([]*ranges.Range, 0, len(owned))
	for _, r := range owned {
		merged, err = appendNormalizedRange(factory, merged, r)
		if err != nil {
			return nil, err
		}
	}
	return &SortedRangeSet{factory: factory, typ: normalized, ranges: merged, includesNaN: includesNaN}, nil
}

// rangesConnected assumes compatible nonempty ranges ordered by their lower
// bounds. The uncovered gap reverses inclusion at each bordering endpoint.
// NewRange detects reversed, empty and discrete-adjacent gaps for every domain.
func rangesConnected(factory valueinterfaces.ValueFactory, left, right *ranges.Range) (bool, error) {
	upper, lower := left.Upper(), right.Lower()
	if upper.IsUnbounded() || lower.IsUnbounded() {
		return true, nil
	}
	gapLow, err := flipBoundedEndpoint(upper)
	if err != nil {
		return false, err
	}
	gapHigh, err := flipBoundedEndpoint(lower)
	if err != nil {
		return false, err
	}
	gap, err := ranges.NewRange(factory, left.Type(), gapLow, gapHigh)
	if err != nil {
		return false, err
	}
	_, present := gap.Get()
	return !present, nil
}

func (s *SortedRangeSet) Type() types.Type {
	s.assertConstructed()
	return s.typ
}

// Ranges returns a slice copy sharing immutable ranges. NaN is never a range.
func (s *SortedRangeSet) Ranges() []*ranges.Range {
	s.assertConstructed()
	result := make([]*ranges.Range, len(s.ranges))
	copy(result, s.ranges)
	return result
}

func (s *SortedRangeSet) IncludesNaN() bool {
	s.assertConstructed()
	return s.includesNaN
}

// Contains checks mathematical set membership, not provider SQL equality.
// Values must be valid. Null Values return ErrInvalidInput; domain mismatches return
// ErrIncompatibleTypes. Nil input asserts. Outer-nullable concrete values are
// admitted; NaN consults the flag and signed zeros compare equal.
func (s *SortedRangeSet) Contains(value valueinterfaces.Value) (bool, error) {
	s.assertConstructed()
	assertions.NotNil(value)
	if value.IsNull() {
		return false, fmt.Errorf("%w: expected valid non-null range-set member", value_set.ErrInvalidInput)
	}
	if !s.typ.EqualsToIgnoringNullability(value.Type()) {
		return false, fmt.Errorf("%w: range-set member", value_set.ErrIncompatibleTypes)
	}
	if _, floating := s.typ.(*types.FloatType); floating && math.IsNaN(value.(valueinterfaces.FloatValue).Float()) {
		return s.includesNaN, nil
	}

	point, err := ranges.NewRangeEndpoint(value, true)
	if err != nil {
		return false, err
	}
	// Find the last range whose lower bound admits this value.
	lo, hi := 0, len(s.ranges)
	for lo < hi {
		middle := lo + (hi-lo)/2
		order, err := ranges.CompareLowerEndpoints(s.ranges[middle].Lower(), point)
		if err != nil {
			return false, err
		}
		if order <= 0 {
			lo = middle + 1
		} else {
			hi = middle
		}
	}
	if lo == 0 {
		return false, nil
	}
	order, err := ranges.CompareUpperEndpoints(s.ranges[lo-1].Upper(), point)
	if err != nil {
		return false, err
	}
	return order >= 0, nil
}

func (s *SortedRangeSet) IsEmpty() (bool, error) {
	s.assertConstructed()
	return len(s.ranges) == 0 && !s.includesNaN, nil
}

func (s *SortedRangeSet) IsUniversal() (bool, error) {
	s.assertConstructed()
	if len(s.ranges) != 1 || !s.ranges[0].Lower().IsUnbounded() || !s.ranges[0].Upper().IsUnbounded() {
		return false, nil
	}
	if _, floating := s.typ.(*types.FloatType); floating {
		return s.includesNaN, nil
	}
	return true, nil
}

func (s *SortedRangeSet) assertConstructed() {
	if s == nil || s.typ == nil || s.factory == nil {
		panic("value_set: expected constructed SortedRangeSet")
	}
}
