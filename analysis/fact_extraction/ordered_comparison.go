package fact_extraction

import (
	"math"

	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// reverseComparison exchanges operand positions, preserving equality operators.
// Its input must be an admitted comparison operator.
func reverseComparison(operator foundation.OperatorType) foundation.OperatorType {
	switch operator {
	case foundation.LessThan:
		return foundation.GreaterThan
	case foundation.LessThanOrEqual:
		return foundation.GreaterThanOrEqual
	case foundation.GreaterThan:
		return foundation.LessThan
	case foundation.GreaterThanOrEqual:
		return foundation.LessThanOrEqual
	default:
		return operator
	}
}

// orderedComparisonSet derives the global non-null true region of an admitted
// ordered comparison with a non-null literal on the right. NaN inputs never
// satisfy ordering, and a NaN literal gives an empty region for every operator.
func orderedComparisonSet(factory valueinterfaces.ValueFactory, typ types.Type, operator foundation.OperatorType, literal valueinterfaces.Value) (*sets.SortedRangeSet, error) {
	if _, floating := literal.Type().(*types.FloatType); floating && math.IsNaN(literal.(valueinterfaces.FloatValue).Float()) {
		return sets.NewSortedRangeSet(factory, typ, nil, false)
	}
	inclusive := operator == foundation.LessThanOrEqual || operator == foundation.GreaterThanOrEqual
	endpoint, err := ranges.NewRangeEndpoint(literal, inclusive)
	if err != nil {
		return nil, err
	}
	lower, upper := ranges.RangeEndpoint{}, ranges.RangeEndpoint{}
	switch operator {
	case foundation.LessThan, foundation.LessThanOrEqual:
		upper = endpoint
	case foundation.GreaterThan, foundation.GreaterThanOrEqual:
		lower = endpoint
	default:
		panic("fact_extraction: expected admitted ordered comparison")
	}
	interval, err := ranges.NewRange(factory, typ, lower, upper)
	if err != nil {
		return nil, err
	}
	var intervals []*ranges.Range
	if r, present := interval.Get(); present {
		intervals = []*ranges.Range{r}
	}
	// Range construction handles extrema, discrete neighbors and empty intervals;
	// even a fully unbounded float range excludes NaN.
	return sets.NewSortedRangeSet(factory, typ, intervals, false)
}
