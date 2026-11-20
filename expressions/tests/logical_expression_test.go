package expressions_test

import (
	"strings"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func logicalLiteral(n foundation.Nullability, b bool) *expressions.LiteralExpression {
	return fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewBool(fixture(types.NewBoolType(n)), b))))
}

func TestLogicalExpressionTypingAndOwnership(t *testing.T) {
	yes, no := logicalLiteral(foundation.NonNull, true), logicalLiteral(foundation.NonNull, false)
	nullable := logicalLiteral(foundation.MayNull, true)
	null := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewNull(nullable.Type()))))
	for _, tc := range []struct {
		name     string
		op       foundation.LogicalOperator
		children []expressions.Expression
		n        foundation.Nullability
	}{
		{"not", foundation.Not, []expressions.Expression{yes}, foundation.NonNull},
		{"not nullable payload", foundation.Not, []expressions.Expression{nullable}, foundation.MayNull},
		{"not null", foundation.Not, []expressions.Expression{null}, foundation.MayNull},
		{"and", foundation.And, []expressions.Expression{yes, no}, foundation.NonNull},
		{"or", foundation.Or, []expressions.Expression{yes, no, yes}, foundation.NonNull},
		{"and conservative", foundation.And, []expressions.Expression{no, nullable}, foundation.MayNull},
		{"or conservative", foundation.Or, []expressions.Expression{yes, null}, foundation.MayNull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]expressions.Expression(nil), tc.children...)
			node := fixture(expressions.NewLogicalExpression(tc.op, tc.children))
			if _, ok := node.Type().(*types.BoolType); !ok || node.Type().Nullability() != tc.n || node.Operator() != tc.op {
				t.Fatal("incorrect logical type/operator")
			}
			tc.children[0] = null
			returned := node.Children()
			returned[0] = null
			if len(node.Children()) != len(original) {
				t.Fatal("constructor simplified children")
			}
			for i, child := range node.Children() {
				if child != original[i] {
					t.Fatal("child ownership lost")
				}
			}
			if !strings.Contains(node.String(), tc.op.String()) || !strings.Contains(node.String(), node.Type().String()) {
				t.Fatal("missing diagnostic details")
			}
		})
	}
}

func TestLogicalExpressionStructuralEquality(t *testing.T) {
	yes, no := logicalLiteral(foundation.NonNull, true), logicalLiteral(foundation.NonNull, false)
	a := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, no}))
	b := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{logicalLiteral(foundation.NonNull, true), logicalLiteral(foundation.NonNull, false)}))
	if !a.EqualsTo(b) || !b.EqualsTo(a) || !a.EqualsTo(a) {
		t.Fatal("structural equality failed")
	}
	for _, other := range []expressions.Expression{
		fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{no, yes})),
		fixture(expressions.NewLogicalExpression(foundation.Or, []expressions.Expression{yes, no})),
		fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, no, yes})),
		fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, logicalLiteral(foundation.MayNull, false)})),
		no,
	} {
		if a.EqualsTo(other) || other.EqualsTo(a) {
			t.Fatal("order/operator/arity/type/variant ignored")
		}
	}
	nested := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{a, a}))
	flat := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, no, yes, no}))
	if nested.EqualsTo(flat) || nested.Children()[0] != a || nested.Children()[1] != a {
		t.Fatal("grouping or sharing changed")
	}
	provider := fixture(testbed.NewStandardContext())
	handle := fixture(provider.LookupBuiltInFunction(functions.BuiltInNot, []types.Type{yes.Type()}))
	call := fixture(expressions.NewCallExpression(provider, handle, []expressions.Expression{yes}))
	not := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{yes}))
	if call.EqualsTo(not) || not.EqualsTo(call) {
		t.Fatal("logical NOT and call conflated")
	}
}

func TestLogicalExpressionInvalidAndNil(t *testing.T) {
	yes := logicalLiteral(foundation.NonNull, true)
	number := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(fixture(types.NewIntType(types.Int64, foundation.NonNull)), 1))))
	for _, tc := range []struct {
		op       foundation.LogicalOperator
		children []expressions.Expression
	}{
		{0, []expressions.Expression{yes}},
		{255, []expressions.Expression{yes}},
		{foundation.Not, nil},
		{foundation.Not, []expressions.Expression{yes, yes}},
		{foundation.And, nil},
		{foundation.And, []expressions.Expression{yes}},
		{foundation.Or, nil},
		{foundation.Or, []expressions.Expression{yes}},
		{foundation.Not, []expressions.Expression{number}},
		{foundation.And, []expressions.Expression{yes, number}},
		{foundation.Or, []expressions.Expression{number, yes}},
	} {
		if node, err := expressions.NewLogicalExpression(tc.op, tc.children); node != nil || err == nil {
			t.Fatal("invalid logical expression accepted")
		}
	}
	valid := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{yes}))
	var nilNode *expressions.LogicalExpression
	for _, call := range []func(){
		func() { _, _ = expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{nil}) },
		func() {
			_, _ = expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{(*expressions.LiteralExpression)(nil)})
		},
		func() { valid.EqualsTo(nil) }, func() { valid.EqualsTo(nilNode) }, func() { nilNode.EqualsTo(valid) },
		func() { nilNode.Operator() }, func() { nilNode.Type() }, func() { nilNode.Children() }, func() { _ = nilNode.String() },
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("nil did not assert")
				}
			}()
			call()
		}()
	}
}
