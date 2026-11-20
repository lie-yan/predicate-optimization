package validation_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/examples/internal/fixtures"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/array_membership_normalization_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/logical_flattening_rewriter"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	shared "github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Observe context API calls, not internal binder invocations, to verify
// provider pairing and absence of evaluation during optimization.
type validationQueries struct {
	metadata, comparison, builtin, evaluation int
}

type validationRecordingContext struct {
	functioninterfaces.FunctionContext
	queries validationQueries
}

func (c *validationRecordingContext) Metadata(h functioninterfaces.FunctionHandle) (*shared.FunctionMetadata, error) {
	c.queries.metadata++
	return c.FunctionContext.Metadata(h)
}

func (c *validationRecordingContext) ComparisonFunction(op foundation.OperatorType, left, right types.Type) (functioninterfaces.FunctionHandle, error) {
	c.queries.comparison++
	return c.FunctionContext.ComparisonFunction(op, left, right)
}

func (c *validationRecordingContext) LookupBuiltInFunction(name shared.BuiltInFunctionName, args []types.Type) (functioninterfaces.FunctionHandle, error) {
	c.queries.builtin++
	return c.FunctionContext.LookupBuiltInFunction(name, args)
}

func (c *validationRecordingContext) Evaluate(h functioninterfaces.FunctionHandle, signature *shared.FunctionSignature, args []valueinterfaces.Value) (valueinterfaces.Value, error) {
	c.queries.evaluation++
	return c.FunctionContext.Evaluate(h, signature, args)
}

func reportValidationQueries(t *testing.T, context *validationRecordingContext, name string, result expressions.Expression) {
	t.Helper()
	t.Logf("%s queries: metadata=%d comparison=%d builtin=%d evaluation=%d", name, context.queries.metadata, context.queries.comparison, context.queries.builtin, context.queries.evaluation)
	if context.queries.evaluation != 0 {
		t.Fatal("optimization evaluated a function")
	}
	if result != nil {
		t.Logf("%s output (%s):\n%s", name, result.Type(), expressions.PrettyPrint(result))
	}
	context.queries = validationQueries{}
}

// Instrument calls with the same underlying catalog and handle identities.
// Fixture setup and this reconstruction are outside every measured operation.
func instrumentValidationCalls(source expressions.Expression, context *validationRecordingContext) expressions.Expression {
	children := source.Children()
	for i, child := range children {
		children[i] = instrumentValidationCalls(child, context)
	}
	if call, ok := source.(*expressions.CallExpression); ok {
		return fixtures.Must(expressions.NewCallExpression(context, call.Handle(), children))
	}
	return fixtures.Must(source.WithChildren(children))
}

func validationPipeline(context functioninterfaces.FunctionContext) []rewriter.ExpressionTreeRewriter[struct{}] {
	return []rewriter.ExpressionTreeRewriter[struct{}]{
		rewriter.NewExpressionTreeRewriter[struct{}](scoped_contextual_rewriter.NewScopedContextualRewriter(testbedvalues.ValueFactory{}, context)),
		rewriter.NewExpressionTreeRewriter[struct{}](boolean_cleanup_rewriter.NewBooleanCleanupRewriter(testbedvalues.ValueFactory{}, context)),
		rewriter.NewExpressionTreeRewriter[struct{}](logical_flattening_rewriter.NewLogicalFlatteningRewriter()),
		rewriter.NewExpressionTreeRewriter[struct{}](array_membership_normalization_rewriter.NewArrayMembershipNormalizationRewriter()),
	}
}
