package interfaces

// ColumnID identifies a column in a caller-defined binding environment.
// Implementations are supplied by callers, with no private marker requirement.
// IDs must be valid, non-nil and immutable throughout an optimization session.
// Their identity must include any collection, scope or version needed to avoid
// conflating distinct inputs. The matching environment resolves their meaning.
// IDs need not be comparable Go values; use EqualsTo, not == or map[ColumnID].
type ColumnID interface {
	// Key returns a canonical, session-stable identity key. Two IDs have equal
	// keys if and only if EqualsTo reports equality, including across concrete
	// representations. Include the identity domain/scope and encode components
	// unambiguously. Expression Type and diagnostic spelling are not identity.
	// Implementations should construct the key once and share its immutable value.
	Key() string
	// EqualsTo compares identity, independently of diagnostic spelling and the
	// reference's declared Type. Equality must be reflexive, symmetric,
	// transitive and stable. Incompatible representations/domains compare false
	// unless they implement a common identity contract. Nil operands assert.
	EqualsTo(other ColumnID) bool
	// String supplies diagnostic text, not an equality or lookup key.
	String() string
}
