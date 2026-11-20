// Package interfaces defines caller-implementable typed value contracts.
package interfaces

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

// Value describes one fully known typed value, supplied by the caller or a
// bundled implementation. Values, Types and child values must be valid, immutable
// and non-nil; they are not revalidated on access. Composite graphs are acyclic.
// Unavailable input is not a Value or a known null. For non-null values, Type
// determines the required payload interface. Known nulls need no payload view.
type Value interface {
	Type() types.Type
	IsNull() bool
	// EqualsTo compares complete typed structural content, not SQL equality.
	// Inputs must be valid, immutable and non-nil. Implementations own their
	// comparison algorithm and must agree across representations: full Types,
	// nullness and ordered payload content determine equality. Canonical NaNs
	// are equal; signed zeros differ. Composite children use their EqualsTo.
	// Do not assume the other value has the same concrete Go representation.
	EqualsTo(other Value) bool
	// WithNullability copies a valid immutable value, changing only outer
	// nullability and sharing immutable payloads and children. Known nulls cannot
	// become NonNull. Invalid nullability returns an error; nil receivers panic.
	WithNullability(foundation.Nullability) (Value, error)
}
