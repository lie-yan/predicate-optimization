package array_exclusion_grouping_rewriter_test

import (
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type functionEvent struct {
	operation string
	name      functions.BuiltInFunctionName     // Set for built-in lookup.
	handle    functioninterfaces.FunctionHandle // Set for metadata/evaluation.
}

// Faults change operation success, never the successful metadata or handle
// meanings. Tests enable them after valid source construction and can count
// selected events in the callback to fail a particular later query.
type recordingContext struct {
	functioninterfaces.FunctionContext
	events []functionEvent
	fault  func(functionEvent) error
}

func (c *recordingContext) record(event functionEvent) error {
	c.events = append(c.events, event)
	if c.fault != nil {
		return c.fault(event)
	}
	return nil
}

func (c *recordingContext) LookupBuiltInFunction(name functions.BuiltInFunctionName, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	if err := c.record(functionEvent{operation: "lookup", name: name}); err != nil {
		return nil, err
	}
	return c.FunctionContext.LookupBuiltInFunction(name, args)
}

func (c *recordingContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if err := c.record(functionEvent{operation: "metadata", handle: handle}); err != nil {
		return nil, err
	}
	return c.FunctionContext.Metadata(handle)
}

func (c *recordingContext) Evaluate(handle functioninterfaces.FunctionHandle, signature *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	if err := c.record(functionEvent{operation: "evaluate", handle: handle}); err != nil {
		return nil, err
	}
	return c.FunctionContext.Evaluate(handle, signature, args)
}

// A slice makes the handle value non-comparable. Pairing belongs to the wrapper
// context, while resolved identity within that domain delegates to the catalog.
type wrappedHandle struct {
	owner    *wrappedContext
	delegate []functioninterfaces.FunctionHandle // Exactly one immutable handle.
}

func (h wrappedHandle) Namespace() string           { return h.delegate[0].Namespace() }
func (h wrappedHandle) Name() string                { return h.delegate[0].Name() }
func (h wrappedHandle) ArgumentTypes() []types.Type { return h.delegate[0].ArgumentTypes() }
func (h wrappedHandle) EqualsTo(other functioninterfaces.FunctionHandle) bool {
	o, ok := other.(wrappedHandle)
	return ok && h.owner == o.owner && h.delegate[0].EqualsTo(o.delegate[0])
}

type wrappedContext struct {
	delegate functioninterfaces.FunctionContext
}

var (
	_ functioninterfaces.FunctionContext = (*wrappedContext)(nil)
	_ functioninterfaces.FunctionHandle  = wrappedHandle{}
)

func (c *wrappedContext) wrap(handle functioninterfaces.FunctionHandle, err error) (functioninterfaces.FunctionHandle, error) {
	if err != nil {
		return nil, err
	}
	return wrappedHandle{owner: c, delegate: []functioninterfaces.FunctionHandle{handle}}, nil
}

func (c *wrappedContext) unwrap(handle functioninterfaces.FunctionHandle) (functioninterfaces.FunctionHandle, error) {
	h, ok := handle.(wrappedHandle)
	if !ok || h.owner != c {
		return nil, functions.ErrIncompatibleHandle
	}
	return h.delegate[0], nil
}

func (c *wrappedContext) LookupBuiltInFunction(name functions.BuiltInFunctionName, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	return c.wrap(c.delegate.LookupBuiltInFunction(name, args))
}

func (c *wrappedContext) ArithmeticFunction(op foundation.OperatorType, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	return c.wrap(c.delegate.ArithmeticFunction(op, args))
}

func (c *wrappedContext) ComparisonFunction(op foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
	return c.wrap(c.delegate.ComparisonFunction(op, left, right))
}

func (c *wrappedContext) IsArithmeticFunction(handle functioninterfaces.FunctionHandle) (bool, error) {
	h, err := c.unwrap(handle)
	if err != nil {
		return false, err
	}
	return c.delegate.IsArithmeticFunction(h)
}

func (c *wrappedContext) IsComparisonFunction(handle functioninterfaces.FunctionHandle) (bool, error) {
	h, err := c.unwrap(handle)
	if err != nil {
		return false, err
	}
	return c.delegate.IsComparisonFunction(h)
}

func (c *wrappedContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	h, err := c.unwrap(handle)
	if err != nil {
		return nil, err
	}
	return c.delegate.Metadata(h)
}

func (c *wrappedContext) Evaluate(handle functioninterfaces.FunctionHandle, signature *functions.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	h, err := c.unwrap(handle)
	if err != nil {
		return nil, err
	}
	return c.delegate.Evaluate(h, signature, args)
}

// Store this wrapper by value: its slice prevents Go interface equality while
// its delegated provider/handle semantics remain fully usable for evaluation.
type nonComparableContext struct {
	functioninterfaces.FunctionContext
	marker []byte
}

var _ functioninterfaces.FunctionContext = nonComparableContext{}
