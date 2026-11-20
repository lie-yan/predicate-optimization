package types

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// An additional implementation exercises extensibility without registration.
type testType struct {
	common
	parameter string
}

func (t *testType) IsOrderable() bool { assertions.NotNilPointer(t); return true }

func (t *testType) WithNullability(n foundation.Nullability) (Type, error) {
	c, err := newCommon(n)
	if err != nil {
		return nil, err
	}
	return &testType{common: c, parameter: t.parameter}, nil
}

func (t *testType) EqualsToIgnoringNullability(other Type) bool {
	return equalsToIgnoringNullabilityImpl(t, other)
}

func (t *testType) EqualsTo(other Type) bool { return equalsToImpl(t, other) }
func (t *testType) equalsToIgnoringNullability(other Type) bool {
	o, ok := other.(*testType)
	return ok && t.parameter == o.parameter
}

func TestAdditionalImplementation(t *testing.T) {
	a := &testType{parameter: "a"}
	b := &testType{parameter: "a"}
	c := &testType{parameter: "b"}
	if !a.EqualsToIgnoringNullability(&testType{common: common{nullability: foundation.NonNull}, parameter: "a"}) || a.EqualsToIgnoringNullability(c) {
		t.Fatal("outer-nullability equality did not dispatch")
	}
	if !a.IsOrderable() || !a.EqualsTo(b) || a.EqualsTo(c) {
		t.Fatal("additional implementation did not dispatch")
	}
	left, err := NewArrayType(a, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewArrayType(b, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	if !left.EqualsTo(right) {
		t.Fatal("composite comparison did not dispatch to child")
	}
	boolean := &BoolType{}
	if a.EqualsTo(boolean) || boolean.EqualsTo(a) {
		t.Fatal("different variants compare equal")
	}
}

func TestRoutineOperationsShareChildren(t *testing.T) {
	child := &testType{parameter: "shared"}
	left, err := NewArrayType(child, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewArrayType(child, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	if !left.EqualsTo(right) {
		t.Fatal("equal arrays differ")
	}
	copied, err := left.WithNullability(foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Nullability() != foundation.MayNull || left.Nullability() != foundation.NonNull {
		t.Fatal("nullability copy changed the original")
	}
}

func (t *testType) String() string { return "Test(" + t.parameter + ")" + t.Nullability().Suffix() }
