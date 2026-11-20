package reconstruction

import (
	"fmt"
	"math"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/internal/function_admission"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set/ranges"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// reconstructRanges prefers points and isolated exclusions before expanding
// intervals. For NaN-inclusive float regions it negates the ordered complement,
// so native comparisons never accidentally lose or introduce NaN membership.
func (r *ScalarReconstructor) reconstructRanges(summary *facts.PredicateSummary, set *sets.SortedRangeSet) (foundation.Optional[expressions.Expression], error) {
	absent := foundation.Optional[expressions.Expression]{}
	intervals := set.Ranges()
	if !set.IncludesNaN() {
		points, all, err := rangePoints(r.factory, intervals)
		if err != nil {
			return absent, err
		}
		if all {
			return r.reconstructDiscrete(summary, points, true)
		}
	}
	complement, err := set.Complement()
	if err != nil {
		return absent, err
	}
	excluded := complement.(*sets.SortedRangeSet)
	if !excluded.IncludesNaN() {
		points, all, err := rangePoints(r.factory, excluded.Ranges())
		if err != nil {
			return absent, err
		}
		if all {
			return r.reconstructDiscrete(summary, points, false)
		}
	}
	if set.IncludesNaN() {
		intervals = excluded.Ranges()
	}
	candidate, err := r.reconstructOrderedRanges(summary, intervals)
	if err != nil {
		return absent, err
	}
	if set.IncludesNaN() {
		candidate, err = expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{candidate})
		if err != nil {
			return absent, err
		}
	}
	return foundation.Some(candidate), nil
}

// rangePoints recognizes stored point intervals without enumerating domains.
// Empty lists are left to the empty/universal handling in Reconstruct.
func rangePoints(factory valueinterfaces.ValueFactory, intervals []*ranges.Range) ([]valueinterfaces.Value, bool, error) {
	points := make([]valueinterfaces.Value, 0, len(intervals))
	for _, interval := range intervals {
		value, present, err := rangePoint(factory, interval)
		if err != nil {
			return nil, false, err
		}
		if !present {
			return nil, false, nil
		}
		points = append(points, value)
	}
	return points, len(points) != 0, nil
}

// rangePoint compares a closed endpoint's canonical point range with the input.
// This also recognizes extrema whose inclusive bound normalized to unbounded.
// Open-only singleton intervals remain correct ordinary interval candidates.
func rangePoint(factory valueinterfaces.ValueFactory, interval *ranges.Range) (valueinterfaces.Value, bool, error) {
	endpoint := interval.Lower()
	if !endpoint.IsInclusive() {
		endpoint = interval.Upper()
	}
	value, bounded := endpoint.Value().Get()
	if !bounded || !endpoint.IsInclusive() {
		return nil, false, nil
	}
	point, err := ranges.NewRange(factory, interval.Type(), endpoint, endpoint)
	if err != nil {
		return nil, false, err
	}
	singleton, present := point.Get()
	if !present {
		return nil, false, nil
	}
	low, err := ranges.CompareLowerEndpoints(interval.Lower(), singleton.Lower())
	if err != nil {
		return nil, false, err
	}
	high, err := ranges.CompareUpperEndpoints(interval.Upper(), singleton.Upper())
	if err != nil {
		return nil, false, err
	}
	return value, low == 0 && high == 0, nil
}

func (r *ScalarReconstructor) reconstructOrderedRanges(summary *facts.PredicateSummary, intervals []*ranges.Range) (expressions.Expression, error) {
	terms := make([]expressions.Expression, 0, len(intervals))
	for _, interval := range intervals {
		point, isPoint, err := rangePoint(r.factory, interval)
		if err != nil {
			return nil, err
		}
		if isPoint {
			comparison, err := r.rangeComparison(summary, foundation.Equal, point)
			if err != nil {
				return nil, err
			}
			terms = append(terms, comparison)
			continue
		}
		var bounds []expressions.Expression
		if value, present := interval.Lower().Value().Get(); present {
			op := foundation.GreaterThan
			if interval.Lower().IsInclusive() {
				op = foundation.GreaterThanOrEqual
			}
			comparison, err := r.rangeComparison(summary, op, value)
			if err != nil {
				return nil, err
			}
			bounds = append(bounds, comparison)
		}
		if value, present := interval.Upper().Value().Get(); present {
			op := foundation.LessThan
			if interval.Upper().IsInclusive() {
				op = foundation.LessThanOrEqual
			}
			comparison, err := r.rangeComparison(summary, op, value)
			if err != nil {
				return nil, err
			}
			bounds = append(bounds, comparison)
		}
		if len(bounds) == 0 {
			// A fully unbounded float interval covers every ordered value, excluding
			// NaN. x >= -Inf preserves that distinction and still propagates null.
			typ, floating := interval.Type().(*types.FloatType)
			if !floating {
				return nil, fmt.Errorf("%w: unexpected universal ordered interval", value_set.ErrInvalidInput)
			}
			minimum, err := r.factory.NewFloat(typ, math.Inf(-1))
			if err != nil {
				return nil, err
			}
			comparison, err := r.rangeComparison(summary, foundation.GreaterThanOrEqual, minimum)
			if err != nil {
				return nil, err
			}
			bounds = append(bounds, comparison)
		}
		term, err := combineExpressions(foundation.And, bounds)
		if err != nil {
			return nil, err
		}
		terms = append(terms, term)
	}
	return combineExpressions(foundation.Or, terms)
}

func (r *ScalarReconstructor) rangeComparison(summary *facts.PredicateSummary, operator foundation.OperatorType, value valueinterfaces.Value) (expressions.Expression, error) {
	literal, err := expressions.NewLiteralExpression(value)
	if err != nil {
		return nil, err
	}
	return function_admission.ResolveComparison(r.context, operator, summary.Operand(), literal)
}

// combineExpressions avoids introducing a unary AND/OR around one term.
func combineExpressions(operator foundation.LogicalOperator, terms []expressions.Expression) (expressions.Expression, error) {
	if len(terms) == 1 {
		return terms[0], nil
	}
	return expressions.NewLogicalExpression(operator, terms)
}
