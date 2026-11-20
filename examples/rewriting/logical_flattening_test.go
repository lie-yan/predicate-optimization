package rewriting_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Flatten matching Boolean groups through the public tree service, retaining
// opposite-operator boundaries and checking results with independent evaluation.
func Example_logicalFlattening() {
	boolean := fixtures.Must(types.NewBoolType(foundation.MayNull))
	a, b, c := fixtures.ScalarReference("a", boolean), fixtures.ScalarReference("b", boolean), fixtures.ScalarReference("c", boolean)
	falsity := fixtures.ScalarLiteral(fixtures.Must(testbedvalues.NewBool(fixtures.Must(types.NewBoolType(foundation.NonNull)), false)))
	// (a AND b) AND (c OR False). Flattening keeps the OR and its False operand;
	// removing constants belongs to a separate simplification rule.
	source := fixtures.ScalarLogical(foundation.And,
		fixtures.ScalarLogical(foundation.And, a, b),
		fixtures.ScalarLogical(foundation.Or, c, falsity),
	)
	if err := utils.ValidateForOptimization(source); err != nil {
		panic(err)
	}
	// No FunctionContext is required by this rule. Enclosing calls, if present,
	// still use their own paired contexts when WithChildren reconstructs them.
	rule := logical_flattening_rewriter.NewLogicalFlatteningRewriter()
	tree := rewriter.NewExpressionTreeRewriter[struct{}](rule)
	// Schedule this traversal before scoped attempts or after their publication.
	// It does not edit any active attempt's guard paths or proof records.
	flattened := fixtures.Must(tree.Rewrite(source, struct{}{}))
	fmt.Println("Before:")
	fmt.Println(expressions.PrettyPrint(source))
	fmt.Println("After flattening:")
	fmt.Println(expressions.PrettyPrint(flattened))
	fmt.Println("exact Type preserved:", flattened.Type().EqualsTo(source.Type()))
	// This rule alone needs one pass; an unchanged second pass retains identity.
	again := fixtures.Must(tree.Rewrite(flattened, struct{}{}))
	fmt.Println("second pass retains identity:", again == flattened)

	// Interpretation is separate from rewriting. Known NULL is a typed value,
	// not an absent binding; each row supplies all three columns.
	yes, no := fixtures.Must(testbedvalues.NewBool(boolean, true)), fixtures.Must(testbedvalues.NewBool(boolean, false))
	null := fixtures.Must(testbedvalues.NewNull(boolean))
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, row := range []struct {
		label   string
		a, b, c valueinterfaces.Value
	}{
		{"true, true, true", yes, yes, yes},
		{"true, true, NULL", yes, yes, null},
		{"false, true, NULL", no, yes, null},
		{"true, false, true", yes, no, yes},
	} {
		env := fixtures.Environment{{ID: a.ID(), Value: row.a}, {ID: b.ID(), Value: row.b}, {ID: c.ID(), Value: row.c}}
		original := fixtures.Must(evaluator.Evaluate(source, env))
		result := fixtures.Must(evaluator.Evaluate(flattened, env))
		truth := "NULL"
		if !result.IsNull() {
			truth = fmt.Sprint(result.(*testbedvalues.BoolValue).Bool())
		}
		fmt.Printf("a, b, c = %s: %s; same Value and Type: %t\n", row.label, truth, original.EqualsTo(result) && result.Type().EqualsTo(source.Type()))
	}
	// Output:
	// Before:
	// LogicalExpression AND : Bool?
	// ├ operand[0]: LogicalExpression AND : Bool?
	// │ ├ operand[0]: ReferenceExpression name="a" id="ColumnID(scope=\"example\", symbol=\"a\")" : Bool?
	// │ └ operand[1]: ReferenceExpression name="b" id="ColumnID(scope=\"example\", symbol=\"b\")" : Bool?
	// └ operand[1]: LogicalExpression OR : Bool?
	//   ├ operand[0]: ReferenceExpression name="c" id="ColumnID(scope=\"example\", symbol=\"c\")" : Bool?
	//   └ operand[1]: LiteralExpression value=false : Bool!
	// After flattening:
	// LogicalExpression AND : Bool?
	// ├ operand[0]: ReferenceExpression name="a" id="ColumnID(scope=\"example\", symbol=\"a\")" : Bool?
	// ├ operand[1]: ReferenceExpression name="b" id="ColumnID(scope=\"example\", symbol=\"b\")" : Bool?
	// └ operand[2]: LogicalExpression OR : Bool?
	//   ├ operand[0]: ReferenceExpression name="c" id="ColumnID(scope=\"example\", symbol=\"c\")" : Bool?
	//   └ operand[1]: LiteralExpression value=false : Bool!
	// exact Type preserved: true
	// second pass retains identity: true
	// a, b, c = true, true, true: true; same Value and Type: true
	// a, b, c = true, true, NULL: NULL; same Value and Type: true
	// a, b, c = false, true, NULL: false; same Value and Type: true
	// a, b, c = true, false, true: false; same Value and Type: true
}
