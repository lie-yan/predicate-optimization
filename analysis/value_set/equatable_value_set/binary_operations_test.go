package equatable_value_set

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"

	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Embedding supplies the private marker without adding a supported concrete
// representation. Dispatch must return an error without calling inherited methods.
type unsupportedSet struct {
	value_set.ValueSet
	typ types.Type
}

func (s *unsupportedSet) Type() types.Type { return s.typ }

func (*unsupportedSet) String() string { return "<unsupportedSet>" }

func TestBinaryOperationsRejectUnsupportedRepresentation(t *testing.T) {
	typ, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	known, err := NewEquatableValueSet(testbedvalues.ValueFactory{}, typ, nil)
	if err != nil {
		t.Fatal(err)
	}
	unknown := &unsupportedSet{typ: typ}
	for _, operation := range []func(value_set.ValueSet) (value_set.ValueSet, error){known.Union, known.Intersect, known.Difference} {
		result, err := operation(unknown)
		if result != nil || !errors.Is(err, value_set.ErrUnsupported) {
			t.Fatalf("unexpected unsupported operand result: %v, %v", result, err)
		}
	}
	for _, query := range []func(value_set.ValueSet) (bool, error){known.IsSubsetOf, known.IsDisjointFrom, known.EqualsTo} {
		if _, err := query(unknown); !errors.Is(err, value_set.ErrUnsupported) {
			t.Fatal("query swallowed unsupported error", err)
		}
	}
}
