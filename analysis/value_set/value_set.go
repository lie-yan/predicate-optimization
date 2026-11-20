package value_set

import (
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ValueSet represents a region of one non-null scalar universe. Membership is
// mathematical set identity, not SQL equality (in particular for NaN).
// Implementations are immutable. Errors report unavailable exact answers rather
// than treating them as false. Binary operations require compatible element Types
// and a supported pair of representations; nil arguments assert.
type ValueSet interface {
	Type() types.Type
	// String returns the complete diagnostic representation without its Type.
	// Output is deterministic for stored normalized contents; equivalent sets
	// may have different strings. This is neither serialization nor equality.
	// Formatting requires a constructed receiver and leaves the set unchanged.
	String() string
	Contains(valueinterfaces.Value) (bool, error)
	Union(ValueSet) (ValueSet, error)
	Intersect(ValueSet) (ValueSet, error)
	Difference(ValueSet) (ValueSet, error)
	IsSubsetOf(ValueSet) (bool, error)
	IsDisjointFrom(ValueSet) (bool, error)
	// EqualsTo compares regions, independently of representation and entry order.
	EqualsTo(ValueSet) (bool, error)
	Complement() (ValueSet, error)
	IsEmpty() (bool, error)
	IsUniversal() (bool, error)
	isValueSet()
}

// ValueSetMarker lets concrete representations in value_set subpackages explicitly
// opt into ValueSet's private marker. Embed it alongside the implemented methods;
// it supplies no default behavior. It is an intent marker, not an access boundary.
type ValueSetMarker struct{}

func (ValueSetMarker) isValueSet() {}
