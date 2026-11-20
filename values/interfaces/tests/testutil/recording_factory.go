// Package testutil supplies value-factory fixtures for value interoperability tests.
package testutil

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Request captures a constructor invocation. Composite Payload slices are copied.
type Request struct {
	Kind    string
	Type    types.Type
	Payload any
}

// RecordingFactory records requests and can inject a wrapped failure before
// construction. Integer, Float and Null hooks can return independent representations;
// other calls use Delegate. Hooks and Delegate must honor ValueFactory contracts.
// This test fixture is not safe for concurrent use.
type RecordingFactory struct {
	Delegate valueinterfaces.ValueFactory
	Requests []Request
	Failure  error
	Integer  func(*types.IntType, int64) (valueinterfaces.Value, error)
	Float    func(*types.FloatType, float64) (valueinterfaces.Value, error)
	Null     func(types.Type) (valueinterfaces.Value, error)
}

var _ valueinterfaces.ValueFactory = (*RecordingFactory)(nil)

func (f *RecordingFactory) record(kind string, typ types.Type, payload any) error {
	f.Requests = append(f.Requests, Request{Kind: kind, Type: typ, Payload: payload})
	if f.Failure != nil {
		return fmt.Errorf("recording %s: %w", kind, f.Failure)
	}
	return nil
}

// NewNull records the request before invoking the configured constructor.
func (f *RecordingFactory) NewNull(typ types.Type) (valueinterfaces.Value, error) {
	if err := f.record("Null", typ, nil); err != nil {
		return nil, err
	}
	if f.Null != nil {
		return f.Null(typ)
	}
	return f.Delegate.NewNull(typ)
}

// NewBool records the request before invoking the configured constructor.
func (f *RecordingFactory) NewBool(typ *types.BoolType, boolean bool) (valueinterfaces.Value, error) {
	if err := f.record("Bool", typ, boolean); err != nil {
		return nil, err
	}
	return f.Delegate.NewBool(typ, boolean)
}

// NewInteger records the request before invoking the configured constructor.
func (f *RecordingFactory) NewInteger(typ *types.IntType, integer int64) (valueinterfaces.Value, error) {
	if err := f.record("Integer", typ, integer); err != nil {
		return nil, err
	}
	if f.Integer != nil {
		return f.Integer(typ, integer)
	}
	return f.Delegate.NewInteger(typ, integer)
}

// NewFloat records the request before invoking the configured constructor.
func (f *RecordingFactory) NewFloat(typ *types.FloatType, floating float64) (valueinterfaces.Value, error) {
	if err := f.record("Float", typ, floating); err != nil {
		return nil, err
	}
	if f.Float != nil {
		return f.Float(typ, floating)
	}
	return f.Delegate.NewFloat(typ, floating)
}

// NewVarChar records the request before invoking the configured constructor.
func (f *RecordingFactory) NewVarChar(typ *types.VarCharType, text string) (valueinterfaces.Value, error) {
	if err := f.record("VarChar", typ, text); err != nil {
		return nil, err
	}
	return f.Delegate.NewVarChar(typ, text)
}

// NewArray records the request before invoking the configured constructor.
func (f *RecordingFactory) NewArray(typ *types.ArrayType, children []valueinterfaces.Value) (valueinterfaces.Value, error) {
	if err := f.record("Array", typ, append([]valueinterfaces.Value(nil), children...)); err != nil {
		return nil, err
	}
	return f.Delegate.NewArray(typ, children)
}

// NewStruct records the request before invoking the configured constructor.
func (f *RecordingFactory) NewStruct(typ *types.StructType, children []valueinterfaces.Value) (valueinterfaces.Value, error) {
	if err := f.record("Struct", typ, append([]valueinterfaces.Value(nil), children...)); err != nil {
		return nil, err
	}
	return f.Delegate.NewStruct(typ, children)
}
