package equatable_value_set

import (
	"strings"

	valuesetinternal "github.com/lie-yan/predicate-optimization/analysis/value_set/internal"
	"github.com/lie-yan/predicate-optimization/types"
)

// String renders stored inclusions or exclusions without the element Type.
// Empty exclusions print ALL; other sets retain their representation rather
// than performing universe-coverage queries. Entries use ascending display
// order, including false before true and NaN last, without changing storage.
func (s *EquatableValueSet) String() string {
	s.assertConstructed()
	entries := s.elements.values
	if !s.isInclusion && len(entries) == 0 {
		return "ALL"
	}
	var output strings.Builder
	if !s.isInclusion {
		output.WriteString("ALL \\ ")
	}
	output.WriteByte('{')
	if _, boolean := s.typ.(*types.BoolType); boolean && len(entries) == 2 {
		// Two unique Boolean entries necessarily contain both values; their stored
		// first-occurrence order remains unchanged.
		output.WriteString("false, true")
	} else {
		for i, value := range entries {
			if i > 0 {
				output.WriteString(", ")
			}
			output.WriteString(valuesetinternal.FormatValue(value))
		}
	}
	output.WriteByte('}')
	return output.String()
}
