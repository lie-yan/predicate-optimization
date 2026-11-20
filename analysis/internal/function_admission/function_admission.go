// Package function_admission checks scalar operator contracts for analysis consumers.
package function_admission

import (
	"errors"
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/internal/diagnostics"
	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	functionutils "github.com/lie-yan/predicate-optimization/functions/utils"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// CheckComparison inspects the call's own paired context without resolving a
// replacement handle or evaluating arguments. Recognized operators are trusted
// to implement the fixed scalar semantics; admission does not prove rewrite safety.
// The call must already be valid under stable provider metadata. Entry validation
// and checked construction own handle/signature consistency, not this consumer.
func CheckComparison(call *expressions.CallExpression) (foundation.OperatorType, error) {
	assertions.NotNilPointer(call)
	metadata, err := call.Context().Metadata(call.Handle())
	if err != nil {
		return 0, fmt.Errorf("comparison metadata: %w", err)
	}
	return admitComparison(metadata, call.Signature())
}

// CheckMembershipEquality resolves Equal through the caller's membership
// context. Candidate-pattern admission belongs to extraction, not this check.
func CheckMembershipEquality(context functioninterfaces.FunctionContext, left, right types.Type) error {
	assertions.NotNil(context)
	assertions.NotNil(left)
	assertions.NotNil(right)
	if err := checkComparisonDomain(foundation.Equal, []types.Type{left, right}); err != nil {
		return err
	}
	handle, err := context.ComparisonFunction(foundation.Equal, left, right)
	if err != nil {
		// Contract and handle failures take precedence over an unsupported cause.
		if errors.Is(err, functions.ErrUnsupportedLookup) &&
			!errors.Is(err, functions.ErrContractViolation) &&
			!errors.Is(err, functions.ErrUnknownHandle) &&
			!errors.Is(err, functions.ErrIncompatibleHandle) {
			return fmt.Errorf("%w: membership equality lookup: %w", value_set.ErrUnsupported, err)
		}
		return fmt.Errorf("membership equality lookup: %w", err)
	}
	metadata, err := functionutils.InspectFunction(context, handle)
	if err != nil {
		return err
	}
	if operator, present := metadata.Operator().Get(); !present || operator != foundation.Equal {
		return fmt.Errorf("%w: equality lookup returned a different operator", functions.ErrContractViolation)
	}
	if err := functions.CheckFunctionBinding(metadata, []types.Type{left, right}); err != nil {
		return fmt.Errorf("%w: resolved equality cannot bind requested types: %w", functions.ErrContractViolation, err)
	}
	if _, supported := functionutils.RecognizeScalarComparisonPair(metadata, left, right).Get(); supported {
		return nil
	}
	return diagnostics.ComparisonUnsupported()
}

// admitComparison admits standard comparisons over compatible scalar domains
// with a non-null Boolean implementation result and PropagateNull for both
// parameters, so analysis can separate non-null true sets from null outcomes.
// The effective signature must already be bound to the metadata; provider
// semantics are trusted, and unsupported contracts return ErrUnsupported.
func admitComparison(metadata *functions.FunctionMetadata, signature *functions.FunctionSignature) (foundation.OperatorType, error) {
	if operator, supported := functionutils.RecognizeScalarComparison(metadata, signature).Get(); supported {
		return operator, nil
	}
	return 0, diagnostics.ComparisonUnsupported()
}

// Domain recognition also applies before lookup; analysis owns the unsupported
// error category, while the shared predicate only reports applicability.
func checkComparisonDomain(operator foundation.OperatorType, arguments []types.Type) error {
	if !functionutils.IsScalarComparisonDomain(operator, arguments) {
		return fmt.Errorf("%w: scalar comparison domain not supported", value_set.ErrUnsupported)
	}
	return nil
}
