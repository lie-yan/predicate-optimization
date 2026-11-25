package expressions_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestWithChildrenLeavesAndUnchangedNodes(t *testing.T) {
	yes := logicalLiteral(foundation.NonNull, true)
	ref := fixture(expressions.NewReferenceExpression(fixture(ids.NewColumnID("t", "x")), yes.Type(), "x"))
	opaque := fixture(expressions.NewOpaqueExpression(fixture(ids.NewOpaqueID("owner", "x")), yes.Type(), "opaque"))
	logical := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, ref}))
	in := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, yes.Type(), []expressions.Expression{ref, yes}))
	for _, node := range []expressions.Expression{yes, ref, opaque, logical, in} {
		got := fixture(node.WithChildren(node.Children()))
		if got != node {
			t.Fatalf("unchanged %T not reused", node)
		}
		if _, err := node.WithChildren(append(node.Children(), yes)); len(node.Children()) == 0 && err == nil {
			t.Fatal("changed arity accepted")
		}
	}
}

func TestWithChildrenSharesChildrenAndPreservesOriginal(t *testing.T) {
	yes, no := logicalLiteral(foundation.NonNull, true), logicalLiteral(foundation.NonNull, false)
	opaque := fixture(expressions.NewOpaqueExpression(fixture(ids.NewOpaqueID("owner", "x")), yes.Type(), "opaque"))
	root := fixture(expressions.NewLogicalExpression(foundation.Or, []expressions.Expression{yes, opaque}))
	children := []expressions.Expression{no, opaque}
	rebuilt := fixture(root.WithChildren(children)).(*expressions.LogicalExpression)
	children[1] = no
	if rebuilt == root || rebuilt.Operator() != foundation.Or || rebuilt.Children()[0] != no || rebuilt.Children()[1] != opaque {
		t.Fatal("reconstruction lost operator, child ownership or sharing")
	}
	if root.Children()[0] != yes || root.Children()[1] != opaque {
		t.Fatal("original changed")
	}
	// Equal content in a newly allocated child is still an explicit replacement.
	equal := logicalLiteral(foundation.NonNull, true)
	sameContent := fixture(root.WithChildren([]expressions.Expression{equal, opaque}))
	if sameContent == root || sameContent.Children()[0] != equal || !sameContent.EqualsTo(root) {
		t.Fatal("replacement identity discarded")
	}
}

func TestWithChildrenMembership(t *testing.T) {
	yes, no := logicalLiteral(foundation.NonNull, true), logicalLiteral(foundation.NonNull, false)
	root := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, yes.Type(), []expressions.Expression{yes, no, yes}))
	rebuilt := fixture(root.WithChildren([]expressions.Expression{no, yes, no})).(*expressions.SpecialFormExpression)
	if rebuilt == root || rebuilt.Form() != root.Form() || !rebuilt.Type().EqualsTo(root.Type()) || rebuilt.Arguments()[0] != no {
		t.Fatal("membership payload or argument order lost")
	}
	nullable := logicalLiteral(foundation.MayNull, true)
	if _, err := root.WithChildren([]expressions.Expression{nullable, no, yes}); err == nil {
		t.Fatal("nullable result change accepted")
	}
}

func TestWithChildrenCallsRebindWithoutEvaluation(t *testing.T) {
	provider := fixture(testbed.NewStandardContext())
	ctx := &validationContext{FunctionContext: provider}
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	nullable := fixture(types.NewIntType(types.Int64, foundation.MayNull))
	one := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	two := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 2))))
	maybe := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(nullable, 1))))
	handle := fixture(provider.ArithmeticFunction(foundation.Add, []types.Type{integer, integer}))
	root := fixture(expressions.NewCallExpression(ctx, handle, []expressions.Expression{one, two}))
	if _, err := root.WithChildren([]expressions.Expression{one}); !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatalf("call arity: %v", err)
	}
	ctx.queries = 0
	if fixture(root.WithChildren(root.Children())) != root || ctx.queries != 0 {
		t.Fatal("unchanged call queried metadata")
	}
	rebuilt := fixture(root.WithChildren([]expressions.Expression{two, one})).(*expressions.CallExpression)
	if rebuilt == root || !rebuilt.Handle().EqualsTo(handle) || ctx.queries != 1 {
		t.Fatal("changed call not rebound")
	}
	twice := fixture(rebuilt.WithChildren([]expressions.Expression{one, two}))
	if !twice.EqualsTo(root) || ctx.queries != 2 {
		t.Fatal("rebuilt call lost its context")
	}
	if err := utils.ValidateForOptimization(rebuilt); err != nil {
		t.Fatal(err)
	}
	if _, err := root.WithChildren([]expressions.Expression{maybe, two}); err == nil {
		t.Fatal("result nullability change accepted")
	}
	if _, err := root.WithChildren([]expressions.Expression{logicalLiteral(foundation.NonNull, true), two}); !errors.Is(err, functions.ErrUnsupportedLookup) {
		t.Fatalf("wrong argument type error: %v", err)
	}
	// A nullable sibling keeps the result nullable while the first argument narrows.
	nullableRoot := fixture(expressions.NewCallExpression(ctx, handle, []expressions.Expression{maybe, maybe}))
	narrowed := fixture(nullableRoot.WithChildren([]expressions.Expression{one, maybe})).(*expressions.CallExpression)
	if !narrowed.Signature().ArgumentTypes()[0].EqualsTo(integer) || !narrowed.Type().EqualsTo(nullable) {
		t.Fatal("stale effective signature")
	}
	if root.Children()[0] != one || nullableRoot.Children()[0] != maybe {
		t.Fatal("failure or rebinding mutated original")
	}
}

func TestWithChildrenRejectsLogicalTypeChanges(t *testing.T) {
	yes := logicalLiteral(foundation.NonNull, true)
	maybe := logicalLiteral(foundation.MayNull, true)
	root := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{maybe}))
	if _, err := root.WithChildren([]expressions.Expression{yes}); err == nil {
		t.Fatal("narrowed result accepted")
	}
	adapted := fixture(yes.Value().WithNullability(foundation.MayNull))
	child := fixture(expressions.NewLiteralExpression(adapted))
	if _, err := root.WithChildren([]expressions.Expression{child}); err != nil {
		t.Fatal(err)
	}
	integer := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	invalid := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(integer, 1))))
	if _, err := root.WithChildren([]expressions.Expression{invalid}); err == nil {
		t.Fatal("non-Boolean child accepted")
	}
}

func TestWithChildrenNilPreconditions(t *testing.T) {
	yes := logicalLiteral(foundation.NonNull, true)
	root := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{yes}))
	for _, run := range []func(){
		func() { _, _ = (*expressions.LiteralExpression)(nil).WithChildren(nil) },
		func() { _, _ = root.WithChildren([]expressions.Expression{nil}) },
		func() {
			var child *expressions.LiteralExpression
			_, _ = root.WithChildren([]expressions.Expression{child})
		},
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("expected nil assertion")
				}
			}()
			run()
		}()
	}
}

func TestWithChildrenVariantArity(t *testing.T) {
	yes := logicalLiteral(foundation.NonNull, true)
	no := logicalLiteral(foundation.NonNull, false)
	for _, op := range []foundation.LogicalOperator{foundation.And, foundation.Or} {
		root := fixture(expressions.NewLogicalExpression(op, []expressions.Expression{yes, no}))
		grown := fixture(root.WithChildren([]expressions.Expression{yes, no, yes}))
		shrunk := fixture(grown.WithChildren([]expressions.Expression{no, yes}))
		if len(grown.Children()) != 3 || len(shrunk.Children()) != 2 || len(root.Children()) != 2 {
			t.Fatal("incorrect operand counts")
		}
		for _, children := range [][]expressions.Expression{nil, {yes}} {
			if _, err := root.WithChildren(children); err == nil {
				t.Fatal("invalid logical arity accepted")
			}
		}
	}
	not := fixture(expressions.NewLogicalExpression(foundation.Not, []expressions.Expression{yes}))
	for _, children := range [][]expressions.Expression{nil, {yes, no}} {
		if _, err := not.WithChildren(children); err == nil {
			t.Fatal("invalid NOT arity accepted")
		}
	}
	in := fixture(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, yes.Type(), []expressions.Expression{yes, no}))
	grown := fixture(in.WithChildren([]expressions.Expression{yes, no, yes}))
	empty := fixture(grown.WithChildren([]expressions.Expression{yes}))
	if len(empty.Children()) != 1 {
		t.Fatal("empty candidate list not preserved")
	}
	if _, err := in.WithChildren(nil); err == nil {
		t.Fatal("missing IN target accepted")
	}
}
