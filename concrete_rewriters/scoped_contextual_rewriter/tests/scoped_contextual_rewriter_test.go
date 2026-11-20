package scoped_contextual_rewriter_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
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

// Construction helpers are local to this rewriter's external-package tests.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func integer(n int64, nullable foundation.Nullability) *expressions.LiteralExpression {
	typ := must(types.NewIntType(types.Int64, nullable))
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(typ, n))))
}

// A nil embedded interface makes any constructor-side provider query fail.
// This fixture checks construction only, not the execution preconditions.
type constructorOnlyContext struct {
	functioninterfaces.FunctionContext
}

func TestScopedContextualConstructorDoesNotQueryProvider(t *testing.T) {
	if scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, &constructorOnlyContext{}) == nil {
		t.Fatal("constructor returned nil")
	}
}

func TestScopedContextualConstructorRejectsNil(t *testing.T) {
	for _, tc := range []struct {
		name    string
		context functioninterfaces.FunctionContext
	}{
		{"nil", nil},
		{"typed nil", (*testbed.Context)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("nil context did not assert")
				}
			}()
			scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, tc.context)
		})
	}
}

type noEvaluationContext struct {
	functioninterfaces.FunctionContext
}

func (*noEvaluationContext) Evaluate(functioninterfaces.FunctionHandle, *functions.FunctionSignature, []valueinterfaces.Value) (valueinterfaces.Value, error) {
	panic("scoped contextual rewriter must not evaluate")
}

func TestScopedContextualPreservesTreesWithoutUsefulGuard(t *testing.T) {
	context := &noEvaluationContext{must(testbed.NewStandardContext())}
	one, two := integer(1, foundation.NonNull), integer(2, foundation.NonNull)
	x := must(expressions.NewReferenceExpression(must(ids.NewColumnID("example", "x")), one.Type(), "x"))
	boolean := must(types.NewBoolType(foundation.NonNull))
	guard := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{x, one, two}))
	handle := must(context.ComparisonFunction(foundation.Equal, x.Type(), one.Type()))
	comparison := must(expressions.NewCallExpression(context, handle, []expressions.Expression{x, one}))
	not := must(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{comparison}))
	// A mixed-reference conjunction supplies no same-operand contextual target.
	y := must(expressions.NewReferenceExpression(must(ids.NewColumnID("example", "y")), one.Type(), "y"))
	other := must(expressions.NewCallExpression(context, handle, []expressions.Expression{y, one}))
	guarded := must(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{comparison, other}))
	shared := must(expressions.NewLogicalExpression(foundation.Or, []expressions.Expression{guarded, guarded}))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("adapter", "region")), boolean, "unsupported"))
	null := must(expressions.NewLiteralExpression(must(testbedvalues.NewNull(must(types.NewBoolType(foundation.MayNull))))))
	rule := scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, context)
	tree := rewriter.NewExpressionTreeRewriter[struct{}](rule)
	for _, root := range []expressions.Expression{one, x, guard, comparison, not, guarded, shared, opaque, null} {
		before := expressions.PrettyPrint(root)
		got := must(tree.Rewrite(root, struct{}{}))
		if got != root || !got.Type().EqualsTo(root.Type()) || expressions.PrettyPrint(root) != before {
			t.Fatal("rewriting changed a tree without a useful guard or its Type")
		}
	}
	if children := shared.Children(); children[0] != guarded || children[1] != guarded {
		t.Fatal("subtree sharing changed")
	}
}
