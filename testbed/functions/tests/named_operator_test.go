package functions_test

import (
	"errors"
	"testing"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestNamedOperatorLookup(t *testing.T) {
	f := newArrayFixture(t)
	text, err := types.NewVarCharType(nil, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		op      foundation.OperatorType
		name    shared.BuiltInFunctionName
		operand types.Type
	}{
		{foundation.Add, shared.BuiltInInt64Add, f.integer},
		{foundation.Subtract, shared.BuiltInInt64Subtract, f.integer},
		{foundation.Multiply, shared.BuiltInInt64Multiply, f.integer},
		{foundation.Divide, shared.BuiltInInt64Divide, f.integer},
		{foundation.Modulus, shared.BuiltInInt64Modulus, f.integer},
		{foundation.Negate, shared.BuiltInInt64Negate, f.integer},
		{foundation.Equal, shared.BuiltInInt64Equal, f.integer},
		{foundation.NotEqual, shared.BuiltInInt64NotEqual, f.integer},
		{foundation.LessThan, shared.BuiltInInt64LessThan, f.integer},
		{foundation.LessThanOrEqual, shared.BuiltInInt64LessThanOrEqual, f.integer},
		{foundation.GreaterThan, shared.BuiltInInt64GreaterThan, f.integer},
		{foundation.GreaterThanOrEqual, shared.BuiltInInt64GreaterThanOrEqual, f.integer},
		{foundation.Equal, shared.BuiltInVarCharEqual, text},
		{foundation.NotEqual, shared.BuiltInVarCharNotEqual, text},
		{foundation.LessThan, shared.BuiltInVarCharLessThan, text},
		{foundation.LessThanOrEqual, shared.BuiltInVarCharLessThanOrEqual, text},
		{foundation.GreaterThan, shared.BuiltInVarCharGreaterThan, text},
		{foundation.GreaterThanOrEqual, shared.BuiltInVarCharGreaterThanOrEqual, text},
	} {
		t.Run(string(tc.name), func(t *testing.T) {
			for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				operand, err := tc.operand.WithNullability(n)
				if err != nil {
					t.Fatal(err)
				}
				args := []types.Type{operand, operand}
				var operatorHandle functioninterfaces.FunctionHandle
				if tc.op.IsArithmeticOperator() {
					if tc.op == foundation.Negate {
						args = args[:1]
					}
					operatorHandle, err = f.context.ArithmeticFunction(tc.op, args)
				} else {
					operatorHandle, err = f.context.ComparisonFunction(tc.op, operand, operand)
				}
				if err != nil {
					t.Fatal(err)
				}
				named, err := f.context.LookupBuiltInFunction(tc.name, args)
				if err != nil {
					t.Fatal(err)
				}
				if !named.EqualsTo(operatorHandle) || !operatorHandle.EqualsTo(named) {
					t.Fatal("lookup paths resolve different entries")
				}
				metadata, err := f.context.Metadata(named)
				if err != nil {
					t.Fatal(err)
				}
				if op, ok := metadata.Operator().Get(); !ok || op != tc.op {
					t.Fatal("operator identity missing")
				}
				if name, ok := metadata.BuiltInName().Get(); !ok || name != tc.name {
					t.Fatal("built-in identity missing")
				}
				arithmetic, err := f.context.IsArithmeticFunction(named)
				if err != nil || arithmetic != tc.op.IsArithmeticOperator() {
					t.Fatal("arithmetic classification differs")
				}
				comparison, err := f.context.IsComparisonFunction(named)
				if err != nil || comparison != tc.op.IsComparisonOperator() {
					t.Fatal("comparison classification differs")
				}
			}
		})
	}
	add, err := f.context.LookupBuiltInFunction(shared.BuiltInInt64Add, []types.Type{f.integer, f.integer})
	if err != nil {
		t.Fatal(err)
	}
	result := evaluateStandard(t, f.context, add, evalInteger(t, f.integer, 6), evalInteger(t, f.integer, 2))
	if result.IsNull() || result.(*testbedvalues.IntegerValue).Integer() != 8 {
		t.Fatal("named addition returned an incorrect result")
	}
	for _, name := range []shared.BuiltInFunctionName{"int8.add", "int64.unknown", shared.BuiltInVarCharEqual} {
		if _, err := f.context.LookupBuiltInFunction(name, []types.Type{f.integer, f.integer}); !errors.Is(err, shared.ErrUnsupportedLookup) {
			t.Fatal("unsupported name or signature accepted")
		}
	}
}
