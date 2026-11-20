package interfaces

// OpaqueID identifies a preserved upstream region in a caller-defined domain.
// Implementations are caller-owned, valid, non-nil and immutable throughout the
// session. The adapter retains the original region and must not reuse an ID for
// different semantics or declared Types. No private marker limits implementation.
// IDs need not be comparable Go values: use EqualsTo, not == or map[OpaqueID].
type OpaqueID interface {
	// Key returns a canonical, session-stable identity key. Two IDs have equal
	// keys if and only if EqualsTo reports equality, including across concrete
	// representations. Include the identity domain/scope and encode components
	// unambiguously. Expression Type and diagnostic spelling are not identity.
	// Implementations should construct the key once and share its immutable value.
	Key() string
	// EqualsTo compares complete preservation identity, including its domain.
	// Equality must be reflexive, symmetric, transitive and stable. Incompatible
	// representations compare false unless they share an identity contract.
	// Nil receivers/operands assert where applicable.
	EqualsTo(other OpaqueID) bool
	// String supplies diagnostics, not an identity or lookup key.
	String() string
}
