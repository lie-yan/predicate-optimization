// Package interfaces defines caller-provided interpreter dependencies.
package interfaces

import (
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Environment resolves immutable values by caller-defined scoped identity.
// Absence means unavailable, including for non-nullable columns. Known SQL NULL
// is a present typed Value. Errors are separate from both absence and SQL NULL.
// IDs need not be comparable; implementations use their EqualsTo contract or
// provider-defined indexing, never diagnostic names as identity keys.
// Lookup is a benign binding lookup; implementations keep bindings stable during
// evaluation. Present Values must be valid, immutable and non-nil; a present nil
// binding violates the precondition and triggers an assertion.
// An environment must agree with the identity domain of the input expressions.
type Environment interface {
	Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error)
}
