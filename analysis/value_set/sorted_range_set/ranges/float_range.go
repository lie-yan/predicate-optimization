package ranges

import (
	"fmt"
	"math"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// The ordered float universe includes infinities but excludes NaN. The latter
// belongs to the future SortedRangeSet's separate membership component.
func newFloatRange(factory valueinterfaces.ValueFactory, typ *types.FloatType, lower, upper RangeEndpoint) (foundation.Optional[*Range], error) {
	empty := foundation.Optional[*Range]{}
	// Check both endpoints before any emptiness shortcut; normalization copies
	// only negative-zero endpoints, leaving source Values unchanged.
	lower, err := normalizeFloatEndpoint(factory, typ, lower)
	if err != nil {
		return empty, err
	}
	upper, err = normalizeFloatEndpoint(factory, typ, upper)
	if err != nil {
		return empty, err
	}
	low, hasLow := lower.Value().Get()
	high, hasHigh := upper.Value().Get()
	if hasLow && !lower.IsInclusive() && math.IsInf(low.(valueinterfaces.FloatValue).Float(), 1) {
		return empty, nil
	}
	if hasHigh && !upper.IsInclusive() && math.IsInf(high.(valueinterfaces.FloatValue).Float(), -1) {
		return empty, nil
	}
	if hasLow && hasHigh {
		order, err := compareRangeValues(low, high)
		if err != nil {
			return empty, err
		}
		if order > 0 || (order == 0 && (!lower.IsInclusive() || !upper.IsInclusive())) {
			return empty, nil
		}
		if order < 0 && !lower.IsInclusive() && !upper.IsInclusive() && floatAdjacent(typ, low.(valueinterfaces.FloatValue).Float(), high.(valueinterfaces.FloatValue).Float()) {
			return empty, nil
		}
	}
	if hasLow && lower.IsInclusive() && math.IsInf(low.(valueinterfaces.FloatValue).Float(), -1) {
		lower = RangeEndpoint{}
	}
	if hasHigh && upper.IsInclusive() && math.IsInf(high.(valueinterfaces.FloatValue).Float(), 1) {
		upper = RangeEndpoint{}
	}
	return foundation.Some(&Range{typ: typ, lower: lower, upper: upper}), nil
}

func normalizeFloatEndpoint(factory valueinterfaces.ValueFactory, typ *types.FloatType, endpoint RangeEndpoint) (RangeEndpoint, error) {
	value, present := endpoint.Value().Get()
	if !present {
		return endpoint, nil
	}
	number := value.(valueinterfaces.FloatValue).Float()
	if math.IsNaN(number) {
		return RangeEndpoint{}, fmt.Errorf("%w: NaN range endpoint", value_set.ErrInvalidInput)
	}
	if number == 0 && math.Signbit(number) {
		normalized, err := factory.NewFloat(typ, 0)
		if err != nil {
			return RangeEndpoint{}, err
		}
		return RangeEndpoint{value: foundation.Some[valueinterfaces.Value](normalized), inclusive: endpoint.inclusive}, nil
	}
	return endpoint, nil
}

// floatNeighbor requires a representable non-NaN value of typ. Absence means
// stepping beyond an infinity. Signed zeros share one position in this ordering.
func floatNeighbor(typ *types.FloatType, number float64, upward bool) (float64, bool) {
	direction := math.Inf(1)
	if !upward {
		direction = math.Inf(-1)
	}
	if number == direction {
		return 0, false
	}
	var neighbor float64
	if typ.FloatWidth() == types.Float32 {
		neighbor = float64(math.Nextafter32(float32(number), float32(direction)))
	} else {
		neighbor = math.Nextafter(number, direction)
	}
	if neighbor == 0 {
		neighbor = 0
	} // Canonical positive zero, including from below.
	return neighbor, true
}

// Inputs belong to typ's non-NaN universe. No real-number density assumption or
// float64 stepping is used for Float32 values.
func floatAdjacent(typ *types.FloatType, lower, upper float64) bool {
	successor, present := floatNeighbor(typ, lower, true)
	return present && successor == upper
}
