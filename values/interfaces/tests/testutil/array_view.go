package testutil

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ArrayView is a non-comparable external array representation used by interoperability
// tests. It preserves its representation through nullability adaptation.
type ArrayView struct {
	source        valueinterfaces.ArrayValue
	nonComparable []byte
}

func WrapArray(source valueinterfaces.ArrayValue) ArrayView   { return ArrayView{source: source} }
func (v ArrayView) Type() types.Type                          { return v.source.Type() }
func (v ArrayView) IsNull() bool                              { return v.source.IsNull() }
func (v ArrayView) EqualsTo(other valueinterfaces.Value) bool { return v.source.EqualsTo(other) }
func (v ArrayView) ElementCount() int                         { return v.source.ElementCount() }
func (v ArrayView) ElementAt(index int) valueinterfaces.Value { return v.source.ElementAt(index) }
func (v ArrayView) Elements() []valueinterfaces.Value         { return v.source.Elements() }
func (v ArrayView) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	adapted, err := v.source.WithNullability(n)
	if err != nil {
		return nil, err
	}
	return WrapArray(adapted.(valueinterfaces.ArrayValue)), nil
}
