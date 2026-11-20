package functions_test

import (
	"errors"
	"testing"

	opterrors "github.com/lie-yan/predicate-optimization/errors"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

type bindingCountingCause struct{ calls int }

func (e *bindingCountingCause) Error() string {
	e.calls++
	return "counted cause"
}

func TestBindingErrorDefersCauseFormatting(t *testing.T) {
	integer := bindingInt(t, types.Int64, foundation.NonNull)
	nullable := bindingInt(t, types.Int64, foundation.MayNull)
	for _, joined := range []bool{false, true} {
		cause := &bindingCountingCause{}
		var original error = cause
		if joined {
			original = errors.Join(cause, functions.ErrEvaluationFailure)
		}
		result := bindingAdaptationFailure{Type: integer, cause: original}
		metadata := bindingMetadata(t, []types.Type{integer}, result, []functions.NullPolicy{functions.PropagateNull})
		call := bindingMust(functions.NewFunctionSignature([]types.Type{nullable}, integer))
		err := functions.ValidateCallSignature(metadata, call)
		var concrete *bindingCountingCause
		var coded opterrors.CodedError
		if !errors.Is(err, original) || !errors.Is(err, cause) || !errors.Is(err, functions.ErrContractViolation) || !errors.Is(err, functions.ErrUnsupportedLookup) || errors.Is(err, functions.ErrEvaluationFailure) != joined {
			t.Fatal("lost original cause or category")
		}
		if !errors.As(err, &concrete) || concrete != cause || !errors.As(err, &coded) || coded.Code() != opterrors.CodeInvalidCallBinding || cause.calls != 0 {
			t.Fatal("inspection formatted or lost the cause")
		}
		want := "function contract violated: invalid call binding: unsupported function lookup: result nullability adaptation: counted cause"
		if joined {
			want += "\nfunction evaluation failed"
		}
		if err.Error() != want || cause.calls != 1 {
			t.Fatal("unexpected lazy diagnostic")
		}
		_ = err.Error()
		if cause.calls != 2 {
			t.Fatal("diagnostic unexpectedly cached")
		}
	}
}

func TestBindingErrorNonzeroDetails(t *testing.T) {
	integer := bindingInt(t, types.Int64, foundation.NonNull)
	boolean := bindingMust(types.NewBoolType(foundation.NonNull))
	metadata := bindingMetadata(t, []types.Type{integer, integer}, integer, []functions.NullPolicy{functions.PropagateNull, functions.PropagateNull})
	for _, args := range [][]types.Type{{integer, boolean}, {integer, integer, integer}} {
		_, err := functions.BindFunctionSignature(metadata, args)
		var detail *functions.BindingError
		if !errors.As(err, &detail) {
			t.Fatal("missing binding error")
		}
		if len(args) == 2 {
			if index, present := detail.ArgumentIndex(); !present || index != 1 {
				t.Fatal("lost nonzero argument index")
			}
		} else if actual, expected, present := detail.ArgumentCounts(); !present || actual != 3 || expected != 2 {
			t.Fatal("lost arities")
		}
	}
}
