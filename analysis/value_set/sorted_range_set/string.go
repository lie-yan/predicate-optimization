package sorted_range_set

import (
	"strings"

	"github.com/lie-yan/predicate-optimization/types"
)

// String joins stored normalized ranges and the optional NaN component without
// the element Type. ALL denotes the full non-null universe, including NaN for
// floats; the fully unbounded ordered float region alone remains (-∞,+∞).
// Formatting performs no set conversion or universe-coverage query.
func (s *SortedRangeSet) String() string {
	s.assertConstructed()
	if len(s.ranges) == 0 {
		if s.includesNaN {
			return "{NaN}"
		}
		return "{}"
	}
	if len(s.ranges) == 1 && s.ranges[0].Lower().IsUnbounded() && s.ranges[0].Upper().IsUnbounded() {
		if _, floating := s.typ.(*types.FloatType); !floating || s.includesNaN {
			return "ALL"
		}
	}
	var output strings.Builder
	for i, r := range s.ranges {
		if i > 0 {
			output.WriteString(" ∪ ")
		}
		output.WriteString(r.String())
	}
	if s.includesNaN {
		output.WriteString(" ∪ {NaN}")
	}
	return output.String()
}
