package functions_test

import (
	"errors"
	"testing"

	opterrors "github.com/lie-yan/predicate-optimization/errors"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestCheckBindingPreservesAdaptationFailure(t *testing.T) {
	integer := bindingInt(t, types.Int64, foundation.NonNull)
	nullable := bindingInt(t, types.Int64, foundation.MayNull)
	cause := &bindingCountingCause{}
	result := bindingAdaptationFailure{Type: integer, cause: cause}
	metadata := bindingMetadata(t, []types.Type{integer}, result, []functions.NullPolicy{functions.PropagateNull})
	err := functions.CheckFunctionBinding(metadata, []types.Type{nullable})
	var detail *functions.BindingError
	if !errors.Is(err, cause) || !errors.Is(err, functions.ErrUnsupportedLookup) || !errors.As(err, &detail) || detail.Code() != opterrors.CodeBindingResultNullability {
		t.Fatal("check skipped result adaptation or lost its error")
	}
	if cause.calls != 0 {
		t.Fatal("check eagerly formatted its error")
	}
	if metadata.Signature().ResultType() != result {
		t.Fatal("check changed metadata")
	}
}

func TestCheckBindingRejectsNestedMismatch(t *testing.T) {
	integer := bindingInt(t, types.Int64, foundation.NonNull)
	nullable := bindingInt(t, types.Int64, foundation.MayNull)
	array := bindingMust(types.NewArrayType(integer, foundation.NonNull))
	different := bindingMust(types.NewArrayType(nullable, foundation.NonNull))
	metadata := bindingMetadata(t, []types.Type{array}, integer, []functions.NullPolicy{functions.PropagateNull})
	err := functions.CheckFunctionBinding(metadata, []types.Type{different})
	var detail *functions.BindingError
	if !errors.As(err, &detail) || detail.Code() != opterrors.CodeBindingArgumentTypeMismatch {
		t.Fatal("check accepted nested constraint mismatch")
	}
}
