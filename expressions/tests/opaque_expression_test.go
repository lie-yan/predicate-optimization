package expressions_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	interfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbed "github.com/lie-yan/predicate-optimization/testbed/expressions"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestOpaqueExpressionIdentity(t *testing.T) {
	boolean := fixture(types.NewBoolType(foundation.MayNull))
	id := fixture(testbed.NewOpaqueID("adapter-a", "region-1"))
	node := fixture(expressions.NewOpaqueExpression(id, boolean, "unsupported"))
	if node.ID() != id || node.Type() != boolean || node.DisplayName() != "unsupported" || len(node.Children()) != 0 {
		t.Fatal("preservation contract lost")
	}
	for _, tc := range []struct {
		name    string
		id      interfaces.OpaqueID
		typ     types.Type
		display string
		equal   bool
	}{
		{"same region", id, fixture(types.NewBoolType(foundation.MayNull)), "unsupported", true},
		{"diagnostic rename", id, boolean, "renamed", true},
		{"empty diagnostic", id, boolean, "", true},
		{"different token", fixture(testbed.NewOpaqueID("adapter-a", "region-2")), boolean, "unsupported", false},
		{"different owner", fixture(testbed.NewOpaqueID("adapter-b", "region-1")), boolean, "unsupported", false},
		{"nullability", id, fixture(types.NewBoolType(foundation.NonNull)), "unsupported", false},
		{"domain", id, fixture(types.NewIntType(types.Int64, foundation.MayNull)), "unsupported", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := fixture(expressions.NewOpaqueExpression(tc.id, tc.typ, tc.display))
			if node.EqualsTo(other) != tc.equal || other.EqualsTo(node) != tc.equal {
				t.Fatal("incorrect structural identity")
			}
		})
	}
	if !node.EqualsTo(node) {
		t.Fatal("not reflexive")
	}
	literal := logicalLiteral(foundation.MayNull, true)
	if node.EqualsTo(literal) || literal.EqualsTo(node) {
		t.Fatal("variant ignored")
	}
}

func TestOpaqueExpressionPreservationAcrossRoots(t *testing.T) {
	typ := fixture(types.NewBoolType(foundation.MayNull))
	opaque := fixture(expressions.NewOpaqueExpression(fixture(testbed.NewOpaqueID("adapter", "r")), typ, "opaque"))
	// Only the adapter keeps original source data. Node copies still resolve it.
	originalID := opaque.ID()
	resolve := func(id interfaces.OpaqueID) string {
		if id.EqualsTo(originalID) {
			return "original unsupported expression"
		}
		return ""
	}
	copied := fixture(expressions.NewOpaqueExpression(opaque.ID(), opaque.Type(), "copy"))
	if !copied.EqualsTo(opaque) || resolve(copied.ID()) != "original unsupported expression" {
		t.Fatal("preservation token lost")
	}
	yes, no := logicalLiteral(foundation.NonNull, true), logicalLiteral(foundation.NonNull, false)
	oldRoot := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{yes, opaque}))
	newRoot := fixture(expressions.NewLogicalExpression(foundation.And, []expressions.Expression{no, opaque}))
	if oldRoot.Children()[0] != yes || newRoot.Children()[0] != no || oldRoot.Children()[1] != opaque || newRoot.Children()[1] != opaque {
		t.Fatal("opaque sharing or original root changed")
	}
	// This constructs two roots; it does not claim their predicates equivalent
	// or implement checked reconstruction.
	array := fixture(types.NewArrayType(fixture(types.NewIntType(types.Int64, foundation.NonNull)), foundation.MayNull))
	composite := fixture(expressions.NewOpaqueExpression(fixture(testbed.NewOpaqueID("adapter", "array")), array, "array-result"))
	if composite.Type() != array || len(composite.Children()) != 0 {
		t.Fatal("composite result exposed internals")
	}
}

func TestOpaqueExpressionInvalidAndNil(t *testing.T) {
	typ := fixture(types.NewBoolType(foundation.NonNull))
	id := fixture(testbed.NewOpaqueID("adapter", "token"))
	valid := fixture(expressions.NewOpaqueExpression(id, typ, "a\n\"b"))
	want := `OpaqueExpression(id=OpaqueID(owner="adapter", token="token"), type=Bool!, name="a\n\"b")`
	if valid.String() != want {
		t.Fatalf("got %q, want %q", valid.String(), want)
	}
	var nilNode *expressions.OpaqueExpression
	for _, call := range []func(){
		func() { _, _ = expressions.NewOpaqueExpression(nil, typ, "") },
		func() { _, _ = expressions.NewOpaqueExpression((*testbed.OpaqueID)(nil), typ, "") },
		func() { _, _ = expressions.NewOpaqueExpression(id, nil, "") },
		func() { _, _ = expressions.NewOpaqueExpression(id, (*types.BoolType)(nil), "") },
		func() { valid.EqualsTo(nil) }, func() { valid.EqualsTo(nilNode) }, func() { nilNode.EqualsTo(valid) },
		func() { nilNode.ID() }, func() { nilNode.Type() }, func() { nilNode.DisplayName() }, func() { nilNode.Children() }, func() { _ = nilNode.String() },
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

// A non-comparable external implementation verifies caller-controlled identity.
type externalOpaqueID struct{ path []string }

func (id externalOpaqueID) String() string                          { return "external region" }
func (id externalOpaqueID) Key() string                             { return fmt.Sprintf("expression-tests/opaque:%q", id.path) }
func (id externalOpaqueID) EqualsTo(other interfaces.OpaqueID) bool { return id.Key() == other.Key() }

func TestOpaqueExpressionCallerID(t *testing.T) {
	typ := fixture(types.NewBoolType(foundation.MayNull))
	a := fixture(expressions.NewOpaqueExpression(externalOpaqueID{[]string{"owner", "region"}}, typ, "a"))
	b := fixture(expressions.NewOpaqueExpression(externalOpaqueID{[]string{"owner", "region"}}, typ, "b"))
	c := fixture(expressions.NewOpaqueExpression(externalOpaqueID{[]string{"other", "region"}}, typ, "a"))
	other := fixture(expressions.NewOpaqueExpression(fixture(testbed.NewOpaqueID("owner", "region")), typ, "a"))
	if !a.EqualsTo(b) || !b.EqualsTo(a) || a.EqualsTo(c) || a.EqualsTo(other) || other.EqualsTo(a) {
		t.Fatal("caller identity was not respected")
	}
}
