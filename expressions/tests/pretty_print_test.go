package expressions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	columns "github.com/lie-yan/predicate-optimization/testbed/expressions"
	functions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestPrettyPrintGolden(t *testing.T) {
	yes := logicalLiteral(foundation.NonNull, true)
	no := logicalLiteral(foundation.NonNull, false)
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	nullableInteger := fixture(integer.WithNullability(foundation.MayNull))
	boolean := fixture(types.NewBoolType(foundation.MayNull))
	x := fixture(expressions.NewReferenceExpression(fixture(columns.NewColumnID("table", "x")), nullableInteger, "x"))
	one := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	ten := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 10))))
	arrayType := fixture(types.NewArrayType(nullableInteger, foundation.NonNull))
	array := fixture(expressions.NewReferenceExpression(fixture(columns.NewColumnID("table", "items")), arrayType, "items"))
	opaque := fixture(expressions.NewOpaqueExpression(fixture(columns.NewOpaqueID("adapter", "region")), boolean, "unsupported"))
	provider := fixture(functions.NewStandardContext())
	handle := fixture(provider.ComparisonFunction(foundation.LessThan, x.Type(), ten.Type()))
	call := fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{x, ten}))
	membership := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{x, ten, one}))
	emptyMembership := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{x}))
	not := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{yes}))
	and := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, no, not}))
	or := fixture(expressions.NewLogicalExpression(foundation.Or, []expressions.Expression{no, not, yes}))
	// Exercise continuations from both an intermediate sibling and a final sibling.
	deep := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{
		fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{or})),
		no,
		fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{and})),
	}))

	for _, tc := range []struct {
		name string
		root expressions.Expression
		want string
	}{
		{"literal root", ten, `LiteralExpression value=10 : Int64!`},
		{"reference root", x, `ReferenceExpression name="x" id="ColumnID(scope=\"table\", symbol=\"x\")" : Int64?`},
		{"nested declared type", array, `ReferenceExpression name="items" id="ColumnID(scope=\"table\", symbol=\"items\")" : Array<Int64?>!`},
		{"opaque root", opaque, `OpaqueExpression name="unsupported" id="OpaqueID(owner=\"adapter\", token=\"region\")" : Bool?`},
		{"call", call, `CallExpression function="testbed.standard"."int64.less_than" : Bool?
├ arg[0]: ReferenceExpression name="x" id="ColumnID(scope=\"table\", symbol=\"x\")" : Int64?
└ arg[1]: LiteralExpression value=10 : Int64!`},
		{"membership", membership, `SpecialFormExpression IN : Bool?
├ target: ReferenceExpression name="x" id="ColumnID(scope=\"table\", symbol=\"x\")" : Int64?
├ candidate[0]: LiteralExpression value=10 : Int64!
└ candidate[1]: LiteralExpression value=1 : Int64!`},
		{"empty membership", emptyMembership, `SpecialFormExpression IN : Bool?
└ target: ReferenceExpression name="x" id="ColumnID(scope=\"table\", symbol=\"x\")" : Int64?`},
		{"not", not, `LogicalExpression NOT : Bool!
└ operand[0]: LiteralExpression value=true : Bool!`},
		{"n-ary and", and, `LogicalExpression AND : Bool!
├ operand[0]: LiteralExpression value=true : Bool!
├ operand[1]: LiteralExpression value=false : Bool!
└ operand[2]: LogicalExpression NOT : Bool!
  └ operand[0]: LiteralExpression value=true : Bool!`},
		{"n-ary or", or, `LogicalExpression OR : Bool!
├ operand[0]: LiteralExpression value=false : Bool!
├ operand[1]: LogicalExpression NOT : Bool!
│ └ operand[0]: LiteralExpression value=true : Bool!
└ operand[2]: LiteralExpression value=true : Bool!`},
		{"deep branches", deep, `LogicalExpression AND : Bool!
├ operand[0]: LogicalExpression NOT : Bool!
│ └ operand[0]: LogicalExpression OR : Bool!
│   ├ operand[0]: LiteralExpression value=false : Bool!
│   ├ operand[1]: LogicalExpression NOT : Bool!
│   │ └ operand[0]: LiteralExpression value=true : Bool!
│   └ operand[2]: LiteralExpression value=true : Bool!
├ operand[1]: LiteralExpression value=false : Bool!
└ operand[2]: LogicalExpression NOT : Bool!
  └ operand[0]: LogicalExpression AND : Bool!
    ├ operand[0]: LiteralExpression value=true : Bool!
    ├ operand[1]: LiteralExpression value=false : Bool!
    └ operand[2]: LogicalExpression NOT : Bool!
      └ operand[0]: LiteralExpression value=true : Bool!`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Exact comparison also checks whitespace, LF separators and no trailing newline.
			if got := expressions.PrettyPrint(tc.root); got != tc.want {
				t.Fatalf("unexpected tree:\n%s\nwant:\n%s\ngot bytes: %q\nwant bytes: %q", got, tc.want, got, tc.want)
			}
		})
	}
}
