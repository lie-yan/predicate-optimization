package types_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestConstructorNilChildAssertions(t *testing.T) {
	for _, child := range []types.Type{nil, (*types.IntType)(nil)} {
		for _, call := range []func(){
			func() { _, _ = types.NewArrayType(child, foundation.NonNull) },
			func() { _, _ = types.NewStructType([]types.StructField{{Name: "x", Type: child}}, foundation.NonNull) },
		} {
			func() {
				if !assertions.Enabled {
					return
				}
				defer func() {
					if recover() == nil {
						t.Error("expected non-nil assertion")
					}
				}()
				call()
			}()
		}
	}
}
