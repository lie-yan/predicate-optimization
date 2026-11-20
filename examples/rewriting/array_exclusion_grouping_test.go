package rewriting_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Group direct exclusions using one paired provider and stable array bindings.
// Unlike singleton normalization, this removes repeated reads/evaluations: the
// caller supplies successful, effect-free membership and reference semantics.
func Example_arrayExclusionGrouping() {
	context := fixtures.Must(testfunctions.NewStandardContext())
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	inputType := fixtures.Must(types.NewArrayType(integer, foundation.MayNull))
	candidateType := fixtures.Must(types.NewArrayType(integer, foundation.NonNull))
	array := fixtures.ScalarReference("tag_ids", inputType)
	first := fixtures.Must(testbedvalues.NewInteger(integer, 8791003))
	second := fixtures.Must(testbedvalues.NewInteger(integer, 8791004))
	call := func(name functions.BuiltInFunctionName, candidate expressions.Expression) expressions.Expression {
		handle := fixtures.Must(context.LookupBuiltInFunction(name, []types.Type{array.Type(), candidate.Type()}))
		return fixtures.Must(expressions.NewCallExpression(context, handle, []expressions.Expression{array, candidate}))
	}
	source := fixtures.ScalarLogical(foundation.And,
		fixtures.ScalarLogical(foundation.Not, call(functions.BuiltInArrayContains, fixtures.ScalarLiteral(first))),
		fixtures.ScalarLogical(foundation.Not, call(functions.BuiltInArrayContainsAny, fixtures.ScalarLiteral(fixtures.Must(testbedvalues.NewArray(candidateType, []valueinterfaces.Value{second, first}))))),
	)
	if err := utils.ValidateForOptimization(source); err != nil {
		panic(err)
	}

	// Complete this pass outside active scoped attempts. The stateless rule uses
	// the calls' own contexts; it needs neither an environment nor an evaluator.
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{}))
	grouped := fixtures.Must(tree.Rewrite(source, struct{}{}))
	fmt.Println("Before:")
	fmt.Println(expressions.PrettyPrint(source))
	fmt.Println("After grouping:")
	fmt.Println(expressions.PrettyPrint(grouped))
	fmt.Println("exact Type preserved:", grouped.Type().EqualsTo(source.Type()))
	fmt.Printf("expression nodes: %d -> %d\n", reconstruction.MeasureExpression(source).NodeCount, reconstruction.MeasureExpression(grouped).NodeCount)
	fmt.Println("second pass retains identity:", fixtures.Must(tree.Rewrite(grouped, struct{}{})) == grouped)

	// The example intentionally keeps the duplicate 8791003. Grouping concatenates
	// candidates in source order; it does not sort or deduplicate their Values.
	// A fresh stable environment evaluates each tree independently on each row.
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, row := range []struct {
		label   string
		numbers []int64
		null    bool
	}{
		{"hit", []int64{8791004}, false},
		{"miss", []int64{42}, false},
		{"empty", nil, false},
		{"NULL", nil, true},
	} {
		var items []valueinterfaces.Value
		for _, n := range row.numbers {
			items = append(items, fixtures.Must(testbedvalues.NewInteger(integer, n)))
		}
		var input valueinterfaces.Value = fixtures.Must(testbedvalues.NewArray(inputType, items))
		if row.null {
			input = fixtures.Must(testbedvalues.NewNull(inputType))
		}
		original := fixtures.Must(evaluator.Evaluate(source, fixtures.Environment{{ID: array.ID(), Value: input}}))
		result := fixtures.Must(evaluator.Evaluate(grouped, fixtures.Environment{{ID: array.ID(), Value: input}}))
		truth := "NULL"
		if !result.IsNull() {
			truth = fmt.Sprint(result.(*testbedvalues.BoolValue).Bool())
		}
		fmt.Printf("%s: %s; same Value and Type: %t\n", row.label, truth, original.EqualsTo(result) && result.Type().EqualsTo(source.Type()))
	}
	// Output:
	// Before:
	// LogicalExpression AND : Bool?
	// ├ operand[0]: LogicalExpression NOT : Bool?
	// │ └ operand[0]: CallExpression function="testbed.standard"."array_contains" : Bool?
	// │   ├ arg[0]: ReferenceExpression name="tag_ids" id="ColumnID(scope=\"example\", symbol=\"tag_ids\")" : Array<Int64!>?
	// │   └ arg[1]: LiteralExpression value=8791003 : Int64!
	// └ operand[1]: LogicalExpression NOT : Bool?
	//   └ operand[0]: CallExpression function="testbed.standard"."array_contains_any" : Bool?
	//     ├ arg[0]: ReferenceExpression name="tag_ids" id="ColumnID(scope=\"example\", symbol=\"tag_ids\")" : Array<Int64!>?
	//     └ arg[1]: LiteralExpression value=[8791004, 8791003] : Array<Int64!>!
	// After grouping:
	// LogicalExpression NOT : Bool?
	// └ operand[0]: CallExpression function="testbed.standard"."array_contains_any" : Bool?
	//   ├ arg[0]: ReferenceExpression name="tag_ids" id="ColumnID(scope=\"example\", symbol=\"tag_ids\")" : Array<Int64!>?
	//   └ arg[1]: LiteralExpression value=[8791003, 8791004, 8791003] : Array<Int64!>!
	// exact Type preserved: true
	// expression nodes: 9 -> 4
	// second pass retains identity: true
	// hit: false; same Value and Type: true
	// miss: true; same Value and Type: true
	// empty: true; same Value and Type: true
	// NULL: NULL; same Value and Type: true
}
