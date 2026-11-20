package ranges

import (
	"math"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func newIntegerRange(factory valueinterfaces.ValueFactory, integer *types.IntType, lower, upper RangeEndpoint) (foundation.Optional[*Range], error) {
	empty := foundation.Optional[*Range]{}
	minimum, maximum := integerExtrema(integer.IntegerWidth())
	lower, lowerEmpty, err := normalizeIntegerEndpoint(factory, integer, lower, true, minimum, maximum)
	if err != nil {
		return empty, err
	}
	upper, upperEmpty, err := normalizeIntegerEndpoint(factory, integer, upper, false, minimum, maximum)
	if err != nil {
		return empty, err
	}
	if lowerEmpty || upperEmpty {
		return empty, nil
	}
	low, hasLow := lower.Value().Get()
	high, hasHigh := upper.Value().Get()
	if hasLow && hasHigh {
		order, err := compareRangeValues(low, high)
		if err != nil {
			return empty, err
		}
		if order > 0 {
			return empty, nil
		}
	}
	return foundation.Some(&Range{typ: integer, lower: lower, upper: upper}), nil
}

func integerExtrema(width types.IntWidth) (int64, int64) {
	if width == types.Int64 {
		return math.MinInt64, math.MaxInt64
	}
	maximum := int64(1)<<(width-1) - 1
	return -maximum - 1, maximum
}

// The Boolean result indicates that this side alone excludes the entire domain.
func normalizeIntegerEndpoint(factory valueinterfaces.ValueFactory, typ *types.IntType, endpoint RangeEndpoint, lower bool, minimum, maximum int64) (RangeEndpoint, bool, error) {
	value, bounded := endpoint.Value().Get()
	if !bounded {
		return endpoint, false, nil
	}
	number := value.(valueinterfaces.IntegerValue).Integer()
	if !endpoint.IsInclusive() {
		if lower {
			if number == maximum {
				return RangeEndpoint{}, true, nil
			}
			number++
		} else {
			if number == minimum {
				return RangeEndpoint{}, true, nil
			}
			number--
		}
	}
	if (lower && number == minimum) || (!lower && number == maximum) {
		return RangeEndpoint{}, false, nil
	}
	if endpoint.IsInclusive() {
		return endpoint, false, nil
	}
	shifted, err := factory.NewInteger(typ, number)
	if err != nil {
		return RangeEndpoint{}, false, err
	}
	return RangeEndpoint{value: foundation.Some[valueinterfaces.Value](shifted), inclusive: true}, false, nil
}
