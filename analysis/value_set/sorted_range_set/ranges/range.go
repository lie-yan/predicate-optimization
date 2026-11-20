package ranges

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Range is an immutable nonempty interval over one non-null domain. Use NewRange;
// the zero struct is invalid. Endpoints and their Values may be shared.
type Range struct {
	typ   types.Type
	lower RangeEndpoint
	upper RangeEndpoint
}

// NewRange admits integer, float and bytewise VARCHAR domains. Open integer bounds become
// closed successor/predecessor bounds; float and VARCHAR preserve open/closed cuts.
// Closed domain extrema become unbounded. Float ranges exclude NaN and reject
// NaN endpoints; a fully unbounded float range covers only non-NaN values.
// An absent result without error means an empty interval, including reversed
// bounds. The required factory constructs normalized payloads and is not retained.
// Both endpoints are checked before any emptiness shortcut. Nil factories/Types
// assert; Types/Values must be valid. Null/incompatible input and unsupported domains return errors.
func NewRange(factory valueinterfaces.ValueFactory, typ types.Type, lower, upper RangeEndpoint) (foundation.Optional[*Range], error) {
	empty := foundation.Optional[*Range]{}
	assertions.NotNil(factory)
	assertions.NotNil(typ)
	if !typ.IsOrderable() {
		return empty, fmt.Errorf("%w: range type is not orderable", value_set.ErrUnsupported)
	}
	switch typ.(type) {
	case *types.IntType, *types.FloatType, *types.VarCharType:
	default:
		return empty, fmt.Errorf("%w: range domain %s", value_set.ErrUnsupported, typ)
	}
	for _, endpoint := range []RangeEndpoint{lower, upper} {
		if value, present := endpoint.Value().Get(); present {
			if value.IsNull() {
				return empty, fmt.Errorf("%w: malformed range endpoint", value_set.ErrInvalidInput)
			}
			if !typ.EqualsToIgnoringNullability(value.Type()) {
				return empty, fmt.Errorf("%w: range endpoint", value_set.ErrIncompatibleTypes)
			}
		}
	}
	normalized, err := typ.WithNullability(foundation.NonNull)
	if err != nil {
		return empty, err
	}
	switch domain := normalized.(type) {
	case *types.IntType:
		return newIntegerRange(factory, domain, lower, upper)
	case *types.VarCharType:
		return newVarCharRange(domain, lower, upper)
	case *types.FloatType:
		return newFloatRange(factory, domain, lower, upper)
	default:
		return empty, fmt.Errorf("%w: range domain %s", value_set.ErrUnsupported, normalized)
	}
}

func (r *Range) Type() types.Type     { r.assertConstructed(); return r.typ }
func (r *Range) Lower() RangeEndpoint { r.assertConstructed(); return r.lower }
func (r *Range) Upper() RangeEndpoint { r.assertConstructed(); return r.upper }
func (r *Range) assertConstructed() {
	if r == nil || r.typ == nil {
		panic("value_set: expected constructed Range")
	}
}
