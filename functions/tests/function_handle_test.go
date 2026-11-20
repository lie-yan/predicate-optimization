package functions_test

import (
	"testing"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	"github.com/lie-yan/predicate-optimization/types"
)

// externalHandle demonstrates a caller-owned, non-comparable representation.
// Its domain and definition identify an immutable resolved catalog entry.
// It is only a conformance fixture, not the reusable testbed implementation.
type externalHandle struct {
	domain     string
	definition string
	arguments  []types.Type
}

var _ functioninterfaces.FunctionHandle = externalHandle{}

func (h externalHandle) Namespace() string { return h.domain }
func (h externalHandle) Name() string      { return h.definition }
func (h externalHandle) ArgumentTypes() []types.Type {
	return append([]types.Type(nil), h.arguments...)
}

func (h externalHandle) EqualsTo(other functioninterfaces.FunctionHandle) bool {
	if other == nil {
		panic("expected non-nil FunctionHandle")
	}
	o, ok := other.(externalHandle)
	return ok && h.domain == o.domain && h.definition == o.definition
}

func TestCallerDefinedFunctionHandle(t *testing.T) {
	boolean := &types.BoolType{}
	newHandle := func(domain, definition string) functioninterfaces.FunctionHandle {
		return externalHandle{domain, definition, []types.Type{boolean}}
	}
	a, b, c := newHandle("fixture", "not"), newHandle("fixture", "not"), newHandle("fixture", "not")
	if a.Namespace() != "fixture" || a.Name() != "not" {
		t.Fatal("caller-defined identity was not exposed")
	}
	if !a.EqualsTo(a) || !a.EqualsTo(b) || !b.EqualsTo(a) || !b.EqualsTo(c) || !a.EqualsTo(c) {
		t.Fatal("equivalent independently constructed handles must obey equality laws")
	}
	for _, different := range []functioninterfaces.FunctionHandle{
		newHandle("other fixture", "not"),
		newHandle("fixture", "other definition"),
	} {
		if a.EqualsTo(different) || different.EqualsTo(a) {
			t.Fatal("distinct catalog identities must compare unequal")
		}
	}
	args := a.ArgumentTypes()
	if len(args) != 1 || args[0] != boolean {
		t.Fatal("argument types should share immutable descriptors")
	}
	args[0] = nil
	if a.ArgumentTypes()[0] != boolean {
		t.Fatal("mutating a returned slice changed the handle")
	}
}
