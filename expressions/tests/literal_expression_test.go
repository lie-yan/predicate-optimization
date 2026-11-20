package expressions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func fixture[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestLiteralExpression(t *testing.T) {
	integer := fixture(types.NewIntType(types.Int64, foundation.MayNull))
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	text := fixture(types.NewVarCharType(nil, foundation.NonNull))
	array := fixture(types.NewArrayType(integer, foundation.NonNull))
	children := []valueinterfaces.Value{fixture(testbedvalues.NewInteger(integer, 42)), fixture(testbedvalues.NewNull(integer))}
	items := fixture(testbedvalues.NewArray(array, children))
	record := fixture(types.NewStructType([]types.StructField{{Name: "items", Type: array}}, foundation.NonNull))
	structValue := fixture(testbedvalues.NewStruct(record, []valueinterfaces.Value{items}))
	for _, tc := range []struct {
		name    string
		value   valueinterfaces.Value
		display string
	}{
		{"integer", children[0], "LiteralExpression(Int64?, 42)"},
		{"null", children[1], "LiteralExpression(Int64?, NULL)"},
		{"bool", fixture(testbedvalues.NewBool(boolean, true)), "LiteralExpression(Bool!, true)"},
		{"string", fixture(testbedvalues.NewVarChar(text, "a\n\"b")), `LiteralExpression(VarChar!, "a\n\"b")`},
		{"array", items, "LiteralExpression(Array<Int64?>!, [42, NULL])"},
		{"struct", structValue, `LiteralExpression(Struct<"items": Array<Int64?>!>!, {[42, NULL]})`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			literal := fixture(expressions.NewLiteralExpression(tc.value))
			var node expressions.Expression = literal
			if literal.Value() != tc.value || node.Type() != tc.value.Type() {
				t.Fatal("immutable Value or Type was copied")
			}
			if len(node.Children()) != 0 {
				t.Fatal("literal payloads became expression children")
			}
			if node.String() != tc.display {
				t.Fatalf("got %q, want %q", node.String(), tc.display)
			}
			if !node.EqualsTo(node) {
				t.Fatal("literal is not reflexive")
			}
		})
	}
	literal := fixture(expressions.NewLiteralExpression(items))
	// Mutating slices obtained from Values cannot change the literal payload.
	children[0] = children[1]
	exported := items.Elements()
	exported[0] = children[1]
	if literal.String() != "LiteralExpression(Array<Int64?>!, [42, NULL])" {
		t.Fatal("literal payload changed through a caller slice")
	}
}

func TestLiteralExpressionEquality(t *testing.T) {
	makeNested := func(n int64, nullable foundation.Nullability) *expressions.LiteralExpression {
		integer := fixture(types.NewIntType(types.Int64, nullable))
		array := fixture(types.NewArrayType(integer, foundation.NonNull))
		record := fixture(types.NewStructType([]types.StructField{{Name: "values", Type: array}}, foundation.NonNull))
		value := fixture(testbedvalues.NewStruct(record, []valueinterfaces.Value{fixture(testbedvalues.NewArray(array, []valueinterfaces.Value{fixture(testbedvalues.NewInteger(integer, n))}))}))
		return fixture(expressions.NewLiteralExpression(value))
	}
	a, b := makeNested(1, foundation.MayNull), makeNested(1, foundation.MayNull)
	if a == b || a.Value() == b.Value() || !a.EqualsTo(b) || !b.EqualsTo(a) {
		t.Fatal("equality depends on allocation identity")
	}
	if a.EqualsTo(makeNested(2, foundation.MayNull)) || a.EqualsTo(makeNested(1, foundation.NonNull)) {
		t.Fatal("payload or nested type ignored")
	}
	null := func() *expressions.LiteralExpression {
		return fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewNull(fixture(types.NewBoolType(foundation.MayNull))))))
	}
	if !null().EqualsTo(null()) {
		t.Fatal("typed null literals must be structurally equal")
	}
}

func TestLiteralExpressionInvalidAndNil(t *testing.T) {
	valid := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewBool(fixture(types.NewBoolType(foundation.NonNull)), false))))
	var nilLiteral *expressions.LiteralExpression
	for _, call := range []func(){
		func() { _, _ = expressions.NewLiteralExpression(nil) },
		func() { _, _ = expressions.NewLiteralExpression((*testbedvalues.BoolValue)(nil)) },
		func() { valid.EqualsTo(nil) },
		func() { valid.EqualsTo(nilLiteral) },
		func() { nilLiteral.EqualsTo(valid) },
		func() { nilLiteral.Value() },
		func() { nilLiteral.Type() },
		func() { nilLiteral.Children() },
		func() { _ = nilLiteral.String() },
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
