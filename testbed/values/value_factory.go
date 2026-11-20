package values

import (
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ValueFactory constructs the bundled testbed representation. Its zero value
// is usable and stateless; it supports concurrent construction.
type ValueFactory struct{}

var _ valueinterfaces.ValueFactory = ValueFactory{}

// NewNull delegates to the checked bundled constructor.
func (ValueFactory) NewNull(typ types.Type) (valueinterfaces.Value, error) {
	value, err := NewNull(typ)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// NewBool delegates to the checked bundled constructor.
func (ValueFactory) NewBool(typ *types.BoolType, boolean bool) (valueinterfaces.Value, error) {
	value, err := NewBool(typ, boolean)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// NewInteger delegates to the checked bundled constructor.
func (ValueFactory) NewInteger(typ *types.IntType, integer int64) (valueinterfaces.Value, error) {
	value, err := NewInteger(typ, integer)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// NewFloat delegates to the checked bundled constructor.
func (ValueFactory) NewFloat(typ *types.FloatType, floating float64) (valueinterfaces.Value, error) {
	value, err := NewFloat(typ, floating)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// NewVarChar delegates to the checked bundled constructor.
func (ValueFactory) NewVarChar(typ *types.VarCharType, text string) (valueinterfaces.Value, error) {
	value, err := NewVarChar(typ, text)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// NewArray delegates to the checked bundled constructor.
func (ValueFactory) NewArray(typ *types.ArrayType, children []valueinterfaces.Value) (valueinterfaces.Value, error) {
	value, err := NewArray(typ, children)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// NewStruct delegates to the checked bundled constructor.
func (ValueFactory) NewStruct(typ *types.StructType, children []valueinterfaces.Value) (valueinterfaces.Value, error) {
	value, err := NewStruct(typ, children)
	if err != nil {
		return nil, err
	}
	return value, nil
}
