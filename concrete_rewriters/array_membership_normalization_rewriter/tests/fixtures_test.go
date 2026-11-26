package array_membership_normalization_rewriter_test

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
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

type normalizationFixture struct {
	context *recordingContext
	integer *types.IntType
	array   *types.ArrayType // Candidate literals always have non-null outer Type.
	input   *expressions.ReferenceExpression
}

func newNormalizationFixture(provider functioninterfaces.FunctionContext, n foundation.Nullability) *normalizationFixture {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	array := must(types.NewArrayType(integer, foundation.NonNull))
	return &normalizationFixture{
		context: &recordingContext{FunctionContext: provider}, integer: integer, array: array,
		input: must(expressions.NewReferenceExpression(must(ids.NewColumnID("normalization", "array")), must(array.WithNullability(n)), "array")),
	}
}

func (f *normalizationFixture) element(n int64) *expressions.LiteralExpression {
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(f.integer, n))))
}

func (f *normalizationFixture) arrayValue(typ *types.ArrayType, numbers ...int64) *testbedvalues.ArrayValue {
	items := make([]valueinterfaces.Value, len(numbers))
	for i, n := range numbers {
		items[i] = must(testbedvalues.NewInteger(typ.Element().(*types.IntType), n))
	}
	return must(testbedvalues.NewArray(typ, items))
}

func (f *normalizationFixture) candidates(numbers ...int64) *expressions.LiteralExpression {
	return must(expressions.NewLiteralExpression(f.arrayValue(f.array, numbers...)))
}

func (f *normalizationFixture) call(name functions.BuiltInFunctionName, input, candidate expressions.Expression) *expressions.CallExpression {
	handle := must(f.context.LookupBuiltInFunction(name, []types.Type{input.Type(), candidate.Type()}))
	return must(expressions.NewCallExpression(f.context, handle, []expressions.Expression{input, candidate}))
}

// Alternative catalogs can expose valid metadata outside initial rule admission,
// or omit contains entirely. Executability is deliberately supplied separately.
func membershipDefinition(name functions.BuiltInFunctionName, args []types.Type, result types.Type, policies []functions.NullPolicy) testbed.Definition {
	metadata := must(functions.NewFunctionMetadata(must(functions.NewFunctionSignature(args, result)), string(name), foundation.Some(name), foundation.Optional[foundation.OperatorType]{}, policies))
	return testbed.Definition{Namespace: "normalization.fixture", Name: string(name), Metadata: metadata}
}

type fixtureBinding struct {
	id    expressioninterfaces.ColumnID
	value valueinterfaces.Value
}
type fixtureEnvironment struct {
	bindings []fixtureBinding
	lookups  []expressioninterfaces.ColumnID
	failure  error
}

func (e *fixtureEnvironment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	e.lookups = append(e.lookups, id)
	if e.failure != nil {
		return foundation.Optional[valueinterfaces.Value]{}, e.failure
	}
	for _, binding := range e.bindings {
		if binding.id.EqualsTo(id) {
			return foundation.Some(binding.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

// A slice makes the handle value non-comparable. Pairing belongs to the wrapper
// context, while resolved identity within that domain delegates to the catalog.
type wrappedHandle struct {
	owner    *wrappedContext
	delegate []functioninterfaces.FunctionHandle // Exactly one immutable handle.
}

func (h wrappedHandle) Namespace() string                       { return h.delegate[0].Namespace() }
func (h wrappedHandle) Name() string                            { return h.delegate[0].Name() }
func (h wrappedHandle) Signature() *functions.FunctionSignature { return h.delegate[0].Signature() }
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
