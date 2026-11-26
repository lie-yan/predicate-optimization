package functions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func definition(t *testing.T, width types.IntWidth, n foundation.Nullability, policy shared.NullPolicy) testbed.Definition {
	t.Helper()
	argument, err := types.NewIntType(width, n)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := shared.NewFunctionSignature([]types.Type{argument}, argument)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := shared.NewFunctionMetadata(signature, "identity", foundation.Optional[shared.BuiltInFunctionName]{}, foundation.Optional[foundation.OperatorType]{}, []shared.NullPolicy{policy})
	if err != nil {
		t.Fatal(err)
	}
	return testbed.Definition{Namespace: "fixture", Name: "identity", Metadata: metadata}
}

func TestCatalogCanonicalizationAndEquality(t *testing.T) {
	first := definition(t, types.Int64, foundation.NonNull, shared.PropagateNull)
	equivalent := definition(t, types.Int64, foundation.NonNull, shared.PropagateNull)
	definitions := []testbed.Definition{
		first, equivalent,
		definition(t, types.Int8, foundation.NonNull, shared.PropagateNull),
		definition(t, types.Int64, foundation.MayNull, shared.PropagateNull),
	}
	catalog, err := testbed.NewCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}
	otherCatalog, err := testbed.NewCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}
	handles := catalog.Handles()
	if len(handles) != 3 {
		t.Fatalf("expected three canonical overloads, got %d", len(handles))
	}
	a, b, c := handles[0], catalog.Handles()[0], catalog.Handles()[0]
	if a == b || !a.EqualsTo(a) || !a.EqualsTo(b) || !b.EqualsTo(a) || !b.EqualsTo(c) || !a.EqualsTo(c) {
		t.Fatal("handle equality must use canonical identity and obey equality laws")
	}
	for _, other := range []*testbed.Handle{handles[1], handles[2], otherCatalog.Handles()[0]} {
		if a.EqualsTo(other) || other.EqualsTo(a) {
			t.Fatal("different overloads or catalog domains compare equal")
		}
	}
	definitions[0].Name = "changed"
	handles[0] = nil
	arguments := a.Signature().ArgumentTypes()
	originalType := first.Metadata.Signature().ArgumentTypes()[0]
	if arguments[0] != originalType {
		t.Fatal("immutable type descriptor was not shared")
	}
	arguments[0] = nil
	if a.Name() != "identity" || a.Namespace() != "fixture" || a.Signature().ArgumentTypes()[0] != originalType || !a.EqualsTo(catalog.Handles()[0]) {
		t.Fatal("caller-owned slice mutation changed catalog state")
	}
}

func TestCatalogRejectsConflicts(t *testing.T) {
	first := definition(t, types.Int64, foundation.MayNull, shared.PropagateNull)
	conflicting := definition(t, types.Int64, foundation.MayNull, shared.PassNull)
	if catalog, err := testbed.NewCatalog([]testbed.Definition{first, conflicting}); err == nil || catalog != nil {
		t.Fatal("conflicting policy accepted for the same identity")
	}
	for _, invalid := range []testbed.Definition{{}, {Namespace: "fixture", Name: "bad", Metadata: &shared.FunctionMetadata{}}} {
		if catalog, err := testbed.NewCatalog([]testbed.Definition{invalid}); err == nil || catalog != nil {
			t.Fatal("invalid definition accepted")
		}
	}
}
