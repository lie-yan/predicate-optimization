package basics_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Pretty printing displays the source and reconstructed candidate without
// changing them. Extraction and reconstruction perform the semantic work.
func Example_prettyPrintReconstruction() {
	context := fixtures.Must(functions.NewStandardContext())
	integer := fixtures.Must(types.NewIntType(types.Int64, foundation.NonNull))
	x := fixtures.ScalarReference("x", fixtures.Must(integer.WithNullability(foundation.MayNull)))
	one, three := fixtures.Must(testbedvalues.NewInteger(integer, 1)), fixtures.Must(testbedvalues.NewInteger(integer, 3))
	// x IN [3, 1, 3] AND x != 1 has the singleton true set {3}.
	// Its result remains NULL when x is NULL, so the candidate retains Bool?.
	source := fixtures.ScalarLogical(foundation.And,
		fixtures.ScalarMembership(x, three, one, three),
		fixtures.ScalarComparison(context, foundation.NotEqual, x, fixtures.ScalarLiteral(one)),
	)
	summary := fixtures.ScalarSummary(context, source)
	candidate, present := fixtures.Must(reconstruction.NewScalarReconstructor(testbedvalues.ValueFactory{}, context).Reconstruct(summary)).Get()
	if !present {
		panic("expected equality candidate")
	}
	fmt.Println("Before:")
	fmt.Println(expressions.PrettyPrint(source))
	fmt.Println("After reconstruction:")
	fmt.Println(expressions.PrettyPrint(candidate))
	// Output:
	// Before:
	// LogicalExpression AND : Bool?
	// ├ operand[0]: SpecialFormExpression IN : Bool?
	// │ ├ target: ReferenceExpression name="x" id="ColumnID(scope=\"example\", symbol=\"x\")" : Int64?
	// │ ├ candidate[0]: LiteralExpression value=3 : Int64!
	// │ ├ candidate[1]: LiteralExpression value=1 : Int64!
	// │ └ candidate[2]: LiteralExpression value=3 : Int64!
	// └ operand[1]: CallExpression function="testbed.standard"."int64.not_equal" : Bool?
	//   ├ arg[0]: ReferenceExpression name="x" id="ColumnID(scope=\"example\", symbol=\"x\")" : Int64?
	//   └ arg[1]: LiteralExpression value=1 : Int64!
	// After reconstruction:
	// CallExpression function="testbed.standard"."int64.equal" : Bool?
	// ├ arg[0]: ReferenceExpression name="x" id="ColumnID(scope=\"example\", symbol=\"x\")" : Int64?
	// └ arg[1]: LiteralExpression value=3 : Int64!
}
