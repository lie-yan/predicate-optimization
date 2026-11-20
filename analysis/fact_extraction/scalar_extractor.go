// Package fact_extraction derives exact predicate facts from expression trees.
package fact_extraction

import (
	"errors"
	"fmt"
	"math"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/internal/function_admission"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/analysis/value_set"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ScalarExtractor derives exact single-reference summaries under the fixed
// scalar semantics. The caller supplies the same equality context used for IN
// evaluation; comparison calls retain their own contexts. Use NewScalarExtractor.
type ScalarExtractor struct {
	factory         valueinterfaces.ValueFactory
	functionContext functioninterfaces.FunctionContext
}

// NewScalarExtractor retains the factory for set/range construction and shares
// the caller's stable function context. Both arguments must be non-nil.
func NewScalarExtractor(factory valueinterfaces.ValueFactory, functionContext functioninterfaces.FunctionContext) *ScalarExtractor {
	assertions.NotNil(factory)
	assertions.NotNil(functionContext)
	return &ScalarExtractor{factory: factory, functionContext: functionContext}
}

// Extract supports reference/literal comparisons in either argument order
// and reference IN nonempty non-null literal lists, composed with NOT/AND/OR
// over the same operand. Unsupported predicates
// retain their source and Type; invalid input and provider failures return errors.
// Inputs must be valid immutable expressions. Establish validity at entry with
// expressions/utils.ValidateForOptimization, or controlled construction preserving
// graph-wide identity/Type consistency. Extraction does not validate the whole graph,
// evaluate expressions or rewrite them; its semantic support checks still apply.
func (e *ScalarExtractor) Extract(predicate expressions.Expression) (AnalysisResult, error) {
	if e == nil || assertions.IsNil(e.functionContext) || assertions.IsNil(e.factory) {
		panic("fact_extraction: expected constructed ScalarExtractor")
	}
	assertions.NotNil(predicate)
	if _, ok := predicate.Type().(*types.BoolType); !ok {
		return AnalysisResult{}, fmt.Errorf("%w: scalar extraction requires a Boolean predicate", value_set.ErrInvalidInput)
	}
	var summary *facts.PredicateSummary
	var err error
	switch node := predicate.(type) {
	case *expressions.CallExpression:
		summary, err = e.extractCall(node)
	case *expressions.SpecialFormExpression:
		summary, err = e.extractSpecialForm(node)
	case *expressions.LogicalExpression:
		summary, err = e.extractLogical(node)
	default:
		err = fmt.Errorf("%w: predicate form is not supported", value_set.ErrUnsupported)
	}
	if err != nil {
		// A contract/handle failure must not become a benign unsupported result,
		// even when the provider wraps an unsupported cause as well.
		if errors.Is(err, value_set.ErrUnsupported) && !errors.Is(err, functions.ErrContractViolation) &&
			!errors.Is(err, functions.ErrUnknownHandle) && !errors.Is(err, functions.ErrIncompatibleHandle) &&
			!errors.Is(err, functions.ErrEvaluationFailure) {
			return NewUnsupportedAnalysisResult(predicate, err.Error())
		}
		return AnalysisResult{}, err
	}
	return NewExactAnalysisResult(summary), nil
}

// extractCall dispatches admitted calls to their semantic extraction handler.
// Comparisons are currently the only supported call family.
func (e *ScalarExtractor) extractCall(call *expressions.CallExpression) (*facts.PredicateSummary, error) {
	operator, err := function_admission.CheckComparison(call)
	if err != nil {
		return nil, err
	}
	return e.extractComparison(call, operator)
}

// extractSpecialForm dispatches special forms to their extraction handlers.
func (e *ScalarExtractor) extractSpecialForm(node *expressions.SpecialFormExpression) (*facts.PredicateSummary, error) {
	switch node.Form() {
	case foundation.SpecialFormIn:
		return e.extractMembership(node)
	default:
		return nil, fmt.Errorf("%w: special form", value_set.ErrUnsupported)
	}
}

// extractComparison summarizes admitted reference/literal comparisons in either
// argument order, reversing ordered operators when the literal is on the left.
// Equality uses finite inclusion/exclusion sets; ordering uses a range set.
func (e *ScalarExtractor) extractComparison(call *expressions.CallExpression, operator foundation.OperatorType) (*facts.PredicateSummary, error) {
	args := call.Arguments()
	operand, ok := args[0].(*expressions.ReferenceExpression)
	candidate := args[1]
	if !ok {
		operand, ok = args[1].(*expressions.ReferenceExpression)
		candidate = args[0]
		operator = reverseComparison(operator)
	}
	if !ok {
		return nil, fmt.Errorf("%w: comparison requires a reference and literal", value_set.ErrUnsupported)
	}
	literal, err := nonNullCandidate(candidate)
	if err != nil {
		return nil, err
	}
	// Ordered comparisons describe a range of matching non-null values.
	if operator != foundation.Equal && operator != foundation.NotEqual {
		set, err := orderedComparisonSet(e.factory, operand.Type(), operator, literal)
		if err != nil {
			return nil, err
		}
		return comparisonSummary(operand, set, call)
	}
	// Equality includes the matching values; inequality excludes them from the non-null universe.
	entries := equalityEntriesInPlace([]valueinterfaces.Value{literal})
	var set value_set.ValueSet
	if operator == foundation.Equal {
		set, err = finite.NewEquatableValueSet(e.factory, operand.Type(), entries)
	} else {
		set, err = finite.NewExcludedEquatableValueSet(e.factory, operand.Type(), entries)
	}
	if err != nil {
		return nil, err
	}
	return comparisonSummary(operand, set, call)
}

// extractMembership summarizes reference IN a nonempty list of admitted
// literals after checking equality through the configured function context.
// Its inclusion set deduplicates matches and omits NaN, while the summary
// preserves the source predicate and its null-input outcome.
func (e *ScalarExtractor) extractMembership(node *expressions.SpecialFormExpression) (*facts.PredicateSummary, error) {
	count := node.ChildCount()
	if count < 2 {
		return nil, fmt.Errorf("%w: IN requires nonempty candidates", value_set.ErrUnsupported)
	}
	operand, ok := node.ChildAt(0).(*expressions.ReferenceExpression)
	if !ok {
		return nil, fmt.Errorf("%w: IN requires a reference target", value_set.ErrUnsupported)
	}
	candidates := make([]valueinterfaces.Value, count-1)
	for i := 1; i < count; i++ {
		value, err := nonNullCandidate(node.ChildAt(i))
		if err != nil {
			return nil, err
		}
		candidates[i-1] = value
	}
	// SpecialFormExpression construction establishes equal candidate Types
	// apart from outer nullability; admission above requires all to be NonNull.
	if err := function_admission.CheckMembershipEquality(e.functionContext, operand.Type(), candidates[0].Type()); err != nil {
		return nil, err
	}
	set, err := finite.NewEquatableValueSet(e.factory, operand.Type(), equalityEntriesInPlace(candidates))
	if err != nil {
		return nil, err
	}
	return comparisonSummary(operand, set, node)
}

// nonNullCandidate returns a literal's shared Value only when both its payload
// and declared Type are non-null; other candidates return ErrUnsupported
// without being folded or evaluated first.
func nonNullCandidate(expression expressions.Expression) (valueinterfaces.Value, error) {
	literal, ok := expression.(*expressions.LiteralExpression)
	if !ok || literal.Type().Nullability() != foundation.NonNull || literal.Value().IsNull() {
		return nil, fmt.Errorf("%w: candidate must be a non-null literal with non-null Type", value_set.ErrUnsupported)
	}
	return literal.Value(), nil
}

// equalityEntriesInPlace removes NaN because provider equality never matches it;
// mathematical set identity alone would incorrectly include NaN as a true input.
// The caller must own the candidate slice; compaction changes its entries.
// Both callers supply fresh local storage, and finite-set construction copies
// the retained values before handling duplicates and signed-zero normalization.
func equalityEntriesInPlace(candidates []valueinterfaces.Value) []valueinterfaces.Value {
	entries := candidates[:0]
	for _, candidate := range candidates {
		if _, floating := candidate.Type().(*types.FloatType); floating && math.IsNaN(candidate.(valueinterfaces.FloatValue).Float()) {
			continue
		}
		entries = append(entries, candidate)
	}
	return entries
}

// comparisonSummary combines an exact non-null true set with the null outcome
// of an admitted null-propagating comparison or nonempty IN predicate.
// Nullable operands yield Null on null input; non-nullable operands have no
// null-input case. The original source expression and result Type are preserved.
func comparisonSummary(operand *expressions.ReferenceExpression, set value_set.ValueSet, source expressions.Expression) (*facts.PredicateSummary, error) {
	nullTruth := foundation.Optional[foundation.TruthValue]{}
	if operand.Type().Nullability() == foundation.MayNull {
		nullTruth = foundation.Some(foundation.Null)
	}
	return facts.NewPredicateSummary(operand, set, nullTruth, source)
}
