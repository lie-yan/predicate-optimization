package ranges

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Inputs are validated endpoints of this domain. Preserve their open/closed
// cuts, avoiding predecessor/successor allocation for arbitrarily large bounds.
func newVarCharRange(typ *types.VarCharType, lower, upper RangeEndpoint) (foundation.Optional[*Range], error) {
	empty := foundation.Optional[*Range]{}
	low, hasLow := lower.Value().Get()
	high, hasHigh := upper.Value().Get()
	if hasLow && !lower.IsInclusive() && isVarCharMaximum(typ, low.(valueinterfaces.VarCharValue).VarChar()) {
		return empty, nil
	}
	if hasHigh && !upper.IsInclusive() && high.(valueinterfaces.VarCharValue).VarChar() == "" {
		return empty, nil
	}
	if hasLow && hasHigh {
		a, b := low.(valueinterfaces.VarCharValue).VarChar(), high.(valueinterfaces.VarCharValue).VarChar()
		order, err := compareRangeValues(low, high)
		if err != nil {
			return empty, err
		}
		if order > 0 || (order == 0 && (!lower.IsInclusive() || !upper.IsInclusive())) {
			return empty, nil
		}
		if order < 0 && !lower.IsInclusive() && !upper.IsInclusive() && varCharAdjacent(typ, a, b) {
			return empty, nil
		}
	}
	// An inclusive minimum/maximum contributes no restriction in its direction.
	if hasLow && lower.IsInclusive() && low.(valueinterfaces.VarCharValue).VarChar() == "" {
		lower = RangeEndpoint{}
	}
	if hasHigh && upper.IsInclusive() && isVarCharMaximum(typ, high.(valueinterfaces.VarCharValue).VarChar()) {
		upper = RangeEndpoint{}
	}
	return foundation.Some(&Range{typ: typ, lower: lower, upper: upper}), nil
}

func isVarCharMaximum(typ *types.VarCharType, value string) bool {
	limit, bounded := typ.MaxBytes()
	if !bounded || uint64(len(value)) != limit {
		return false
	}
	for i := range len(value) {
		if value[i] != 0xff {
			return false
		}
	}
	return true
}

// varCharAdjacent reports whether upper is the immediate successor of lower.
// Inputs must be valid strings of typ. Work and storage depend on actual input
// lengths, never on a potentially enormous declared maximum byte length.
func varCharAdjacent(typ *types.VarCharType, lower, upper string) bool {
	limit, bounded := typ.MaxBytes()
	if !bounded || uint64(len(lower)) < limit {
		return len(upper) > 0 && len(upper)-1 == len(lower) && upper[:len(lower)] == lower && upper[len(lower)] == 0
	}
	// At the length limit, increment the rightmost non-ff byte and truncate.
	// If all bytes are ff, lower is the maximum and has no successor.
	for i := len(lower) - 1; i >= 0; i-- {
		if lower[i] != 0xff {
			return len(upper) == i+1 && upper[:i] == lower[:i] && upper[i] == lower[i]+1
		}
	}
	return false
}
