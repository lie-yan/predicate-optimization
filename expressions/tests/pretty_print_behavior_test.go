package expressions_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	columns "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestPrettyPrintLiteralValues(t *testing.T) {
	integer := fixture(types.NewIntType(types.Int64, foundation.MayNull))
	boolean := fixture(types.NewBoolType(foundation.MayNull))
	varchar := fixture(types.NewVarCharType(nil, foundation.NonNull))
	innerType := fixture(types.NewArrayType(integer, foundation.NonNull))
	outerType := fixture(types.NewArrayType(innerType, foundation.MayNull))
	emptyArray := fixture(testbedvalues.NewArray(innerType, nil))
	inner := fixture(testbedvalues.NewArray(innerType, []valueinterfaces.Value{
		fixture(testbedvalues.NewInteger(integer, 1)), fixture(testbedvalues.NewNull(integer)),
	}))
	nested := fixture(testbedvalues.NewArray(outerType, []valueinterfaces.Value{inner, emptyArray}))
	structType := fixture(types.NewStructType([]types.StructField{
		{Name: "items", Type: outerType}, {Name: "name", Type: varchar},
	}, foundation.NonNull))
	structure := fixture(testbedvalues.NewStruct(structType, []valueinterfaces.Value{nested, fixture(testbedvalues.NewVarChar(varchar, "hello"))}))
	emptyStruct := fixture(testbedvalues.NewStruct(fixture(types.NewStructType(nil, foundation.NonNull)), nil))

	for _, tc := range []struct {
		name  string
		value valueinterfaces.Value
		want  string
	}{
		{"null", fixture(testbedvalues.NewNull(boolean)), `LiteralExpression value=NULL : Bool?`},
		{"nullable non-null", fixture(testbedvalues.NewBool(boolean, false)), `LiteralExpression value=false : Bool?`},
		{"negative integer", fixture(testbedvalues.NewInteger(integer, -123)), `LiteralExpression value=-123 : Int64?`},
		{"escaped varchar", fixture(testbedvalues.NewVarChar(varchar, "a\n\t\r\x1b\"\\中\xff")), `LiteralExpression value="a\n\t\r\x1b\"\\\u4e2d\xff" : VarChar!`},
		{"empty array", emptyArray, `LiteralExpression value=[] : Array<Int64?>!`},
		{"nested array", nested, `LiteralExpression value=[[1, NULL], []] : Array<Array<Int64?>!>?`},
		{"empty struct", emptyStruct, `LiteralExpression value={} : Struct<>!`},
		{"nested struct", structure, `LiteralExpression value={[[1, NULL], []], "hello"} : Struct<"items": Array<Array<Int64?>!>?, "name": VarChar!>!`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(expressions.NewLiteralExpression(tc.value))
			before := root.String()
			if got := expressions.PrettyPrint(root); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if len(root.Children()) != 0 || root.String() != before {
				t.Fatal("printing changed the literal's children or existing diagnostics")
			}
		})
	}
}

func TestPrettyPrintFloatValues(t *testing.T) {
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := fixture(types.NewFloatType(width, foundation.MayNull))
		t.Run(typ.String(), func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				payload float64
				want    string
			}{
				{"positive zero", 0, "0"},
				{"negative zero", math.Copysign(0, -1), "-0"},
				{"positive infinity", math.Inf(1), "+Inf"},
				{"negative infinity", math.Inf(-1), "-Inf"},
				{"nan", math.NaN(), "NaN"},
				{"finite", 1.25, "1.25"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					root := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewFloat(typ, tc.payload))))
					want := "LiteralExpression value=" + tc.want + " : " + typ.String()
					if got := expressions.PrettyPrint(root); got != want {
						t.Fatalf("got %q, want %q", got, want)
					}
				})
			}
		})
	}
	// The same carrier needs different shortest decimals at the two declared widths.
	for _, tc := range []struct {
		width types.FloatWidth
		want  string
	}{
		{types.Float32, "LiteralExpression value=0.1 : Float32!"},
		{types.Float64, "LiteralExpression value=0.10000000149011612 : Float64!"},
	} {
		typ := fixture(types.NewFloatType(tc.width, foundation.NonNull))
		root := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewFloat(typ, float64(float32(0.1))))))
		if got := expressions.PrettyPrint(root); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}

// Override only diagnostic text; underlying fixtures retain their identity contracts.
type prettyColumnID struct{ expressioninterfaces.ColumnID }

func (prettyColumnID) String() string { return "id\n\t\"\\中\xff" }

type prettyOpaqueID struct{ expressioninterfaces.OpaqueID }

func (prettyOpaqueID) String() string { return "id\n\t\"\\中\xff" }

type prettyType struct{ types.Type }

func (prettyType) String() string { return "Custom<\"field\">!\n\t\\中\xff" }

func TestPrettyPrintEscapedDiagnostics(t *testing.T) {
	typ := fixture(types.NewBoolType(foundation.NonNull))
	name := "name\n\t\"\\中\xff"
	reference := fixture(expressions.NewReferenceExpression(prettyColumnID{fixture(columns.NewColumnID("table", "x"))}, prettyType{typ}, name))
	opaque := fixture(expressions.NewOpaqueExpression(prettyOpaqueID{fixture(columns.NewOpaqueID("adapter", "x"))}, typ, name))
	for _, tc := range []struct {
		root expressions.Expression
		want string
	}{
		{reference, `ReferenceExpression name="name\n\t\"\\\u4e2d\xff" id="id\n\t\"\\\u4e2d\xff" : Custom<"field">!\n\t\\u4e2d\xff`},
		{opaque, `OpaqueExpression name="name\n\t\"\\\u4e2d\xff" id="id\n\t\"\\\u4e2d\xff" : Bool!`},
	} {
		if got := expressions.PrettyPrint(tc.root); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}

type prettyFunctionHandle struct {
	functioninterfaces.FunctionHandle
}

func (prettyFunctionHandle) Namespace() string { return "ns\n中" }
func (prettyFunctionHandle) Name() string      { return "fn\t\"\\\xff" }

// Construction can bind metadata; after sealing, every context query is forbidden.
// Other methods are promoted from a nil interface and would panic if called.
type prettyPrintContext struct {
	functioninterfaces.FunctionContext
	metadata *functions.FunctionMetadata
	sealed   bool
}

func (c *prettyPrintContext) Metadata(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if c.sealed {
		panic("pretty printer queried metadata")
	}
	return c.metadata, nil
}

func (*prettyPrintContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("pretty printer evaluated a function")
}

func TestPrettyPrintSharedCallWithoutContextQueries(t *testing.T) {
	yes := logicalLiteral(foundation.NonNull, true)
	provider := fixture(testfunctions.NewStandardContext())
	handle := fixture(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{yes.Type()}))
	context := &prettyPrintContext{metadata: fixture(provider.Metadata(handle))}
	call := fixture(expressions.NewCallExpression(context, prettyFunctionHandle{handle}, []expressions.Expression{yes}))
	root := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{call, call}))
	context.sealed = true
	want := `LogicalExpression AND : Bool!
├ operand[0]: CallExpression function="ns\n\u4e2d"."fn\t\"\\\xff" : Bool!
│ └ arg[0]: LiteralExpression value=true : Bool!
└ operand[1]: CallExpression function="ns\n\u4e2d"."fn\t\"\\\xff" : Bool!
  └ arg[0]: LiteralExpression value=true : Bool!`
	if got := expressions.PrettyPrint(root); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if children := root.Children(); children[0] != call || children[1] != call {
		t.Fatal("printing changed subtree sharing")
	}
}
