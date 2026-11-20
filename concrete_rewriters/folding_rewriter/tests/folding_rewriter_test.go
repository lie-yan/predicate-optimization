package folding_rewriter_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/folding_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
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

type environment struct {
	id    expressioninterfaces.ColumnID
	value valueinterfaces.Value
	err   error
}

func (e environment) Lookup(id expressioninterfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	if e.err != nil {
		return foundation.Optional[valueinterfaces.Value]{}, e.err
	}
	if e.id != nil && e.id.EqualsTo(id) {
		return foundation.Some(e.value), nil
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

func integer(n int64, nullable foundation.Nullability) *expressions.LiteralExpression {
	typ := must(types.NewIntType(types.Int64, nullable))
	return must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(typ, n))))
}

func arithmetic(p functioninterfaces.FunctionContext, op foundation.OperatorType, args ...expressions.Expression) *expressions.CallExpression {
	typesOf := make([]types.Type, len(args))
	for i, arg := range args {
		typesOf[i] = arg.Type()
	}
	return must(expressions.NewCallExpression(p, must(p.ArithmeticFunction(op, typesOf)), args))
}

func tree(p functioninterfaces.FunctionContext, preserve bool) rewriter.ExpressionTreeRewriter[interfaces.Environment] {
	return rewriter.NewExpressionTreeRewriter[interfaces.Environment](folding_rewriter.NewFoldingRewriter(interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, p), preserve))
}

func TestFoldsArithmeticComparisonAndContainment(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	one, two := integer(1, foundation.NonNull), integer(2, foundation.MayNull)
	sum := arithmetic(provider, foundation.Add, one, two)
	comparison := must(expressions.NewCallExpression(provider, must(provider.ComparisonFunction(foundation.Equal, sum.Type(), two.Type())), []expressions.Expression{sum, two}))
	arrayType := must(types.NewArrayType(one.Type(), foundation.NonNull))
	array := must(expressions.NewLiteralExpression(must(testbedvalues.NewArray(arrayType, []valueinterfaces.Value{one.Value()}))))
	handle := must(provider.LookupBuiltInFunction(functions.BuiltInArrayContains, []types.Type{array.Type(), one.Type()}))
	contains := must(expressions.NewCallExpression(provider, handle, []expressions.Expression{array, one}))
	rewrite := tree(provider, false)
	for _, node := range []expressions.Expression{sum, comparison, contains} {
		got := must(rewrite.Rewrite(node, environment{}))
		literal, ok := got.(*expressions.LiteralExpression)
		if !ok || !got.Type().EqualsTo(node.Type()) {
			t.Fatal("folded value/type lost")
		}
		expected := must(interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{}).Evaluate(node, environment{}))
		if !literal.Value().EqualsTo(expected) {
			t.Fatal("folded value differs")
		}
	}
	if sum.Arguments()[0] != one || comparison.Arguments()[0] != sum {
		t.Fatal("original tree mutated")
	}
	if got := must(rewrite.Rewrite(one, environment{})); got != one {
		t.Fatal("unchanged literal not shared")
	}
	if got := must(rewrite.Rewrite(sum, environment{})).(*expressions.LiteralExpression).Value().(*testbedvalues.IntegerValue).Integer(); got != 3 {
		t.Fatal("wrong arithmetic result")
	}
}

func TestResidualAndAdmissionPreservation(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	rewrite := tree(provider, false)
	one := integer(1, foundation.NonNull)
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), one.Type(), "x"))
	inner := arithmetic(provider, foundation.Add, one, one)
	root := arithmetic(provider, foundation.Add, ref, inner)
	got := must(rewrite.Rewrite(root, environment{}))
	if got.Children()[0] != ref || got.Children()[1].(*expressions.LiteralExpression).Value().(*testbedvalues.IntegerValue).Integer() != 2 {
		t.Fatal("partial call wrong")
	}
	// Dynamic IN candidates stay gated even when their expression could fold.
	candidate := arithmetic(provider, foundation.Add, one, one)
	membership := must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, must(types.NewBoolType(foundation.NonNull)), []expressions.Expression{one, candidate}))
	if got := must(rewrite.Rewrite(membership, environment{})); got != membership {
		t.Fatal("folding traversed gated region")
	}
	overflow := arithmetic(provider, foundation.Add, integer(math.MaxInt64, foundation.NonNull), one)
	if got := must(rewrite.Rewrite(overflow, environment{})); got != overflow {
		t.Fatal("overflow fabricated a value")
	}
	bound := environment{id: ref.ID(), value: one.Value()}
	if got := must(rewrite.Rewrite(ref, bound)).(*expressions.LiteralExpression).Value(); got != one.Value() {
		t.Fatal("bound reference not folded")
	}
}

func TestSemanticFailurePolicy(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	one, zero := integer(1, foundation.NonNull), integer(0, foundation.NonNull)
	left := arithmetic(provider, foundation.Add, one, one)
	failure := arithmetic(provider, foundation.Divide, one, zero)
	root := arithmetic(provider, foundation.Add, left, failure)
	if got, err := tree(provider, false).Rewrite(root, environment{}); got != nil || !errors.Is(err, functions.ErrEvaluationFailure) {
		t.Fatal("semantic failure not propagated", err)
	}
	if got := must(tree(provider, true).Rewrite(root, environment{})); got != root || root.Children()[0] != left {
		t.Fatal("failure did not preserve whole original")
	}
}

func TestContractErrorsAlwaysPropagate(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	one := integer(1, foundation.NonNull)
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "x")), one.Type(), "x"))
	rewrite := tree(provider, true)
	for _, err := range []error{functions.ErrContractViolation, errors.Join(functions.ErrEvaluationFailure, functions.ErrContractViolation), errors.New("provider failed")} {
		if got, cause := rewrite.Rewrite(ref, environment{err: err}); got != nil || !errors.Is(cause, err) {
			t.Fatal("error concealed", cause)
		}
	}
	invalid := environment{id: ref.ID(), value: integer(1, foundation.MayNull).Value()}
	if _, err := rewrite.Rewrite(ref, invalid); !errors.Is(err, interpreter.ErrInvalidBinding) {
		t.Fatal(err)
	}
}
