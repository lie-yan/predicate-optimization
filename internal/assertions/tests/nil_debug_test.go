//go:build debug

package assertions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

func TestNotNilRejectsNil(t *testing.T) {
	for _, input := range []any{nil, (*pointerFixture)(nil), mapFixture(nil), []int(nil), (func())(nil), (chan int)(nil)} {
		func() {
			defer func() {
				if got := recover(); got != "assertions: expected non-nil input" {
					t.Errorf("unexpected assertion: %v", got)
				}
			}()
			assertions.NotNil(input)
		}()
	}
}

func TestNotNilPointerRejectsNil(t *testing.T) {
	defer func() {
		if got := recover(); got != "assertions: expected non-nil input" {
			t.Fatalf("unexpected assertion: %v", got)
		}
	}()
	assertions.NotNilPointer((*pointerFixture)(nil))
}
