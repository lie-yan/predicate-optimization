package value_set

import valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"

// FiniteValueSet exists only to resolve the cyclic package dependency between
// equatable_value_set and sorted_range_set during mixed-operation dispatch.
// It describes a set through finite included or excluded entries.
// The represented set itself may be infinite when IsInclusion is false.
// This interface describes membership independently of storage or range conversion.
type FiniteValueSet interface {
	ValueSet
	// Values returns an independent slice of unique, immutable, non-null entries
	// with normalized Types equal to Type(). Entries are members when IsInclusion
	// is true and exclusions from Type's non-null universe otherwise.
	Values() []valueinterfaces.Value
	// IsInclusion reports whether the set uses an inclusion (true) or exclusion
	// (false) representation for Values within the non-null domain universe.
	// Empty entries denote the empty set when true and the universal set when false.
	IsInclusion() bool
}
