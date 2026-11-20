package logical_flattening_rewriter_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// Calls need metadata during construction, but unchanged traversal must
// neither reconstruct them through the provider nor execute them.
type unchangedContext struct {
	functioninterfaces.FunctionContext
	frozen bool
}

func (p *unchangedContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if p.frozen {
		panic("unchanged traversal must not query call metadata")
	}
	return p.FunctionContext.Metadata(handle)
}

func (*unchangedContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("logical flattening must not evaluate")
}

func TestPreservesTreesWithoutNestedMatchingGroups(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	nullable := must(types.NewBoolType(foundation.MayNull))
	a := must(expressions.NewReferenceExpression(must(ids.NewColumnID("flattening", "a")), boolean, "a"))
	b := must(expressions.NewReferenceExpression(must(ids.NewColumnID("flattening", "b")), nullable, "b"))
	truth := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(boolean, true))))
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullable))))
	group := func(op foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
		return must(expressions.NewLogicalExpression(op, children))
	}
	shared := group(foundation.And, a, b)
	sharedOccurrences := group(foundation.Or, shared, shared, truth)
	mixed := group(foundation.And, group(foundation.Or, a, b), null)
	not := group(foundation.Not, sharedOccurrences)
	provider := &unchangedContext{FunctionContext: must(testbed.NewStandardContext())}
	handle := must(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{not.Type()}))
	call := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{not}))
	membership := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, nullable, []expressions.Expression{b, truth}))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("flattening", "unsupported")), nullable, "unsupported"))
	provider.frozen = true
	rule := logical_flattening_rewriter.NewLogicalFlatteningRewriter()
	if rule == nil {
		t.Fatal("constructor returned nil")
	}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](rule)
	for _, tc := range []struct {
		name string
		root expressions.Expression
	}{
		{"literal", truth},
		{"known NULL", null},
		{"reference", a},
		{"nullable reference", b},
		{"flat AND", shared},
		{"shared boundary occurrences", sharedOccurrences},
		{"mixed operators", mixed},
		{"NOT", not},
		{"call", call},
		{"IN", membership},
		{"opaque", opaque},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := expressions.PrettyPrint(tc.root)
			children := tc.root.Children()
			result := must(tree.Rewrite(tc.root, struct{}{}))
			if result != tc.root || !result.Type().EqualsTo(tc.root.Type()) {
				t.Fatal("unchanged tree lost identity or Type")
			}
			if expressions.PrettyPrint(tc.root) != before {
				t.Fatal("source was mutated")
			}
			after := tc.root.Children()
			if len(after) != len(children) {
				t.Fatal("child count changed")
			}
			for i, child := range children {
				if after[i] != child {
					t.Fatal("child identity changed")
				}
			}
		})
	}
	// Shared AND boundaries of an OR remain independent unchanged occurrences.
	children := sharedOccurrences.Children()
	if len(children) != 3 || children[0] != shared || children[1] != shared {
		t.Fatal("changed grouping or lost shared occurrences")
	}
}
