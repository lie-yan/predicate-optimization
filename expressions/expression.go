// Package expressions represents immutable typed scalar expressions.
// Nodes share immutable values and children; parent relationships belong to
// traversal contexts, not expression nodes.
package expressions

import (
	"github.com/lie-yan/predicate-optimization/types"
)

// Expression is a typed scalar computation, including Boolean predicates.
// Concrete pointer types identify variants. Implementations must remain valid
// and immutable after construction. Routine methods require non-nil inputs.
type Expression interface {
	Type() types.Type
	// Children returns ordered expression children in a caller-owned slice.
	Children() []Expression
	// ChildCount returns the number of ordered children without copying them.
	ChildCount() int
	// ChildAt returns the existing immutable child at a zero-based index without
	// allocating. The receiver must be valid and non-nil. Indexes outside
	// [0, ChildCount()) panic, including every index on a leaf. ChildAt, ChildCount
	// and Children must describe the same ordered children, also in wrappers.
	ChildAt(index int) Expression
	// WithChildren enforces variant-specific arity and exact result Type. Unchanged child
	// pointers reuse the receiver. Inputs must be valid and non-nil; nil asserts.
	// It checks local construction invariants, not logical equivalence.
	WithChildren(children []Expression) (Expression, error)
	// EqualsTo compares typed structural identity, not predicate equivalence.
	EqualsTo(other Expression) bool
	// String is diagnostic text, not serialization or an identity key.
	String() string
	// isExpression marks package-owned implementations. Embedding can still
	// satisfy the interface; implementations must honor its contracts.
	isExpression()
	// equalsTo shares successful pair comparisons across an immutable DAG.
	equalsTo(other Expression, seen map[[2]Expression]bool) bool
}
