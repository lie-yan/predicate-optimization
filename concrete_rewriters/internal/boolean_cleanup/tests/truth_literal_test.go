package boolean_cleanup_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestTruthLiteralPreservesType(t *testing.T) {
	for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		typ := must(types.NewBoolType(nullability))
		for _, truth := range []foundation.TruthValue{foundation.False, foundation.True, foundation.Null} {
			literal, err := boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, typ, truth)
			if truth == foundation.Null && nullability == foundation.NonNull {
				if literal != nil || !errors.Is(err, rewriter.ErrInvalidReplacement) {
					t.Fatalf("NULL under Bool!: got %v, %v", literal, err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("truth %d under %s: %v", truth, typ, err)
			}
			// Preserve the shared declaration itself, not just an equivalent Type.
			if literal.Type() != typ || literal.Value().Type() != typ || typ.Nullability() != nullability {
				t.Fatalf("truth %d under %s changed the declared Type", truth, typ)
			}
			if truth == foundation.Null {
				if !literal.Value().IsNull() {
					t.Fatal("NULL truth did not become a known null literal")
				}
			} else {
				value, ok := literal.Value().(*testbedvalues.BoolValue)
				if !ok || value.Bool() != (truth == foundation.True) {
					t.Fatalf("truth %d has incorrect literal payload", truth)
				}
			}
		}
	}
}

func TestTruthLiteralRejectsInvalidTruth(t *testing.T) {
	for _, nullability := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		typ := must(types.NewBoolType(nullability))
		for _, truth := range []foundation.TruthValue{3, 255} {
			literal, err := boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, typ, truth)
			if literal != nil || !errors.Is(err, rewriter.ErrInvalidReplacement) {
				t.Fatalf("invalid truth %d under %s: got %v, %v", truth, typ, literal, err)
			}
		}
	}
}

func TestTruthLiteralNilType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("nil Type did not assert")
		}
	}()
	_, _ = boolean_cleanup.TruthLiteral(testbedvalues.ValueFactory{}, nil, foundation.True)
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
