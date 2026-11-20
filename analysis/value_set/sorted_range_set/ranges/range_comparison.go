package ranges

import (
	"cmp"
	"fmt"
	"math"
	"strings"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"

	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// CompareLowerEndpoints orders endpoints as lower bounds. Inputs must come from
// compatible validated ranges; an unbounded endpoint sorts before bounded ones.
func CompareLowerEndpoints(left, right RangeEndpoint) (int, error) {
	return compareRangeEndpoints(left, right, true)
}

// CompareUpperEndpoints orders endpoints as upper bounds. Inputs must come from
// compatible validated ranges; an unbounded endpoint sorts after bounded ones.
func CompareUpperEndpoints(left, right RangeEndpoint) (int, error) {
	return compareRangeEndpoints(left, right, false)
}

// compareRangeEndpoints orders endpoints as lower bounds when lower is true,
// otherwise as upper bounds. Unbounded sides sort first for lower bounds and last
// for upper bounds; at equal values, inclusive lower bounds precede exclusive ones,
// while inclusive upper bounds follow exclusive ones.
func compareRangeEndpoints(left, right RangeEndpoint, lower bool) (int, error) {
	a, hasA := left.Value().Get()
	b, hasB := right.Value().Get()
	if !hasA && !hasB {
		return 0, nil
	}
	if !hasA {
		if lower {
			return -1, nil
		}
		return 1, nil
	}
	if !hasB {
		if lower {
			return 1, nil
		}
		return -1, nil
	}
	order, err := compareRangeValues(a, b)
	if err != nil || order != 0 {
		return order, err
	}
	if left.IsInclusive() == right.IsInclusive() {
		return 0, nil
	}
	// Inclusive lower bounds precede exclusive lower bounds; inclusive upper
	// bounds follow exclusive upper bounds at the same value.
	if left.IsInclusive() == lower {
		return -1, nil
	}
	return 1, nil
}

func compareRangeValues(left, right valueinterfaces.Value) (int, error) {
	typ := left.Type()
	if !typ.EqualsTo(right.Type()) {
		return 0, fmt.Errorf("%w: range values", value_set.ErrIncompatibleTypes)
	}
	switch typ.(type) {
	case *types.IntType:
		a := left.(valueinterfaces.IntegerValue)
		return cmp.Compare(a.Integer(), right.(valueinterfaces.IntegerValue).Integer()), nil
	case *types.FloatType:
		a := left.(valueinterfaces.FloatValue).Float()
		b := right.(valueinterfaces.FloatValue).Float()
		if math.IsNaN(a) || math.IsNaN(b) {
			return 0, fmt.Errorf("%w: NaN is not an ordered range value", value_set.ErrInvalidInput)
		}
		return cmp.Compare(a, b), nil
	case *types.VarCharType:
		a := left.(valueinterfaces.VarCharValue)
		return strings.Compare(a.VarChar(), right.(valueinterfaces.VarCharValue).VarChar()), nil
	default:
		return 0, fmt.Errorf("%w: range comparison for %T", value_set.ErrUnsupported, left)
	}
}
