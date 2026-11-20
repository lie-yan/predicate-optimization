package array_membership_normalization_rewriter_test

import (
	"testing"

	normalization "github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

func TestNormalizeExternalArrayPayloads(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	nestedType := must(types.NewArrayType(integer, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	for _, element := range []valueinterfaces.Value{fixtures.WrapScalar(must(testbedvalues.NewInteger(integer, 7))), fixtures.WrapArray(must(testbedvalues.NewArray(nestedType, nil)))} {
		array := must(types.NewArrayType(element.Type(), foundation.NonNull))
		policies := []functions.NullPolicy{functions.PropagateNull, functions.NullPolicyUnspecified}
		context := testbed.NewContext(must(testbed.NewCatalog([]testbed.Definition{
			membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{array, array}, boolean, policies),
			membershipDefinition(functions.BuiltInArrayContains, []types.Type{array, element.Type()}, boolean, policies),
		})))
		input := must(expressions.NewReferenceExpression(must(ids.NewColumnID("external", "array")), array, "array"))
		tree := rewriter.NewExpressionTreeRewriter[struct{}](normalization.NewArrayMembershipNormalizationRewriter())
		for _, children := range [][]valueinterfaces.Value{nil, {element}, {element, element}} {
			view := fixtures.WrapArray(must(testbedvalues.NewArray(array, children)))
			candidate := must(expressions.NewLiteralExpression(view))
			handle := must(context.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{array, array}))
			source := must(expressions.NewCallExpression(context, handle, []expressions.Expression{input, candidate}))
			result := must(tree.Rewrite(source, struct{}{}))
			if len(children) != 1 {
				if result != source {
					t.Fatal("non-singleton normalized")
				}
				continue
			}
			call := result.(*expressions.CallExpression)
			if name, _ := must(context.Metadata(call.Handle())).BuiltInName().Get(); name != functions.BuiltInArrayContains {
				t.Fatal("singleton not normalized")
			}
			value := call.Arguments()[1].(*expressions.LiteralExpression).Value()
			if !value.EqualsTo(element) || !result.Type().EqualsTo(source.Type()) {
				t.Fatal("element or result type changed")
			}
			switch element.(type) {
			case fixtures.ScalarView:
				if _, ok := value.(fixtures.ScalarView); !ok {
					t.Fatal("scalar representation lost")
				}
			case fixtures.ArrayView:
				if _, ok := value.(fixtures.ArrayView); !ok {
					t.Fatal("nested representation lost")
				}
			}
		}
		// Typed null arrays have no readable payload, even when wrapped externally.
		nullable := must(array.WithNullability(foundation.MayNull))
		null := must(expressions.NewLiteralExpression(fixtures.WrapScalar(must(testbedvalues.NewNull(nullable)))))
		nullContext := testbed.NewContext(must(testbed.NewCatalog([]testbed.Definition{
			membershipDefinition(functions.BuiltInArrayContainsAny, []types.Type{array, nullable}, boolean, []functions.NullPolicy{functions.PropagateNull, functions.PassNull}),
		})))
		handle := must(nullContext.LookupBuiltInFunction(functions.BuiltInArrayContainsAny, []types.Type{array, nullable}))
		source := must(expressions.NewCallExpression(nullContext, handle, []expressions.Expression{input, null}))
		if must(tree.Rewrite(source, struct{}{})) != source {
			t.Fatal("null candidate normalized")
		}
	}
}
