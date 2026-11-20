package diagnostics_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/internal/diagnostics"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	opterrors "github.com/lie-yan/predicate-optimization/errors"
	"github.com/lie-yan/predicate-optimization/functions"
)

func TestFixedDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		get     func() error
		code    opterrors.Code
		message string
	}{
		{diagnostics.ComparisonUnsupported, opterrors.CodeAnalysisComparisonUnsupported, "scalar analysis: unsupported semantics: scalar comparison semantics not supported"},
		{diagnostics.LogicalUnsupported, opterrors.CodeAnalysisLogicalUnsupported, "scalar analysis: unsupported semantics: logical children are unsupported or refer to different operands"},
		{diagnostics.ConstantOnly, opterrors.CodeAnalysisConstantOnly, "scalar analysis: unsupported semantics: constant-only logical expression has no operand domain"},
	} {
		err := tc.get()
		if err != tc.get() || err.Error() != tc.message || errors.Unwrap(err) != nil {
			t.Fatal("fixed diagnostic changed or acquired a cause")
		}
		for _, wrapped := range []error{err, fmt.Errorf("outer: %w", err), errors.Join(functions.ErrContractViolation, err)} {
			var coded opterrors.CodedError
			if !errors.Is(wrapped, value_set.ErrUnsupported) || !errors.As(wrapped, &coded) || coded.Code() != tc.code {
				t.Fatal("lost category or code")
			}
		}
		if errors.Is(err, functions.ErrContractViolation) || !errors.Is(errors.Join(functions.ErrContractViolation, err), functions.ErrContractViolation) {
			t.Fatal("stronger category changed")
		}
		if allocations := testing.AllocsPerRun(100, func() {
			got := tc.get()
			if got != err || got.Error() != tc.message || !errors.Is(got, value_set.ErrUnsupported) {
				panic("unstable diagnostic")
			}
		}); allocations != 0 {
			t.Fatalf("fixed diagnostic access allocated %g times", allocations)
		}
	}
}
