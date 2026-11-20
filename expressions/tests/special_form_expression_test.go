package expressions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	columns "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestSpecialFormExpressionArguments(t *testing.T) {
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	ref := fixture(expressions.NewReferenceExpression(fixture(columns.NewColumnID("table", "review")), integer, "is_review"))
	args := []expressions.Expression{ref}
	for _, n := range []int64{8795001, 8795002, 8795003, 8795004, 8795005, 8795002} {
		args = append(args, fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, n)))))
	}
	original := append([]expressions.Expression(nil), args...)
	node := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, args))
	if node.Type() != boolean || node.Form() != foundation.SpecialFormIn || len(node.Children()) != len(args) {
		t.Fatal("form/type/children lost")
	}
	args[0] = args[1]
	returned := node.Arguments()
	returned[1] = ref
	children := node.Children()
	children[0] = nil
	for i, child := range node.Children() {
		if child != original[i] {
			t.Fatal("ordered child ownership lost")
		}
	}
	same := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, fixture(types.NewBoolType(foundation.NonNull)), original))
	if !node.EqualsTo(same) || !same.EqualsTo(node) {
		t.Fatal("structural equality failed")
	}
	reversed := append([]expressions.Expression(nil), original...)
	reversed[1], reversed[2] = reversed[2], reversed[1]
	different := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, reversed))
	if node.EqualsTo(different) {
		t.Fatal("candidate order ignored")
	}
	negated := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{node}))
	if negated.Children()[0] != node || node.EqualsTo(negated) || negated.EqualsTo(node) {
		t.Fatal("explicit logical NOT must retain its child")
	}
	// Dynamic candidates are ordinary expression children, independent of the
	// narrower initial interpreter scope for literal candidate lists.
	dynamic := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{ref, ref}))
	if dynamic.Children()[1] != ref {
		t.Fatal("dynamic candidate not shared")
	}
	empty := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{ref}))
	if len(empty.Arguments()) != 1 {
		t.Fatal("empty candidate list not retained")
	}
	if node.EqualsTo(empty) || node.EqualsTo(ref) {
		t.Fatal("arity or variant ignored")
	}
}

func TestSpecialFormExpressionNullableCandidates(t *testing.T) {
	for _, width := range []types.IntWidth{types.Int8, types.Int16, types.Int32, types.Int64} {
		integer := fixture(types.NewIntType(width, foundation.NonNull))
		nullable := fixture(integer.WithNullability(foundation.MayNull))
		target := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
		for _, candidate := range []valueinterfaces.Value{fixture(testbedvalues.NewNull(nullable)), fixture(testbedvalues.NewInteger(nullable.(*types.IntType), 1))} {
			child := fixture(expressions.NewLiteralExpression(candidate))
			args := []expressions.Expression{target, child}
			result := fixture(types.NewBoolType(foundation.MayNull))
			node := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, result, args))
			negated := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{node}))
			if !negated.Type().EqualsTo(result) || negated.Children()[0] != node {
				t.Fatal("NOT IN wrapper lost nullable contract")
			}
			if node.Children()[1] != child || node.Type() != result {
				t.Fatal("nullable candidate/type changed")
			}
			if got, err := expressions.NewSpecialFormExpression(foundation.SpecialFormIn, fixture(types.NewBoolType(foundation.NonNull)), args); got != nil || err == nil {
				t.Fatal("incorrect result nullability accepted")
			}
		}
	}
	for _, value := range []valueinterfaces.Value{
		fixture(testbedvalues.NewBool(fixture(types.NewBoolType(foundation.NonNull)), true)),
		fixture(testbedvalues.NewVarChar(fixture(types.NewVarCharType(nil, foundation.NonNull)), "tsa_01")),
	} {
		literal := fixture(expressions.NewLiteralExpression(value))
		node := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, fixture(types.NewBoolType(foundation.NonNull)), []expressions.Expression{literal, literal}))
		if len(node.Children()) != 2 {
			t.Fatal("primitive IN not represented")
		}
	}
}

func TestSpecialFormExpressionCompositeTypes(t *testing.T) {
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	array := fixture(types.NewArrayType(integer, foundation.NonNull))
	nested := fixture(types.NewArrayType(array, foundation.NonNull))
	structure := fixture(types.NewStructType([]types.StructField{{Name: "items", Type: array}}, foundation.NonNull))
	reference := func(typ types.Type) expressions.Expression {
		return fixture(expressions.NewReferenceExpression(fixture(columns.NewColumnID("table", "value")), typ, "value"))
	}
	for _, typ := range []types.Type{array, nested, structure} {
		t.Run(typ.String(), func(t *testing.T) {
			for _, targetNullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
				for _, candidateNullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
					target := reference(fixture(typ.WithNullability(targetNullability)))
					candidate := reference(fixture(typ.WithNullability(candidateNullability)))
					nullability := targetNullability
					if candidateNullability == foundation.MayNull {
						nullability = foundation.MayNull
					}
					result := fixture(types.NewBoolType(nullability))
					node := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, result, []expressions.Expression{target, candidate}))
					if node.ChildAt(0) != target || node.ChildAt(1) != candidate || !node.Type().EqualsTo(result) {
						t.Fatal("composite arguments or result type changed")
					}
				}
			}
		})
	}
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	nullableElement := fixture(types.NewArrayType(fixture(integer.WithNullability(foundation.MayNull)), foundation.NonNull))
	for _, candidate := range []types.Type{integer, nested, structure, nullableElement} {
		if node, err := expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{reference(array), reference(candidate)}); node != nil || err == nil {
			t.Errorf("nonuniform candidate %s accepted for %s", candidate, array)
		}
	}
}

func TestSpecialFormExpressionInvalidAndNil(t *testing.T) {
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	yes := logicalLiteral(foundation.NonNull, true)
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	number := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	for _, form := range []foundation.SpecialForm{0, 255} {
		if node, err := expressions.NewSpecialFormExpression(form, boolean, []expressions.Expression{yes, yes}); node != nil || err == nil {
			t.Fatal("unimplemented/invalid form accepted")
		}
	}
	for _, args := range [][]expressions.Expression{nil, {yes, number}} {
		if node, err := expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, args); node != nil || err == nil {
			t.Fatal("invalid IN accepted")
		}
	}
	for _, result := range []types.Type{integer, fixture(types.NewBoolType(foundation.MayNull))} {
		if node, err := expressions.NewSpecialFormExpression(foundation.SpecialFormIn, result, []expressions.Expression{yes, yes}); node != nil || err == nil {
			t.Fatal("incorrect result type accepted")
		}
	}
	valid := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{yes, yes}))
	var nilNode *expressions.SpecialFormExpression
	for _, call := range []func(){
		func() {
			_, _ = expressions.NewSpecialFormExpression(foundation.SpecialFormIn, nil, []expressions.Expression{yes})
		},
		func() {
			_, _ = expressions.NewSpecialFormExpression(foundation.SpecialFormIn, (*types.BoolType)(nil), []expressions.Expression{yes})
		},
		func() {
			_, _ = expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{nil})
		},
		func() {
			_, _ = expressions.NewSpecialFormExpression(foundation.SpecialFormIn, boolean, []expressions.Expression{(*expressions.LiteralExpression)(nil)})
		},
		func() { valid.EqualsTo(nil) }, func() { valid.EqualsTo(nilNode) }, func() { nilNode.EqualsTo(valid) },
		func() { nilNode.Form() }, func() { nilNode.Type() }, func() { nilNode.Arguments() }, func() { nilNode.Children() }, func() { _ = nilNode.String() },
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
