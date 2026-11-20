package equatable_value_set

import (
	"fmt"
	"math"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// EquatableValueSet represents included or excluded non-null Boolean, integer,
// float or VARCHAR values. The zero value is invalid. Constructed sets, stored
// Values and their Types are immutable; excluded sets are relative to Type's universe.
type EquatableValueSet struct {
	value_set.ValueSetMarker
	typ         types.Type
	factory     valueinterfaces.ValueFactory
	elements    valueArray
	isInclusion bool
}

var _ value_set.FiniteValueSet = (*EquatableValueSet)(nil)

// NewEquatableValueSet constructs a Boolean, integer, float or VARCHAR inclusion set. It accepts concrete
// non-null Values even when their declared Types are nullable, and stores a
// NonNull element Type. Expression-level candidate admission remains separate.
// Existing Types/Values must be valid. Nil inputs assert; null or incompatible input returns an error.
// Orderable domains sort and deduplicate in O(n log(n+1)) comparisons. Boolean
// construction uses O(n) equality work and stores at most two Values. The required
// factory is retained for normalization and derived finite sets. Concurrent use
// requires a factory that supports it; factory identity is not set identity.
func NewEquatableValueSet(factory valueinterfaces.ValueFactory, typ types.Type, elements []valueinterfaces.Value) (*EquatableValueSet, error) {
	assertions.NotNil(factory)
	assertions.NotNil(typ)
	switch typ.(type) {
	case *types.BoolType, *types.IntType, *types.FloatType, *types.VarCharType:
	default:
		return nil, fmt.Errorf("%w: element type %s", value_set.ErrUnsupported, typ)
	}
	normalized, err := typ.WithNullability(foundation.NonNull)
	if err != nil {
		return nil, err
	}
	entries, err := newValueArray(factory, normalized, elements)
	if err != nil {
		return nil, err
	}
	return &EquatableValueSet{factory: factory, typ: normalized, elements: entries, isInclusion: true}, nil
}

// Type returns the shared non-null element Type. The receiver must be constructed.
func (s *EquatableValueSet) Type() types.Type { s.assertConstructed(); return s.typ }

// Values enumerates stored entries in ascending order for orderable Types
// (NaN last for floats), and first-occurrence order for Boolean. For an excluded
// set these are exclusions, not all members. The returned slice is independent; immutable Values and their Types are shared. Each Value
// has a non-null Type equal to the set's element Type. Order is not set identity.
func (s *EquatableValueSet) Values() []valueinterfaces.Value {
	s.assertConstructed()
	result := make([]valueinterfaces.Value, len(s.elements.values))
	copy(result, s.elements.values)
	return result
}

// NewExcludedEquatableValueSet constructs the complement of the supplied finite
// entries. Validation and normalization are identical to NewEquatableValueSet.
func NewExcludedEquatableValueSet(factory valueinterfaces.ValueFactory, typ types.Type, elements []valueinterfaces.Value) (*EquatableValueSet, error) {
	set, err := NewEquatableValueSet(factory, typ, elements)
	if err != nil {
		return nil, err
	}
	set.isInclusion = false
	return set, nil
}

// IsInclusion reports whether the set uses an inclusion (true) or exclusion
// (false) representation for its stored Values.
func (s *EquatableValueSet) IsInclusion() bool { s.assertConstructed(); return s.isInclusion }

// Contains checks mathematical membership using normalized set identity. Null
// Values return value_set.ErrInvalidInput; incompatible Types return
// value_set.ErrIncompatibleTypes. Nil input asserts. A nullable-typed non-null payload is
// admitted. This method does not implement SQL IN evaluation.
func (s *EquatableValueSet) Contains(value valueinterfaces.Value) (bool, error) {
	s.assertConstructed()
	normalized, err := normalizeSetElement(s.factory, s.typ, value)
	if err != nil {
		return false, err
	}
	if s.elements.contains(normalized) {
		return s.isInclusion, nil
	}
	return !s.isInclusion, nil
}

// Complement flips polarity in O(1), sharing immutable entries and Type.
func (s *EquatableValueSet) Complement() (value_set.ValueSet, error) {
	s.assertConstructed()
	return &EquatableValueSet{factory: s.factory, typ: s.typ, elements: s.elements, isInclusion: !s.isInclusion}, nil
}

func (s *EquatableValueSet) IsEmpty() (bool, error) {
	s.assertConstructed()
	if s.isInclusion {
		return len(s.elements.values) == 0, nil
	}
	return coversUniverse(s.typ, uint64(len(s.elements.values)))
}

func (s *EquatableValueSet) IsUniversal() (bool, error) {
	s.assertConstructed()
	if !s.isInclusion {
		return len(s.elements.values) == 0, nil
	}
	return coversUniverse(s.typ, uint64(len(s.elements.values)))
}

// normalizeSetElement validates the typed non-null domain before normalizing
// outer nullability and signed zero. Canonical NaN retains its set identity.
func normalizeSetElement(factory valueinterfaces.ValueFactory, typ types.Type, element valueinterfaces.Value) (valueinterfaces.Value, error) {
	assertions.NotNil(element)
	if element.IsNull() {
		return nil, fmt.Errorf("%w: expected valid non-null value", value_set.ErrInvalidInput)
	}
	if !typ.EqualsToIgnoringNullability(element.Type()) {
		return nil, fmt.Errorf("%w: set element", value_set.ErrIncompatibleTypes)
	}
	var err error
	if element.Type().Nullability() != foundation.NonNull {
		element, err = element.WithNullability(foundation.NonNull)
		if err != nil {
			return nil, err
		}
	}
	if floatingType, ok := typ.(*types.FloatType); ok {
		floating := element.(valueinterfaces.FloatValue)
		if floating.Float() == 0 && math.Signbit(floating.Float()) {
			return factory.NewFloat(floatingType, 0)
		}
	}
	return element, nil
}

func (s *EquatableValueSet) assertConstructed() {
	if s == nil || s.typ == nil || s.factory == nil {
		panic("value_set: expected constructed EquatableValueSet")
	}
}
