package ranges

import valuesetinternal "github.com/lie-yan/predicate-optimization/analysis/value_set/internal"

// String describes the stored normalized interval without its Type. Unbounded
// sides use -∞/+∞; actual float infinities use -Inf/+Inf. Equal closed bounded
// endpoints print as a singleton. This diagnostic form is not serialization.
func (r *Range) String() string {
	r.assertConstructed()
	low, hasLow := r.lower.Value().Get()
	high, hasHigh := r.upper.Value().Get()
	if hasLow && hasHigh && r.lower.IsInclusive() && r.upper.IsInclusive() && low.EqualsTo(high) {
		return "{" + valuesetinternal.FormatValue(low) + "}"
	}
	left, right := "(", ")"
	lower, upper := "-∞", "+∞"
	if hasLow {
		lower = valuesetinternal.FormatValue(low)
		if r.lower.IsInclusive() {
			left = "["
		}
	}
	if hasHigh {
		upper = valuesetinternal.FormatValue(high)
		if r.upper.IsInclusive() {
			right = "]"
		}
	}
	return left + lower + "," + upper + right
}
