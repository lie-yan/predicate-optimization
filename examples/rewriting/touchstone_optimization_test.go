package rewriting_test

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"

	"github.com/lie-yan/predicate-optimization/analysis/reconstruction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/testbed/touchstone"
)

// Derive the documented final touchstone from its input using public rewriters.
// The primary fixture has non-null article_type and nullable remaining fields;
// its paired provider and stable references satisfy the rules' execution contracts.
func Example_touchstoneOptimization() {
	fixture := fixtures.Must(touchstone.NewPrimaryTouchstone())
	if err := utils.ValidateForOptimization(fixture.Input); err != nil {
		panic(err)
	}

	// Finish each traversal before starting the next, outside active scoped
	// attempts. This explicit schedule demonstrates the five-pass pipeline; scheduling,
	// revisits and work budgets for a general optimizer remain separate work.
	passes := []struct {
		name string
		rule rewriter.ExpressionRewriter[struct{}]
	}{
		{"Contextual rewriting", scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, fixture.Context)},
		{"Boolean cleanup", boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, fixture.Context)},
		{"Logical flattening", logical_flattening_rewriter.NewLogicalFlatteningRewriter()},
		{"Array normalization", array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter()},
		{"Array grouping", array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{})},
	}
	printSize := func(label string, node expressions.Expression) {
		size := reconstruction.MeasureExpression(node)
		fmt.Printf("%s: %d nodes / %d literals / %d IN entries\n", label, size.NodeCount, size.LiteralOccurrences, size.MembershipEntries)
	}
	result := fixture.Input
	printSize("Input", result)
	for _, pass := range passes {
		tree := rewriter.NewExpressionTreeRewriter[struct{}](pass.rule)
		result = fixtures.Must(tree.Rewrite(result, struct{}{}))
		printSize(pass.name, result)
	}

	// Print the generated tree. Expected is independently assembled and used
	// only for comparison; it supplies no replacement to any rewrite pass.
	fmt.Println("Final generated tree:")
	fmt.Println(expressions.PrettyPrint(result))
	fmt.Println("matches independent expected tree:", result.EqualsTo(fixture.Expected))
	fmt.Println("exact Type preserved:", result.Type().EqualsTo(fixture.Input.Type()))
	// Output:
	// Input: 111 nodes / 57 literals / 46 IN entries
	// Contextual rewriting: 65 nodes / 29 literals / 19 IN entries
	// Boolean cleanup: 64 nodes / 28 literals / 19 IN entries
	// Logical flattening: 63 nodes / 28 literals / 19 IN entries
	// Array normalization: 63 nodes / 28 literals / 19 IN entries
	// Array grouping: 39 nodes / 22 literals / 19 IN entries
	// Final generated tree:
	// LogicalExpression AND : Bool?
	// ├ operand[0]: SpecialFormExpression IN : Bool!
	// │ ├ target: ReferenceExpression name="article_type" id="ColumnID(scope=\"touchstone\", symbol=\"article_type\")" : VarChar!
	// │ ├ candidate[0]: LiteralExpression value="tsa_01" : VarChar!
	// │ ├ candidate[1]: LiteralExpression value="tsa_02" : VarChar!
	// │ └ candidate[2]: LiteralExpression value="tsa_03" : VarChar!
	// ├ operand[1]: SpecialFormExpression IN : Bool?
	// │ ├ target: ReferenceExpression name="is_review" id="ColumnID(scope=\"touchstone\", symbol=\"is_review\")" : Int64?
	// │ ├ candidate[0]: LiteralExpression value=8795001 : Int64!
	// │ ├ candidate[1]: LiteralExpression value=8795002 : Int64!
	// │ ├ candidate[2]: LiteralExpression value=8795003 : Int64!
	// │ ├ candidate[3]: LiteralExpression value=8795004 : Int64!
	// │ └ candidate[4]: LiteralExpression value=8795005 : Int64!
	// ├ operand[2]: LogicalExpression NOT : Bool?
	// │ └ operand[0]: CallExpression function="testbed.standard"."array_contains" : Bool?
	// │   ├ arg[0]: ReferenceExpression name="category_ids" id="ColumnID(scope=\"touchstone\", symbol=\"category_ids\")" : Array<Int64!>?
	// │   └ arg[1]: LiteralExpression value=8792001 : Int64!
	// ├ operand[3]: LogicalExpression NOT : Bool?
	// │ └ operand[0]: SpecialFormExpression IN : Bool?
	// │   ├ target: ReferenceExpression name="brand_id" id="ColumnID(scope=\"touchstone\", symbol=\"brand_id\")" : Int64?
	// │   ├ candidate[0]: LiteralExpression value=8794001 : Int64!
	// │   ├ candidate[1]: LiteralExpression value=8794002 : Int64!
	// │   ├ candidate[2]: LiteralExpression value=8794003 : Int64!
	// │   ├ candidate[3]: LiteralExpression value=8794004 : Int64!
	// │   ├ candidate[4]: LiteralExpression value=8794005 : Int64!
	// │   ├ candidate[5]: LiteralExpression value=8794006 : Int64!
	// │   ├ candidate[6]: LiteralExpression value=8794007 : Int64!
	// │   ├ candidate[7]: LiteralExpression value=8794008 : Int64!
	// │   ├ candidate[8]: LiteralExpression value=8794009 : Int64!
	// │   ├ candidate[9]: LiteralExpression value=8794010 : Int64!
	// │   └ candidate[10]: LiteralExpression value=8794011 : Int64!
	// ├ operand[4]: LogicalExpression NOT : Bool?
	// │ └ operand[0]: CallExpression function="testbed.standard"."array_contains" : Bool?
	// │   ├ arg[0]: ReferenceExpression name="mall_ids" id="ColumnID(scope=\"touchstone\", symbol=\"mall_ids\")" : Array<Int64!>?
	// │   └ arg[1]: LiteralExpression value=8793001 : Int64!
	// └ operand[5]: LogicalExpression NOT : Bool?
	//   └ operand[0]: CallExpression function="testbed.standard"."array_contains_any" : Bool?
	//     ├ arg[0]: ReferenceExpression name="tag_ids" id="ColumnID(scope=\"touchstone\", symbol=\"tag_ids\")" : Array<Int64!>?
	//     └ arg[1]: LiteralExpression value=[8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009] : Array<Int64!>!
	// matches independent expected tree: true
	// exact Type preserved: true
}
