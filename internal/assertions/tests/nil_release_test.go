//go:build !debug

package assertions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

func TestAssertionsDisabled(t *testing.T) {
	assertions.NotNil(nil)
	assertions.NotNil((*pointerFixture)(nil))
	assertions.NotNil(mapFixture(nil))
	assertions.NotNilPointer((*pointerFixture)(nil))
}
