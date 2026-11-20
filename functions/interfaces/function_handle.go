package interfaces

import "github.com/lie-yan/predicate-optimization/types"

// FunctionHandle identifies a resolved function in a matching caller-supplied
// context. Its concrete representation belongs to the caller. Handles must be
// valid and non-nil, including not containing typed nil values, and their meaning
// must remain stable throughout optimization. Implementations assert non-nil
// receivers and equality operands where applicable.
//
// A handle's concrete type alone does not establish context compatibility; the
// caller must pair handles with contexts that recognize their identity domain.
// Handles need not be comparable Go values and must not be used directly as map
// keys. No private marker method restricts implementation outside this package.
type FunctionHandle interface {
	// Namespace returns the caller's function namespace.
	Namespace() string
	// Name returns the resolved function name, not diagnostic display text.
	Name() string
	// ArgumentTypes returns a copy of the ordered implementation parameter
	// types. Immutable Type descriptors may be shared. These types agree with
	// metadata and may differ from a Call's effective argument types.
	ArgumentTypes() []types.Type
	// EqualsTo compares complete resolved identity, including signature and
	// any version or context identity needed to distinguish semantics. Names,
	// argument types and pointer identity alone need not establish equality.
	// Equality must be reflexive, symmetric, transitive and stable. Different
	// representations or identity domains compare unequal unless they support
	// a common identity contract. Equality does not grant context compatibility.
	EqualsTo(other FunctionHandle) bool
}
