package function_admission

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	functionutils "github.com/lie-yan/predicate-optimization/functions/utils"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// ResolveComparison resolves and constructs a standard comparison in the target
// context. It verifies the requested operator, handle/metadata agreement and
// scalar null contract before returning the call. Checked construction is the sole
// binding authority; a constructed but unadmitted candidate is discarded. No
// arguments are evaluated.
// Lookup and provider errors remain errors, including an unsupported lookup.
func ResolveComparison(context functioninterfaces.FunctionContext, operator foundation.OperatorType, left, right expressions.Expression) (*expressions.CallExpression, error) {
	assertions.NotNil(context)
	assertions.NotNil(left)
	assertions.NotNil(right)
	argumentTypes := []types.Type{left.Type(), right.Type()}
	if !operator.IsComparisonOperator() {
		return nil, fmt.Errorf("%w: expected comparison operator", functions.ErrContractViolation)
	}
	if err := checkComparisonDomain(operator, argumentTypes); err != nil {
		return nil, err
	}
	handle, err := context.ComparisonFunction(operator, left.Type(), right.Type())
	if err != nil {
		return nil, fmt.Errorf("comparison lookup: %w", err)
	}
	metadata, err := functionutils.InspectFunction(context, handle)
	if err != nil {
		return nil, err
	}
	if resolved, present := metadata.Operator().Get(); !present || resolved != operator {
		return nil, fmt.Errorf("%w: comparison lookup returned a different operator", functions.ErrContractViolation)
	}
	candidate, err := expressions.NewCallExpression(context, handle, []expressions.Expression{left, right})
	if err != nil {
		return nil, fmt.Errorf("resolved comparison construction: %w", err)
	}
	if _, err := admitComparison(metadata, candidate.Signature()); err != nil {
		return nil, err
	}
	return candidate, nil
}
