package functions

import (
	"errors"
	"fmt"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	shared "github.com/lie-yan/predicate-optimization/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Evaluator is testbed-specific concrete execution, after checked adaptation.
// It receives implementation argument types and must return the implementation
// result type. Values may use any implementation of the public payload
// interfaces. Values are immutable; evaluation is synchronous.
type Evaluator func([]valueinterfaces.Value) (valueinterfaces.Value, error)

// EvaluationBinding configures execution for one catalog entry. A nil Evaluate
// means unavailable; Disallowed blocks even known-null propagation. Callbacks
// and captured resources must preserve the catalog's semantic contract.
type EvaluationBinding struct {
	Handle     functioninterfaces.FunctionHandle
	Evaluate   Evaluator
	Disallowed bool
}

// NewContextWithEvaluators fixes testbed execution bindings at construction.
// It does not modify the catalog. Duplicate or foreign bindings are errors;
// entries without a binding retain metadata but have no concrete evaluator.
func NewContextWithEvaluators(catalog *Catalog, bindings []EvaluationBinding) (*Context, error) {
	c := NewContext(catalog)
	c.evaluation = make(map[*entry]EvaluationBinding, len(bindings))
	for _, binding := range bindings {
		definition, err := c.resolveEntry(binding.Handle)
		if err != nil {
			return nil, err
		}
		if _, exists := c.evaluation[definition]; exists {
			return nil, fmt.Errorf("%w: duplicate evaluation binding", shared.ErrContractViolation)
		}
		c.evaluation[definition] = binding
	}
	return c, nil
}

// Evaluate validates the effective call contract, applies null policies and
// invokes a private evaluator only when needed. Results retain effective types.
func (c *Context) Evaluate(handle functioninterfaces.FunctionHandle, callSignature *shared.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	assertions.NotNilPointer(callSignature)
	for _, argument := range args {
		assertions.NotNil(argument)
	}
	definition, err := c.resolveEntry(handle)
	if err != nil {
		return nil, err
	}
	argumentTypes := callSignature.ArgumentTypes()
	metadata := definition.metadata
	if err := shared.ValidateCallSignature(metadata, callSignature); err != nil {
		return nil, err
	}
	if len(args) != len(argumentTypes) {
		return nil, fmt.Errorf("%w: incorrect argument count", shared.ErrContractViolation)
	}
	for i, argument := range args {
		if !argument.Type().EqualsTo(argumentTypes[i]) {
			return nil, fmt.Errorf("%w: invalid argument at position %d", shared.ErrContractViolation, i)
		}
	}
	binding := c.evaluation[definition]
	if binding.Disallowed {
		return nil, shared.ErrEvaluationDisallowed
	}
	policies := metadata.NullPolicies()
	for i, argument := range args {
		if argument.IsNull() && policies[i] == shared.PropagateNull {
			return testbedvalues.NewNull(callSignature.ResultType())
		}
	}
	for i, argument := range args {
		if argument.IsNull() && policies[i] == shared.NullPolicyUnspecified {
			return nil, shared.ErrEvaluationDisallowed
		}
	}
	if binding.Evaluate == nil {
		return nil, shared.ErrEvaluationUnavailable
	}
	parameters := metadata.Signature().ArgumentTypes()
	adapted := make([]valueinterfaces.Value, len(args))
	for i, argument := range args {
		adapted[i], err = argument.WithNullability(parameters[i].Nullability())
		if err != nil {
			return nil, fmt.Errorf("%w: argument adaptation: %w", shared.ErrContractViolation, err)
		}
	}
	result, err := binding.Evaluate(adapted)
	if err != nil {
		return nil, classifyEvaluationError(err)
	}
	assertions.NotNil(result)
	if !result.Type().EqualsTo(metadata.Signature().ResultType()) {
		return nil, fmt.Errorf("%w: invalid evaluator result", shared.ErrContractViolation)
	}
	result, err = result.WithNullability(callSignature.ResultType().Nullability())
	if err != nil {
		return nil, fmt.Errorf("%w: result adaptation: %w", shared.ErrContractViolation, err)
	}
	return result, nil
}

func classifyEvaluationError(err error) error {
	for _, category := range []error{
		shared.ErrUnknownHandle, shared.ErrIncompatibleHandle, shared.ErrUnsupportedLookup,
		shared.ErrEvaluationUnavailable, shared.ErrEvaluationDisallowed,
		shared.ErrEvaluationFailure, shared.ErrContractViolation,
	} {
		if errors.Is(err, category) {
			return err
		}
	}
	return fmt.Errorf("%w: %w", shared.ErrEvaluationFailure, err)
}
