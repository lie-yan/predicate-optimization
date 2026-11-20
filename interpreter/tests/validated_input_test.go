package interpreter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/folding_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	expressionutils "github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/interpreter/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

// Counts consumer queries separately from the testbed's internal evaluation work.
type inspectedProvider struct {
	*callProvider
	metadataQueries int
	equalityLookups int
}

func (p *inspectedProvider) Metadata(handle functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	p.metadataQueries++
	return p.FunctionContext.Metadata(handle)
}

func (p *inspectedProvider) ComparisonFunction(operator foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
	p.equalityLookups++
	return p.FunctionContext.ComparisonFunction(operator, left, right)
}

func TestValidatedCallTrustsStoredSignatureAndChecksRuntimeResult(t *testing.T) {
	provider := &inspectedProvider{callProvider: &callProvider{FunctionContext: must(testbed.NewStandardContext())}}
	one := intLiteral(1, foundation.NonNull)
	call := arithmetic(provider, foundation.Add, one, one)
	if err := expressionutils.ValidateForOptimization(call); err != nil {
		t.Fatal(err)
	}
	provider.metadataQueries = 0
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	tree := rewriter.NewExpressionTreeRewriter[interfaces.Environment](folding_rewriter.NewFoldingRewriter(evaluator, true))
	for _, evaluate := range []struct {
		name string
		run  func() (expressions.Expression, error)
	}{
		{"interpreter", func() (expressions.Expression, error) {
			result, err := evaluator.PartialEvaluate(call, &environment{})
			if err != nil {
				return nil, err
			}
			if value, known := result.Known(); known {
				return expressions.NewLiteralExpression(value)
			}
			residual, _ := result.Residual()
			return residual, nil
		}},
		{"folding", func() (expressions.Expression, error) { return tree.Rewrite(call, &environment{}) }},
	} {
		t.Run(evaluate.name, func(t *testing.T) {
			provider.override = true
			provider.result = intLiteral(2, foundation.NonNull).Value()
			provider.err = nil
			got := must(evaluate.run())
			if !got.(*expressions.LiteralExpression).Value().EqualsTo(provider.result) {
				t.Fatal("incorrect folded value")
			}
			// Input validity cannot establish a future execution result's Type.
			provider.result = intLiteral(2, foundation.MayNull).Value()
			if got, err := evaluate.run(); got != nil || !errors.Is(err, functions.ErrContractViolation) {
				t.Fatal("invalid runtime result was accepted or preserved", err)
			}
			provider.result = nil
			provider.err = functions.ErrEvaluationUnavailable
			if got := must(evaluate.run()); got != call {
				t.Fatal("unavailable valid call was not retained")
			}
			if provider.metadataQueries != 0 {
				t.Fatal("stored call was reinspected", provider.metadataQueries)
			}
		})
	}
}

func TestValidatedMembershipResolvesEqualityAtRuntime(t *testing.T) {
	provider := &inspectedProvider{callProvider: &callProvider{FunctionContext: must(testbed.NewStandardContext())}}
	one := intLiteral(1, foundation.NonNull)
	node := membership(one, one)
	if err := expressionutils.ValidateForOptimization(node); err != nil {
		t.Fatal(err)
	}
	if provider.metadataQueries != 0 || provider.equalityLookups != 0 {
		t.Fatal("graph validation unexpectedly resolved runtime equality")
	}
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, provider)
	result := must(evaluator.PartialEvaluate(node, &environment{}))
	value, known := result.Known()
	if !known || !value.(*testbedvalues.BoolValue).Bool() || !value.Type().EqualsTo(node.Type()) {
		t.Fatal("membership did not evaluate to true")
	}
	if provider.equalityLookups != 1 || provider.metadataQueries != 1 || provider.calls != 1 {
		t.Fatal("new equality invocation did not retain its runtime boundary", provider.equalityLookups, provider.metadataQueries, provider.calls)
	}
}
