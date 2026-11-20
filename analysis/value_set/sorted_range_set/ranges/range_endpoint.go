package ranges

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// RangeEndpoint is an immutable unbounded side or a non-null typed value with
// an inclusion flag. The containing Range determines lower versus upper usage.
// Its zero value is unbounded, not SQL NULL. Copying shares immutable Values.
type RangeEndpoint struct {
	value     foundation.Optional[valueinterfaces.Value]
	inclusive bool
}

// NewRangeEndpoint constructs a bounded endpoint and normalizes outer
// nullability. Values must be valid. Nil Values assert; null Values return value_set.ErrInvalidInput.
// Domain ordering and compatibility are checked by NewRange.
func NewRangeEndpoint(value valueinterfaces.Value, inclusive bool) (RangeEndpoint, error) {
	assertions.NotNil(value)
	if value.IsNull() {
		return RangeEndpoint{}, fmt.Errorf("%w: expected valid non-null endpoint", value_set.ErrInvalidInput)
	}
	if value.Type().Nullability() != foundation.NonNull {
		var err error
		value, err = value.WithNullability(foundation.NonNull)
		if err != nil {
			return RangeEndpoint{}, err
		}
	}
	return RangeEndpoint{value: foundation.Some(value), inclusive: inclusive}, nil
}

func (e RangeEndpoint) Value() foundation.Optional[valueinterfaces.Value] { return e.value }
func (e RangeEndpoint) IsUnbounded() bool                                 { _, present := e.value.Get(); return !present }

// IsInclusive returns false for an unbounded side, where inclusion is irrelevant.
func (e RangeEndpoint) IsInclusive() bool { return e.inclusive }
