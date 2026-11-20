package boolean_cleanup

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functionutils "github.com/lie-yan/predicate-optimization/functions/utils"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
	"github.com/lie-yan/predicate-optimization/types"
)

// CanOmit checks structural eligibility under the caller's pure, stable, total
// execution and reference-binding contracts. It neither evaluates nor proves truth.
func (r *Cleaner) CanOmit(source expressions.Expression) (bool, error) {
	if r == nil {
		panic("boolean_cleanup: expected non-nil rewriter")
	}
	assertions.NotNil(source)
	switch node := source.(type) {
	case *expressions.LiteralExpression, *expressions.ReferenceExpression:
		return true, nil
	case *expressions.LogicalExpression:
		return r.canOmitLogical(node)
	case *expressions.CallExpression:
		return r.canOmitCall(node)
	case *expressions.SpecialFormExpression:
		return r.canOmitSpecialForm(node)
	default:
		// Opaque expressions and future variants remain unadmitted.
		return false, nil
	}
}

// canOmitLogical admits Boolean composition when every operand is eligible.
func (r *Cleaner) canOmitLogical(node *expressions.LogicalExpression) (bool, error) {
	return r.canOmitChildren(node)
}

// canOmitCall checks the invocation contract before inspecting its arguments.
func (r *Cleaner) canOmitCall(node *expressions.CallExpression) (bool, error) {
	metadata, err := invocationMetadata(node)
	if err != nil {
		return false, err
	}
	if !admitCallInvocation(metadata, node.Signature(), node.ChildAt) {
		return false, nil
	}
	return r.canOmitChildren(node)
}

// admitCallInvocation checks only the enclosing operation, not argument computations.
func admitCallInvocation(metadata *functions.FunctionMetadata, signature *functions.FunctionSignature, childAt func(int) expressions.Expression) bool {
	name, _ := metadata.BuiltInName().Get()
	switch name {
	case functions.BuiltInArrayContains, functions.BuiltInArrayContainsAny:
		return admitArrayMembershipInvocation(metadata, signature, childAt)
	default:
		return admitScalarInvocation(metadata, signature)
	}
}

// Inspect every child of a supported operation so an unsupported sibling does
// not hide a later metadata failure. Shared nodes are inspected per occurrence.
func (r *Cleaner) canOmitChildren(source expressions.Expression) (bool, error) {
	admitted := true
	for index := 0; index < source.ChildCount(); index++ {
		eligible, err := r.CanOmit(source.ChildAt(index))
		if err != nil {
			return false, fmt.Errorf("%T child %d eligibility: %w", source, index, err)
		}
		admitted = admitted && eligible
	}
	return admitted, nil
}

// invocationMetadata reads semantic metadata for a valid stored call. Entry
// validation and checked construction own handle/signature consistency. The
// provider's successful metadata must remain valid and stable; query errors propagate.
func invocationMetadata(call *expressions.CallExpression) (*functions.FunctionMetadata, error) {
	metadata, err := call.Context().Metadata(call.Handle())
	if err != nil {
		return nil, fmt.Errorf("call eligibility metadata: %w", err)
	}
	return metadata, nil
}

// admitScalarInvocation follows the scalar-analysis comparison domains and null
// contract, and additionally recognizes the standard Boolean NOT invocation.
// Inputs must have established handle/metadata agreement and effective binding.
func admitScalarInvocation(metadata *functions.FunctionMetadata, signature *functions.FunctionSignature) bool {
	if _, present := metadata.Operator().Get(); present {
		_, supported := functionutils.RecognizeScalarComparison(metadata, signature).Get()
		return supported
	}
	// Boolean NOT has its own contract and remains local to cleanup.
	result, boolean := metadata.Signature().ResultType().(*types.BoolType)
	if !boolean || result.Nullability() != foundation.NonNull {
		return false
	}
	for i := 0; i < metadata.Signature().ArgumentCount(); i++ {
		if metadata.NullPolicyAt(i) != functions.PropagateNull {
			return false
		}
	}
	name, present := metadata.BuiltInName().Get()
	if !present || name != functions.BuiltInNot || signature.ArgumentCount() != 1 {
		return false
	}
	_, boolean = signature.ArgumentTypeAt(0).(*types.BoolType)
	return boolean
}

// admitArrayMembershipInvocation checks the selected Array<Int64!> built-ins.
// Binding is already validated; only the input array may gain outer nullability.
func admitArrayMembershipInvocation(metadata *functions.FunctionMetadata, signature *functions.FunctionSignature, childAt func(int) expressions.Expression) bool {
	if _, present := metadata.Operator().Get(); present {
		return false
	}
	name, present := metadata.BuiltInName().Get()
	if !present || (name != functions.BuiltInArrayContains && name != functions.BuiltInArrayContainsAny) {
		return false
	}
	implementation := metadata.Signature()
	result, boolean := implementation.ResultType().(*types.BoolType)
	if !boolean || result.Nullability() != foundation.NonNull {
		return false
	}
	if implementation.ArgumentCount() != 2 {
		return false
	}
	first, second := implementation.ArgumentTypeAt(0), implementation.ArgumentTypeAt(1)
	if first.Nullability() != foundation.NonNull || second.Nullability() != foundation.NonNull {
		return false
	}
	array, ok := first.(*types.ArrayType)
	if !ok || !isNonNullInt64(array.Element()) {
		return false
	}
	if metadata.NullPolicyAt(0) != functions.PropagateNull || metadata.NullPolicyAt(1) != functions.NullPolicyUnspecified {
		return false
	}
	if signature.ArgumentTypeAt(1).Nullability() != foundation.NonNull {
		return false
	}
	if name == functions.BuiltInArrayContains {
		return isNonNullInt64(second)
	}
	if !second.EqualsTo(first) {
		return false
	}
	candidate, literal := childAt(1).(*expressions.LiteralExpression)
	return literal && !candidate.Value().IsNull()
}

func isNonNullInt64(typ types.Type) bool {
	integer, ok := typ.(*types.IntType)
	return ok && integer.IntegerWidth() == types.Int64 && integer.Nullability() == foundation.NonNull
}

// canOmitSpecialForm dispatches admission by form; unsupported forms remain unadmitted.
func (r *Cleaner) canOmitSpecialForm(node *expressions.SpecialFormExpression) (bool, error) {
	switch node.Form() {
	case foundation.SpecialFormIn:
		return r.canOmitMembership(node)
	default:
		return false, nil
	}
}

// canOmitMembership admits the original IN shape before inspecting its target
// or provider. Folding cannot turn a dynamic or nullable candidate into admission.
func (r *Cleaner) canOmitMembership(node *expressions.SpecialFormExpression) (bool, error) {
	if !admitMembershipShape(node.ChildCount(), node.ChildAt) {
		return false, nil
	}
	admitted, err := r.CanOmit(node.ChildAt(0))
	if err != nil {
		return false, fmt.Errorf("IN target eligibility: %w", err)
	}
	invocations, err := r.admitMembershipInvocations(node)
	if err != nil {
		return false, err
	}
	return admitted && invocations, nil
}

// admitMembershipShape checks the original or proposed candidate list without folding.
func admitMembershipShape(count int, childAt func(int) expressions.Expression) bool {
	if count < 2 {
		return false
	}
	for i := 1; i < count; i++ {
		literal, ok := childAt(i).(*expressions.LiteralExpression)
		if !ok || literal.Type().Nullability() != foundation.NonNull || literal.Value().IsNull() {
			return false
		}
	}
	return true
}

// Every candidate position requires a valid invocation, even after an
// unsupported invocation or an obvious literal match. Target eligibility is separate.
func (r *Cleaner) admitMembershipInvocations(node *expressions.SpecialFormExpression) (bool, error) {
	admitted := true
	for index := 1; index < node.ChildCount(); index++ {
		eligible, err := r.admitMembershipEquality(node.ChildAt(0).Type(), node.ChildAt(index).Type())
		if err != nil {
			return false, fmt.Errorf("IN candidate %d eligibility: %w", index-1, err)
		}
		admitted = admitted && eligible
	}
	return admitted, nil
}

// admitMembershipEquality resolves Equal through the function context and
// checks the bound invocation without evaluating values or extracting a true set.
func (r *Cleaner) admitMembershipEquality(left, right types.Type) (bool, error) {
	handle, err := r.functionContext.ComparisonFunction(foundation.Equal, left, right)
	if err != nil {
		return false, fmt.Errorf("membership equality lookup: %w", err)
	}
	metadata, err := functionutils.InspectFunction(r.functionContext, handle)
	if err != nil {
		return false, err
	}
	if operator, present := metadata.Operator().Get(); !present || operator != foundation.Equal {
		return false, fmt.Errorf("%w: equality lookup returned a different operator", functions.ErrContractViolation)
	}
	if err := functions.CheckFunctionBinding(metadata, []types.Type{left, right}); err != nil {
		return false, fmt.Errorf("%w: equality cannot bind requested types: %w", functions.ErrContractViolation, err)
	}
	_, supported := functionutils.RecognizeScalarComparisonPair(metadata, left, right).Get()
	return supported, nil
}

// CanChangeChildren admits changed enclosing uses, not the child replacements
// themselves. Callers supply their semantic/omission evidence separately; unchanged
// siblings stay evaluated in their original positions, even if they can fail.
func (r *Cleaner) CanChangeChildren(source expressions.Expression, children []expressions.Expression) (bool, error) {
	if r == nil {
		panic("boolean_cleanup: expected non-nil rewriter")
	}
	assertions.NotNil(source)
	for _, child := range children {
		assertions.NotNil(child)
	}
	if source.ChildCount() != len(children) {
		return false, fmt.Errorf("%w: changed child count", rewriter.ErrInvalidReplacement)
	}
	changed := false
	for i, child := range children {
		original := source.ChildAt(i)
		if !child.Type().EqualsTo(original.Type()) {
			return false, fmt.Errorf("%w: child %d Type changed", rewriter.ErrInvalidReplacement, i)
		}
		changed = changed || child != original
	}
	if !changed {
		return true, nil
	}
	proposedChildAt := func(i int) expressions.Expression { return children[i] }
	switch node := source.(type) {
	case *expressions.LogicalExpression:
		return true, nil
	case *expressions.CallExpression:
		metadata, err := invocationMetadata(node)
		if err != nil {
			return false, err
		}
		// Exact per-position Types preserve the effective signature; both source
		// and proposed expression shapes must meet the invocation restrictions.
		return admitCallInvocation(metadata, node.Signature(), node.ChildAt) &&
			admitCallInvocation(metadata, node.Signature(), proposedChildAt), nil
	case *expressions.SpecialFormExpression:
		if node.Form() != foundation.SpecialFormIn || !admitMembershipShape(node.ChildCount(), node.ChildAt) || !admitMembershipShape(len(children), proposedChildAt) {
			return false, nil
		}
		// Equal Types at each position mean the same resolved invocations apply
		// before and after replacement. The retained target need not be omittable.
		return r.admitMembershipInvocations(node)
	default:
		return false, nil
	}
}
