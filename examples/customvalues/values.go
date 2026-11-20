// Package customvalues demonstrates caller-owned storage and construction.
// It depends only on public contracts, foundation and Types, never the testbed.
// This compact representation favors clarity rather than optimized storage.
package customvalues

import (
	"fmt"
	"math"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Value is immutable after construction; composite slices are owned copies.
// Multiple accessors are exposed, so consumers must select one by semantic Type.
type Value struct {
	typ      types.Type
	payload  any
	children []valueinterfaces.Value
	null     bool
}

func (v Value) Type() types.Type                          { return v.typ }
func (v Value) IsNull() bool                              { return v.null }
func (v Value) Bool() bool                                { return v.payload.(bool) }
func (v Value) Integer() int64                            { return v.payload.(int64) }
func (v Value) Float() float64                            { return v.payload.(float64) }
func (v Value) VarChar() string                           { return v.payload.(string) }
func (v Value) ElementCount() int                         { return len(v.children) }
func (v Value) ElementAt(index int) valueinterfaces.Value { return v.children[index] }
func (v Value) Elements() []valueinterfaces.Value {
	return append([]valueinterfaces.Value(nil), v.children...)
}

func (v Value) Fields() []valueinterfaces.Value {
	return append([]valueinterfaces.Value(nil), v.children...)
}

func (v Value) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	if v.null && n == foundation.NonNull {
		return nil, fmt.Errorf("cannot narrow null")
	}
	typ, err := v.typ.WithNullability(n)
	if err != nil {
		return nil, err
	}
	v.typ = typ
	return v, nil
}

// EqualsTo owns comparison and reads foreign payloads without reciprocal dispatch.
func (v Value) EqualsTo(other valueinterfaces.Value) bool {
	if !v.typ.EqualsTo(other.Type()) || v.null != other.IsNull() {
		return false
	}
	if v.null {
		return true
	}
	var children []valueinterfaces.Value
	switch v.typ.(type) {
	case *types.BoolType:
		return v.Bool() == other.(valueinterfaces.BoolValue).Bool()
	case *types.IntType:
		return v.Integer() == other.(valueinterfaces.IntegerValue).Integer()
	case *types.FloatType:
		return math.Float64bits(v.Float()) == math.Float64bits(other.(valueinterfaces.FloatValue).Float())
	case *types.VarCharType:
		return v.VarChar() == other.(valueinterfaces.VarCharValue).VarChar()
	case *types.ArrayType:
		children = other.(valueinterfaces.ArrayValue).Elements()
	case *types.StructType:
		children = other.(valueinterfaces.StructValue).Fields()
	default:
		panic("unsupported value domain")
	}
	if len(v.children) != len(children) {
		return false
	}
	for i, child := range v.children {
		if !child.EqualsTo(children[i]) {
			return false
		}
	}
	return true
}

// Factory supports every public domain. Types/children must already be valid;
// construction checks new payloads. Its zero value is stateless and usable.
type Factory struct{}

var _ valueinterfaces.ValueFactory = Factory{}
var (
	_ valueinterfaces.BoolValue    = Value{}
	_ valueinterfaces.IntegerValue = Value{}
	_ valueinterfaces.FloatValue   = Value{}
	_ valueinterfaces.VarCharValue = Value{}
	_ valueinterfaces.ArrayValue   = Value{}
	_ valueinterfaces.StructValue  = Value{}
)

func (Factory) NewNull(typ types.Type) (valueinterfaces.Value, error) {
	if typ.Nullability() != foundation.MayNull {
		return nil, fmt.Errorf("null requires nullable Type")
	}
	return Value{typ: typ, null: true}, nil
}

func (Factory) NewBool(typ *types.BoolType, v bool) (valueinterfaces.Value, error) {
	return Value{typ: typ, payload: v}, nil
}

func (Factory) NewInteger(typ *types.IntType, v int64) (valueinterfaces.Value, error) {
	if width := typ.IntegerWidth(); width != types.Int64 {
		limit := int64(1) << (width - 1)
		if v < -limit || v >= limit {
			return nil, fmt.Errorf("integer exceeds width")
		}
	}
	return Value{typ: typ, payload: v}, nil
}

func (Factory) NewFloat(typ *types.FloatType, v float64) (valueinterfaces.Value, error) {
	if math.IsNaN(v) {
		v = math.NaN()
	} else if typ.FloatWidth() == types.Float32 && float64(float32(v)) != v {
		return nil, fmt.Errorf("float is not exactly representable")
	}
	return Value{typ: typ, payload: v}, nil
}

func (Factory) NewVarChar(typ *types.VarCharType, v string) (valueinterfaces.Value, error) {
	if limit, bounded := typ.MaxBytes(); bounded && uint64(len(v)) > limit {
		return nil, fmt.Errorf("string exceeds byte bound")
	}
	return Value{typ: typ, payload: v}, nil
}

func (Factory) NewArray(typ *types.ArrayType, children []valueinterfaces.Value) (valueinterfaces.Value, error) {
	for _, v := range children {
		if !v.Type().EqualsTo(typ.Element()) {
			return nil, fmt.Errorf("array child Type mismatch")
		}
	}
	return Value{typ: typ, children: append([]valueinterfaces.Value(nil), children...)}, nil
}

func (Factory) NewStruct(typ *types.StructType, children []valueinterfaces.Value) (valueinterfaces.Value, error) {
	fields := typ.Fields()
	if len(fields) != len(children) {
		return nil, fmt.Errorf("struct field count mismatch")
	}
	for i, v := range children {
		if !v.Type().EqualsTo(fields[i].Type) {
			return nil, fmt.Errorf("struct field Type mismatch")
		}
	}
	return Value{typ: typ, children: append([]valueinterfaces.Value(nil), children...)}, nil
}
