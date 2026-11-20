package array_membership_normalization_rewriter_test

import (
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
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

// Construction needs lookup and metadata; unchanged unchanged traversal needs
// neither. Evaluate is forbidden even before the fixture is frozen.
type unchangedContext struct {
	functioninterfaces.FunctionContext
	frozen bool
}

func (c *unchangedContext) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if c.frozen {
		panic("unchanged traversal queried metadata")
	}
	return c.FunctionContext.Metadata(handle)
}

func (c *unchangedContext) LookupBuiltInFunction(name functions.BuiltInFunctionName, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	if c.frozen {
		panic("unchanged traversal performed function lookup")
	}
	return c.FunctionContext.LookupBuiltInFunction(name, args)
}

func (*unchangedContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("normalization rewriter must not evaluate")
}

func TestPreservesTreesWithoutSingletonCandidates(t *testing.T) {
	provider := &unchangedContext{FunctionContext: must(testbed.NewStandardContext())}
	boolean := must(types.NewBoolType(foundation.MayNull))
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	arrayType := must(types.NewArrayType(integer, foundation.NonNull))
	nullableArray := must(arrayType.WithNullability(foundation.MayNull))
	array := must(expressions.NewReferenceExpression(must(ids.NewColumnID("normalization", "category_ids")), nullableArray, "category_ids"))
	flag := must(expressions.NewReferenceExpression(must(ids.NewColumnID("normalization", "flag")), boolean, "flag"))
	item := must(testbedvalues.NewInteger(integer, 8792001))
	candidates := must(expressions.NewLiteralExpression(must(testbedvalues.NewArray(arrayType, []valueinterfaces.Value{item, item}))))
	handle := must(provider.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{array.Type(), candidates.Type()}))
	multiple := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{array, candidates}))
	truth := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(boolean, true))))
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(boolean))))
	group := func(op foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
		return must(expressions.NewLogicalExpression(op, children))
	}
	shared := group(foundation.And, multiple, flag)
	nested := group(foundation.And, shared, truth)
	repeated := group(foundation.Or, nested, nested, null)
	not := group(foundation.Not, repeated)
	notHandle := must(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{not.Type()}))
	outerCall := must(expressions.NewCallExpression(provider, notHandle, []expressions.Expression{not}))
	membership := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{flag, truth, null}))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("normalization", "unsupported")), boolean, "unsupported"))
	provider.frozen = true
	tree := rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter())
	for _, tc := range []struct {
		name string
		root expressions.Expression
	}{
		{"literal", truth},
		{"known NULL", null},
		{"array literal", candidates},
		{"reference", flag},
		{"array reference", array},
		{"multiple candidates unchanged", multiple},
		{"nested AND", nested},
		{"shared OR operands", repeated},
		{"NOT", not},
		{"enclosing call", outerCall},
		{"IN", membership},
		{"opaque", opaque},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := expressions.PrettyPrint(tc.root)
			children := tc.root.Children()
			saved := slices.Clone(children)
			result := must(tree.Rewrite(tc.root, struct{}{}))
			if result != tc.root || !result.Type().EqualsTo(tc.root.Type()) {
				t.Fatal("rewriter changed identity or full Type")
			}
			if expressions.PrettyPrint(tc.root) != before || !slices.Equal(children, saved) || !slices.Equal(tc.root.Children(), saved) {
				t.Fatal("source or child slice mutated")
			}
		})
	}
	if repeated.Children()[0] != nested || repeated.Children()[1] != nested || multiple.Arguments()[0] != array || candidates.Value().(*testbedvalues.ArrayValue).Elements()[0] != item {
		t.Fatal("source sharing or literal element identity changed")
	}
}
