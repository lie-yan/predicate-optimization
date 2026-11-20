package expressions_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	interfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbed "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestReferenceExpressionIdentity(t *testing.T) {
	typ := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	id := fixture(testbed.NewColumnID("collection-a", "field-1"))
	ref := fixture(expressions.NewReferenceExpression(id, typ, "brand_id"))
	var node expressions.Expression = ref
	if ref.ID() != id || node.Type() != typ || ref.DisplayName() != "brand_id" || len(node.Children()) != 0 {
		t.Fatal("reference contract lost")
	}
	// No value or runtime environment is needed, even for a NonNull reference.
	for _, tc := range []struct {
		name    string
		id      interfaces.ColumnID
		typ     types.Type
		display string
		equal   bool
	}{
		{"same binding", id, fixture(types.NewIntType(types.Int64, foundation.NonNull)), "brand_id", true},
		{"renamed diagnostic", id, typ, "another name", true},
		{"empty diagnostic", id, typ, "", true},
		{"other scope", fixture(testbed.NewColumnID("collection-b", "field-1")), typ, "brand_id", false},
		{"other symbol", fixture(testbed.NewColumnID("collection-a", "field-2")), typ, "brand_id", false},
		{"width", id, fixture(types.NewIntType(types.Int8, foundation.NonNull)), "brand_id", false},
		{"nullability", id, fixture(typ.WithNullability(foundation.MayNull)), "brand_id", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := fixture(expressions.NewReferenceExpression(tc.id, tc.typ, tc.display))
			if ref.EqualsTo(other) != tc.equal || other.EqualsTo(ref) != tc.equal {
				t.Fatal("incorrect identity/type equality")
			}
		})
	}
	literal := fixture(expressions.NewLiteralExpression(fixture(testbedvalues.NewInteger(typ, 1))))
	if ref.EqualsTo(literal) || literal.EqualsTo(ref) {
		t.Fatal("different expression variants compare equal")
	}
	if !ref.EqualsTo(ref) {
		t.Fatal("not reflexive")
	}
}

func TestReferenceExpressionCompositeAndDisplay(t *testing.T) {
	typ := fixture(types.NewArrayType(fixture(types.NewIntType(types.Int64, foundation.NonNull)), foundation.MayNull))
	ref := fixture(expressions.NewReferenceExpression(fixture(testbed.NewColumnID("a\nb", "tags")), typ, "tag_ids"))
	if ref.Type() != typ || len(ref.Children()) != 0 {
		t.Fatal("composite input became child expressions")
	}
	want := `ReferenceExpression(column=ColumnID(scope="a\nb", symbol="tags"), type=Array<Int64!>?, name="tag_ids")`
	if ref.String() != want {
		t.Fatalf("got %q, want %q", ref.String(), want)
	}
}

func TestReferenceExpressionInvalidAndNil(t *testing.T) {
	typ := fixture(types.NewBoolType(foundation.NonNull))
	id := fixture(testbed.NewColumnID("scope", "symbol"))
	ref := fixture(expressions.NewReferenceExpression(id, typ, ""))
	var nilRef *expressions.ReferenceExpression
	for _, call := range []func(){
		func() { _, _ = expressions.NewReferenceExpression(nil, typ, "") },
		func() { _, _ = expressions.NewReferenceExpression((*testbed.ColumnID)(nil), typ, "") },
		func() { _, _ = expressions.NewReferenceExpression(id, nil, "") },
		func() { _, _ = expressions.NewReferenceExpression(id, (*types.BoolType)(nil), "") },
		func() { ref.EqualsTo(nil) }, func() { ref.EqualsTo(nilRef) }, func() { nilRef.EqualsTo(ref) },
		func() { nilRef.ID() }, func() { nilRef.Type() }, func() { nilRef.Children() }, func() { nilRef.DisplayName() }, func() { _ = nilRef.String() },
	} {
		func() {
			if !assertions.Enabled {
				return
			}
			defer func() {
				if recover() == nil {
					t.Error("nil precondition did not assert")
				}
			}()
			call()
		}()
	}
}

// An external non-comparable representation proves that shared expressions
// neither require the testbed ID nor use interface equality/map keys for IDs.
type externalColumnID struct{ path []string }

func (id externalColumnID) String() string                          { return "external column" }
func (id externalColumnID) Key() string                             { return fmt.Sprintf("expression-tests/column:%q", id.path) }
func (id externalColumnID) EqualsTo(other interfaces.ColumnID) bool { return id.Key() == other.Key() }

func TestReferenceExpressionCallerColumnID(t *testing.T) {
	typ := fixture(types.NewIntType(types.Int64, foundation.NonNull))
	a := fixture(expressions.NewReferenceExpression(externalColumnID{path: []string{"table", "column"}}, typ, "a"))
	b := fixture(expressions.NewReferenceExpression(externalColumnID{path: []string{"table", "column"}}, typ, "b"))
	c := fixture(expressions.NewReferenceExpression(externalColumnID{path: []string{"other", "column"}}, typ, "a"))
	testbedRef := fixture(expressions.NewReferenceExpression(fixture(testbed.NewColumnID("table", "column")), typ, "a"))
	if !a.EqualsTo(b) || !b.EqualsTo(a) || a.EqualsTo(c) || a.EqualsTo(testbedRef) || testbedRef.EqualsTo(a) {
		t.Fatal("reference did not delegate column identity")
	}
}
