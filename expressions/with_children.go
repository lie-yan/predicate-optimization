package expressions

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

// unchangedChildren uses node identity so an explicitly supplied equal-but-distinct
// child is retained. Arity validation belongs to each variant. Expression implementations are package-owned pointers.
func unchangedChildren(previous, children []Expression) bool {
	for _, child := range children {
		assertions.NotNil(child)
	}
	if len(children) != len(previous) {
		return false
	}
	unchanged := true
	for i, child := range children {
		if child != previous[i] {
			unchanged = false
		}
	}
	return unchanged
}

func checkRebuiltType(original, rebuilt Expression) (Expression, error) {
	if !rebuilt.Type().EqualsTo(original.Type()) {
		return nil, fmt.Errorf("result type changed from %s to %s", original.Type(), rebuilt.Type())
	}
	return rebuilt, nil
}
