package array_exclusion_grouping_rewriter_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_exclusion_grouping_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
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

// Construction and entry validation can query the provider. Clearing this
// delegate afterward makes every FunctionContext method fail if traversal uses
// it, including Evaluate and classification queries. Stored handles stay valid.
type unchangedContext struct {
	functioninterfaces.FunctionContext
}

func TestPreservesTreesWithoutDirectExclusions(t *testing.T) {
	for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		t.Run(fmt.Sprint(nullability), func(t *testing.T) {
			provider := &unchangedContext{FunctionContext: must(testbed.NewStandardContext())}
			boolean := must(types.NewBoolType(nullability))
			integer := must(types.NewIntType(types.Int64, foundation.NonNull))
			candidateType := must(types.NewArrayType(integer, foundation.NonNull))
			arrayType := must(types.NewArrayType(integer, nullability))
			reference := func(name string, typ types.Type) *expressions.ReferenceExpression {
				return must(expressions.NewReferenceExpression(must(ids.NewColumnID("grouping", name)), typ, name))
			}
			logical := func(op foundation.LogicalOperator, args ...expressions.Expression) *expressions.LogicalExpression {
				return must(expressions.NewLogicalExpression(op, args))
			}
			call := func(name functions.BuiltInFunctionName, args ...expressions.Expression) *expressions.CallExpression {
				argumentTypes := make([]types.Type, len(args))
				for i, arg := range args {
					argumentTypes[i] = arg.Type()
				}
				handle := must(provider.LookupBuiltInFunction(name, argumentTypes))
				return must(expressions.NewCallExpression(provider, handle, args))
			}
			array, flag := reference("tags", arrayType), reference("flag", boolean)
			one := must(testbedvalues.NewInteger(integer, 1))
			two := must(testbedvalues.NewInteger(integer, 2))
			oneLiteral := must(expressions.NewLiteralExpression(one))
			candidates := must(expressions.NewLiteralExpression(must(testbedvalues.NewArray(candidateType, []valueinterfaces.Value{one, two}))))
			contains := call(functions.BuiltInArrayContains, array, oneLiteral)
			containsAny := call(functions.BuiltInArrayContainsAny, array, candidates)
			exclusion := logical(foundation.Not, contains)
			// Positive membership is outside negative AND grouping.
			positive := logical(foundation.And, contains, containsAny)
			truth := must(expressions.NewLiteralExpression(must(testbedvalues.NewBool(boolean, true))))
			nullableBoolean := must(types.NewBoolType(foundation.MayNull))
			null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(nullableBoolean))))
			nullArray := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(must(arrayType.WithNullability(foundation.MayNull))))))
			nested := logical(foundation.And, positive, flag, truth)
			repeated := logical(foundation.Or, nested, nested, null)
			outer := call(functions.BuiltInNot, repeated)
			membership := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{flag, truth}))
			opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("grouping", "opaque")), boolean, "opaque"))
			cases := []struct {
				name string
				root expressions.Expression
			}{
				{"literal", truth},
				{"NULL Boolean", null},
				{"array literal", candidates},
				{"NULL array", nullArray},
				{"reference", flag},
				{"array reference", array},
				{"contains", contains},
				{"contains any", containsAny},
				{"NOT", exclusion},
				{"positive membership group", positive},
				{"nested AND", nested},
				{"shared OR operands", repeated},
				{"enclosing call", outer},
				{"IN", membership},
				{"opaque", opaque},
			}
			for _, tc := range cases {
				if err := utils.ValidateForOptimization(tc.root); err != nil {
					t.Fatal(err)
				}
			}
			provider.FunctionContext = nil
			tree := rewriter.NewExpressionTreeRewriter[struct{}](array_exclusion_grouping_rewriter.NewArrayExclusionGroupingRewriter(testbedvalues.ValueFactory{}))
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					before := expressions.PrettyPrint(tc.root)
					children := tc.root.Children()
					saved := slices.Clone(children)
					result := must(tree.Rewrite(tc.root, struct{}{}))
					if result != tc.root || !result.Type().EqualsTo(tc.root.Type()) {
						t.Fatal("rewriter changed identity or exact Type")
					}
					if expressions.PrettyPrint(tc.root) != before || !slices.Equal(children, saved) || !slices.Equal(tc.root.Children(), saved) {
						t.Fatal("source or child slice changed")
					}
					if again := must(tree.Rewrite(result, struct{}{})); again != result {
						t.Fatal("second pass changed identity")
					}
				})
			}
			elements := candidates.Value().(*testbedvalues.ArrayValue).Elements()
			if repeated.Children()[0] != nested || repeated.Children()[1] != nested || contains.Arguments()[0] != array || elements[0] != one || elements[1] != two {
				t.Fatal("shared expression or literal Value identity changed")
			}
		})
	}
}
