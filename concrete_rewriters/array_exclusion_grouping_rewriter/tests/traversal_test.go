package array_exclusion_grouping_rewriter_test

import (
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// The base fallback observes all nonlogical variants. Logical callbacks record
// their occurrence and delegate to the actual grouping rule's post-order work.
type occurrenceGrouping struct {
	*array_exclusion_grouping_rewriter.ArrayExclusionGroupingRewriter
	visited []expressions.Expression
}

func (r *occurrenceGrouping) RewriteExpression(node expressions.Expression, _ struct{}, _ rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	r.visited = append(r.visited, node)
	return foundation.Optional[expressions.Expression]{}, nil
}

func (r *occurrenceGrouping) RewriteLogical(node *expressions.LogicalExpression, scope struct{}, tree rewriter.ExpressionTreeRewriter[struct{}]) (foundation.Optional[expressions.Expression], error) {
	r.visited = append(r.visited, node)
	return r.ArrayExclusionGroupingRewriter.RewriteLogical(node, scope, tree)
}

func occurrenceOrder(node expressions.Expression) []expressions.Expression {
	result := []expressions.Expression{node}
	for _, child := range node.Children() {
		result = append(result, occurrenceOrder(child)...)
	}
	return result
}

func TestEveryOccurrenceAndImmutableSharing(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.MayNull))
	fixture := newMembershipFixture(must(testbed.NewStandardContext()), integer, foundation.MayNull)
	// Equal IDs are independently constructed, not the same ReferenceExpression.
	alias := fixtureReference("array", fixture.input.Type())
	if alias == fixture.input || !alias.ID().EqualsTo(fixture.input.ID()) {
		t.Fatal("invalid alias fixture")
	}
	one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
	candidateSlice := []valueinterfaces.Value{two, one}
	candidateValue := must(testbedvalues.NewArray(fixture.array, candidateSlice))
	literal := fixtureLiteral(candidateValue)
	first := fixtureExclude(fixture.call(functions.BuiltInArrayContainsAny, fixture.input, literal))
	second := fixtureExclude(fixture.contains(alias, one))
	pair := fixtureLogical(foundation.And, first, second, first)
	flag := fixtureReference("flag", boolean)
	parent := fixtureLogical(foundation.And, flag, pair)
	var trace []string
	call := executionCall(&trace, "unsupported_wrapper", boolean, []expressions.Expression{parent}, func(args []valueinterfaces.Value) (valueinterfaces.Value, error) { return args[0], nil })
	truth := fixtureLiteral(must(testbedvalues.NewBool(must(types.NewBoolType(foundation.NonNull)), true)))
	special := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{pair, truth}))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("traversal", "boolean")), boolean, "opaque"))
	source := fixtureLogical(foundation.Or, call, special, opaque, pair)
	if err := utils.ValidateForOptimization(source); err != nil {
		t.Fatal(err)
	}
	before := expressions.PrettyPrint(source)
	nodes := occurrenceOrder(source)
	childSnapshots := make([][]expressions.Expression, len(nodes))
	for i, node := range nodes {
		childSnapshots[i] = node.Children()
	}
	observer := &occurrenceGrouping{ArrayExclusionGroupingRewriter: array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{})}
	observer.Fallback = observer
	tree := rewriter.NewExpressionTreeRewriter[struct{}](observer)
	result := must(tree.Rewrite(source, struct{}{}))
	if !slices.Equal(observer.visited, nodes) {
		t.Fatal("source occurrence skipped, revisited or traversed out of order")
	}
	expectedGroup := fixture.expectedGroup(fixture.input, two, one, one, two, one)
	expectedParent := fixtureLogical(foundation.And, flag, expectedGroup)
	expectedCall := must(expressions.NewCallExpression(call.Context(), call.Handle(), []expressions.Expression{expectedParent}))
	expectedSpecial := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{expectedGroup, truth}))
	expected := fixtureLogical(foundation.Or, expectedCall, expectedSpecial, opaque, expectedGroup)
	if !result.EqualsTo(expected) || !result.Type().EqualsTo(source.Type()) {
		t.Fatalf("descendant work was lost:\n%s", expressions.PrettyPrint(result))
	}
	if result.Children()[2] != opaque || result.Children()[0].Children()[0].Children()[0] != flag || result.Children()[1].Children()[1] != truth {
		t.Fatal("unrelated immutable nodes were replaced")
	}
	// Every generated group retains the first array node and original immutable Values.
	for _, grouped := range []expressions.Expression{result.Children()[0].Children()[0].Children()[1], result.Children()[1].Children()[0], result.Children()[3]} {
		args := grouped.Children()[0].Children()
		items := args[1].(*expressions.LiteralExpression).Value().(*testbedvalues.ArrayValue).Elements()
		if args[0] != fixture.input || !slices.Equal(items, []valueinterfaces.Value{two, one, one, two, one}) {
			t.Fatal("shared input/Value identity lost")
		}
	}
	for i, node := range nodes {
		if !slices.Equal(node.Children(), childSnapshots[i]) {
			t.Fatal("source child storage changed")
		}
	}
	if expressions.PrettyPrint(source) != before || !slices.Equal(candidateValue.Elements(), candidateSlice) {
		t.Fatal("source or literal payload changed")
	}
	// Caller-owned input/accessor slices can be changed without touching either tree.
	resultBefore := expressions.PrettyPrint(result)
	candidateSlice[0] = one
	for _, node := range append(nodes, occurrenceOrder(result)...) {
		children := node.Children()
		if len(children) > 0 {
			children[0] = nil
		}
		if literal, ok := node.(*expressions.LiteralExpression); ok {
			if array, ok := literal.Value().(*testbedvalues.ArrayValue); ok {
				items := array.Elements()
				if len(items) > 0 {
					items[0] = nil
				}
			}
		}
	}
	if expressions.PrettyPrint(source) != before || expressions.PrettyPrint(result) != resultBefore {
		t.Fatal("owned slices alias internal storage")
	}
	if err := utils.ValidateForOptimization(result); err != nil {
		t.Fatal(err)
	}
	observer.visited = nil
	if again := must(tree.Rewrite(result, struct{}{})); again != result {
		t.Fatal("second pass changed root identity")
	}
	if !slices.Equal(observer.visited, occurrenceOrder(result)) {
		t.Fatal("second pass traversal repeated or skipped an occurrence")
	}
	assertExecutionTrace(t, trace, nil)
}

func TestNestedBoundariesFlatteningAndNormalizationStability(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	fixture := newMembershipFixture(must(testbed.NewStandardContext()), integer, foundation.MayNull)
	one, two := must(testbedvalues.NewInteger(integer, 1)), must(testbedvalues.NewInteger(integer, 2))
	first, second := fixtureExclude(fixture.contains(fixture.input, one)), fixtureExclude(fixture.contains(fixture.input, two))
	flag := fixtureReference("flag", must(types.NewBoolType(foundation.MayNull)))
	nested := fixtureLogical(foundation.And, first, fixtureLogical(foundation.And, second, flag))
	tree := boundaryTree()
	if err := utils.ValidateForOptimization(nested); err != nil {
		t.Fatal(err)
	}
	if result := must(tree.Rewrite(nested, struct{}{})); result != nested {
		t.Fatal("grouping crossed nested AND boundary")
	}
	flatten := rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter())
	flat := must(flatten.Rewrite(nested, struct{}{}))
	grouped := must(tree.Rewrite(flat, struct{}{}))
	expected := fixtureLogical(foundation.And, fixture.expectedGroup(fixture.input, one, two), flag)
	if !grouped.EqualsTo(expected) {
		t.Fatal("explicit flattening did not expose group")
	}
	duplicates := fixtureLogical(foundation.And, first, first, first)
	duplicateGroup := must(tree.Rewrite(duplicates, struct{}{}))
	if !duplicateGroup.EqualsTo(fixture.expectedGroup(fixture.input, one, one, one)) {
		t.Fatal("repeated shared exclusions lost positions")
	}
	normalize := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	for _, node := range []expressions.Expression{grouped, duplicateGroup} {
		for range 3 {
			if normalized := must(normalize.Rewrite(node, struct{}{})); normalized != node {
				t.Fatal("normalization split or changed grouped candidates")
			}
			if again := must(tree.Rewrite(node, struct{}{})); again != node {
				t.Fatal("grouping/normalization cycle changed root")
			}
		}
		if err := utils.ValidateForOptimization(node); err != nil {
			t.Fatal(err)
		}
	}
}
