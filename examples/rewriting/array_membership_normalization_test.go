package rewriting_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
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

// Normalize a singleton array membership call without evaluating its input.
// The standard testbed supplies paired, successful/effect-free built-ins with
// the same element equality and outer-array null propagation semantics.
func Example_arrayMembershipNormalization() {
	context := fixtures.Must(testfunctions.NewStandardContext())
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	candidateType := fixtures.Must(types.NewArrayType(integer, foundation.NonNull))
	inputType := fixtures.Must(types.NewArrayType(integer, foundation.MayNull))
	array := fixtures.ScalarReference("category_ids", inputType)
	candidate := fixtures.ScalarLiteral(fixtures.Must(testbedvalues.NewArray(candidateType, []valueinterfaces.Value{fixtures.Must(testbedvalues.NewInteger(integer, 8792001))})))
	handle := fixtures.Must(context.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{array.Type(), candidate.Type()}))
	source := fixtures.ScalarLogical(foundation.Not, fixtures.Must(expressions.NewCallExpression(context, handle, []expressions.Expression{array, candidate})))
	if err := utils.ValidateForOptimization(source); err != nil {
		panic(err)
	}

	// The rule has no context parameter: target lookup and construction use the
	// source call's paired context. Run this pass outside active scoped attempts.
	rule := array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter()
	tree := rewriter.NewExpressionTreeRewriter[struct{}](rule)
	normalized := fixtures.Must(tree.Rewrite(source, struct{}{}))
	fmt.Println("Before:")
	fmt.Println(expressions.PrettyPrint(source))
	fmt.Println("After normalization:")
	fmt.Println(expressions.PrettyPrint(normalized))
	fmt.Println("exact Type preserved:", normalized.Type().EqualsTo(source.Type()))
	// Acceptance is the canonical call form, even though replacing an array
	// literal with a scalar literal does not reduce expression-node counts.
	fmt.Println("structural counts unchanged:", reconstruction.MeasureExpression(source) == reconstruction.MeasureExpression(normalized))
	again := fixtures.Must(tree.Rewrite(normalized, struct{}{}))
	fmt.Println("second pass retains identity:", again == normalized)

	// Evaluate both trees independently. Known NULL is a bound typed Value;
	// it is distinct from an empty array or an absent environment binding.
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, row := range []struct {
		label   string
		numbers []int64
		null    bool
	}{
		{"hit", []int64{8792001}, false},
		{"miss", []int64{42}, false},
		{"empty", nil, false},
		{"NULL", nil, true},
	} {
		elements := make([]valueinterfaces.Value, len(row.numbers))
		for i, n := range row.numbers {
			elements[i] = fixtures.Must(testbedvalues.NewInteger(integer, n))
		}
		var input valueinterfaces.Value = fixtures.Must(testbedvalues.NewArray(inputType, elements))
		if row.null {
			input = fixtures.Must(testbedvalues.NewNull(inputType))
		}
		original := fixtures.Must(evaluator.Evaluate(source, fixtures.Environment{{ID: array.ID(), Value: input}}))
		result := fixtures.Must(evaluator.Evaluate(normalized, fixtures.Environment{{ID: array.ID(), Value: input}}))
		truth := "NULL"
		if !result.IsNull() {
			truth = fmt.Sprint(result.(*testbedvalues.BoolValue).Bool())
		}
		fmt.Printf("%s: %s; same Value and Type: %t\n", row.label, truth, original.EqualsTo(result) && result.Type().EqualsTo(source.Type()))
	}
	// Output:
	// Before:
	// LogicalExpression NOT : Bool?
	// └ operand[0]: CallExpression function="testbed.standard"."array_contains_any" : Bool?
	//   ├ arg[0]: ReferenceExpression name="category_ids" id="ColumnID(scope=\"example\", symbol=\"category_ids\")" : Array<Int64!>?
	//   └ arg[1]: LiteralExpression value=[8792001] : Array<Int64!>!
	// After normalization:
	// LogicalExpression NOT : Bool?
	// └ operand[0]: CallExpression function="testbed.standard"."array_contains" : Bool?
	//   ├ arg[0]: ReferenceExpression name="category_ids" id="ColumnID(scope=\"example\", symbol=\"category_ids\")" : Array<Int64!>?
	//   └ arg[1]: LiteralExpression value=8792001 : Int64!
	// exact Type preserved: true
	// structural counts unchanged: true
	// second pass retains identity: true
	// hit: false; same Value and Type: true
	// miss: true; same Value and Type: true
	// empty: true; same Value and Type: true
	// NULL: NULL; same Value and Type: true
}
