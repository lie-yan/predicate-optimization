package foundation_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
)

func TestOptionalPresence(t *testing.T) {
	var absent foundation.Optional[int]
	if value, ok := absent.Get(); value != 0 || ok {
		t.Fatal("zero-value optional must be absent")
	}
	presentZero := foundation.Some(0)
	if value, ok := presentZero.Get(); value != 0 || !ok {
		t.Fatal("present zero must be distinguishable from absence")
	}
	copy := presentZero
	presentZero = foundation.Some(42)
	if value, ok := copy.Get(); value != 0 || !ok {
		t.Fatal("assigning an optional changed its copy")
	}
	if value, ok := presentZero.Get(); value != 42 || !ok {
		t.Fatal("present value was lost")
	}
	if value, ok := foundation.Some[*int](nil).Get(); value != nil || !ok {
		t.Fatal("present nil must remain present")
	}
}
