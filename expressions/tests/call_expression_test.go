package expressions_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	columns "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestCallExpressionBindingAndOwnership(t *testing.T) {
	provider := fixture(testbed.NewStandardContext())
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := fixture(integer.WithNullability(foundation.MayNull))
	ref := fixture(expressions.NewReferenceExpression(fixture(columns.NewColumnID("table", "x")), nullable, "x"))
	literal := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	handle := fixture(provider.ArithmeticFunction(foundation.Add, []types.Type{nullable, integer}))
	named := fixture(provider.LookupBuiltInFunction(functions.BuiltInInt64Add, []types.Type{nullable, integer}))
	args := []expressions.Expression{ref, literal}
	call := fixture(expressions.NewCallExpression(provider, handle, args))
	same := fixture(expressions.NewCallExpression(provider, named, []expressions.Expression{ref, literal}))
	if !call.EqualsTo(same) || !same.EqualsTo(call) || !call.EqualsTo(call) {
		t.Fatal("equivalent lookup paths differ")
	}
	if call.Handle() != handle || !call.Type().EqualsTo(nullable) || call.Type() != call.Signature().ResultType() {
		t.Fatal("handle or effective result lost")
	}
	declared := fixture(provider.Metadata(handle)).Signature()
	if declared.ResultType().Nullability() != foundation.NonNull {
		t.Fatal("implementation signature changed")
	}
	bound := call.Signature().ArgumentTypes()
	if !bound[0].EqualsTo(nullable) || !bound[1].EqualsTo(integer) {
		t.Fatal("effective argument types lost")
	}
	args[0] = literal
	bound[0] = integer
	returned := call.Arguments()
	returned[0] = literal
	children := call.Children()
	children[1] = ref
	if call.Arguments()[0] != ref || call.Children()[1] != literal || !call.Signature().ArgumentTypes()[0].EqualsTo(nullable) {
		t.Fatal("caller slice mutation changed call")
	}
	swapped := fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{literal, ref}))
	if call.EqualsTo(swapped) || call.EqualsTo(literal) || literal.EqualsTo(call) {
		t.Fatal("order or variant ignored")
	}
	nested := fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{call, literal}))
	if nested.Children()[0] != call {
		t.Fatal("nested child not shared")
	}
	if !strings.Contains(call.String(), "int64.add") || !strings.Contains(call.String(), "Int64?") {
		t.Fatal("diagnostic missing identity/type")
	}
}

func TestCallExpressionMetadataOnlyAndArrayLiteral(t *testing.T) {
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	sig := fixture(functions.NewFunctionSignature(nil, boolean))
	metadata := fixture(functions.NewFunctionMetadata(sig, "constant", foundation.Optional[functions.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, nil))
	catalog := fixture(testbed.NewCatalog([]testbed.Definition{{Namespace: "fixture", Name: "constant", Metadata: metadata}}))
	provider := testbed.NewContext(catalog)
	call := fixture(expressions.NewCallExpression(provider, catalog.Handles()[0], nil))
	if len(call.Children()) != 0 || call.Type() != boolean {
		t.Fatal("zero-argument metadata-only call invalid")
	}
	if _, err := provider.Evaluate(call.Handle(), call.Signature(), nil); !errors.Is(err, functions.ErrEvaluationUnavailable) {
		t.Fatal("metadata-only fixture unexpectedly executable")
	}
	standard := fixture(testbed.NewStandardContext())
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	array := fixture(types.NewArrayType(integer, foundation.NonNull))
	v := fixture(testbedvalues.NewArray(array, []valueinterfaces.Value{fixture(testbedvalues.NewInteger(integer, 8792001))}))
	list := fixture(expressions.NewLiteralExpression(v))
	ref := fixture(expressions.NewReferenceExpression(fixture(columns.NewColumnID("table", "category_ids")), array, "category_ids"))
	handle := fixture(standard.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{array, array}))
	contains := fixture(expressions.NewCallExpression(standard, handle, []expressions.Expression{ref, list}))
	if len(contains.Children()) != 2 || contains.Children()[1] != list || len(list.Children()) != 0 {
		t.Fatal("array literal flattened into arguments")
	}
}

func TestCallExpressionInvalidBindings(t *testing.T) {
	provider := fixture(testbed.NewStandardContext())
	other := fixture(testbed.NewStandardContext())
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	literal := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewBool(boolean, true))))
	handle := fixture(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{boolean}))
	args := []expressions.Expression{literal}
	if call, err := expressions.NewCallExpression(other, handle, args); call != nil || !errors.Is(err, functions.ErrIncompatibleHandle) {
		t.Fatal("foreign handle accepted")
	}
	otherHandle := fixture(other.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{boolean}))
	if fixture(expressions.NewCallExpression(provider, handle, args)).EqualsTo(fixture(expressions.NewCallExpression(other, otherHandle, args))) {
		t.Fatal("catalog identity ignored")
	}
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	number := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	for _, invalid := range [][]expressions.Expression{nil, {literal, literal}, {number}} {
		if call, err := expressions.NewCallExpression(provider, handle, invalid); call != nil || !errors.Is(err, functions.ErrUnsupportedLookup) {
			t.Fatal("invalid binding accepted")
		}
	}
	valid := fixture(expressions.NewCallExpression(provider, handle, args))
	var nilCall *expressions.CallExpression
	for _, call := range []func(){
		func() { _, _ = expressions.NewCallExpression(nil, handle, args) },
		func() { _, _ = expressions.NewCallExpression((*testbed.Context)(nil), handle, args) },
		func() { _, _ = expressions.NewCallExpression(provider, nil, args) },
		func() { _, _ = expressions.NewCallExpression(provider, (*testbed.Handle)(nil), args) },
		func() { _, _ = expressions.NewCallExpression(provider, handle, []expressions.Expression{nil}) },
		func() {
			_, _ = expressions.NewCallExpression(provider, handle, []expressions.Expression{(*expressions.LiteralExpression)(nil)})
		},
		func() { valid.EqualsTo(nil) }, func() { valid.EqualsTo(nilCall) }, func() { nilCall.EqualsTo(valid) },
		func() { nilCall.Handle() }, func() { nilCall.Signature() }, func() { nilCall.Type() }, func() { nilCall.Arguments() }, func() { nilCall.Children() }, func() { _ = nilCall.String() },
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("nil precondition did not assert")
				}
			}()
			call()
		}()
	}
}
