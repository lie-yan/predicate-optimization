package sorted_range_set

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/internal"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// rangeConvertible is an explicit exact-conversion capability, not part of
// ValueSet. EquatableValueSet implements it without this package importing the
// finite implementation. Conversion must preserve Type and membership and must
// return a constructed set. It terminates before any binary range operation.
type rangeConvertible interface {
	ToSortedRangeSet() (*SortedRangeSet, error)
}

// The caller has checked operand Type compatibility and non-nil preconditions.
// Existing sorted sets are borrowed unchanged. Callers must pass the selected
// factory explicitly when synthesizing through a borrowed operand; only new
// conversions inherit it here.
func asSortedRangeSet(factory valueinterfaces.ValueFactory, other value_set.ValueSet) (*SortedRangeSet, error) {
	if result, ok := other.(*SortedRangeSet); ok {
		return result, nil
	}
	_, ok := other.(rangeConvertible)
	if !ok {
		return nil, fmt.Errorf("%w: range conversion for %T", value_set.ErrUnsupported, other)
	}
	converted, err := internal.Apply(factory, other, func(view value_set.ValueSet) (value_set.ValueSet, error) {
		result, err := view.(rangeConvertible).ToSortedRangeSet()
		if err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	result := converted.(*SortedRangeSet)
	result.assertConstructed()
	if !result.typ.EqualsTo(other.Type()) {
		return nil, fmt.Errorf("%w: converted range set", value_set.ErrIncompatibleTypes)
	}
	return result, nil
}
