package rewriter_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/utils"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

type failingMetadata struct {
	functioninterfaces.FunctionContext
	err error
}

func (c *failingMetadata) Metadata(h functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.FunctionContext.Metadata(h)
}

func TestFailedRebindingPreservesGraph(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	ctx := &failingMetadata{FunctionContext: provider}
	typ := must(types.NewIntType(types.Int64, foundation.NonNull))
	one := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(typ, 1))))
	two := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(typ, 2))))
	h := must(provider.ArithmeticFunction(foundation.Add, []types.Type{typ, typ}))
	root := must(expressions.NewCallExpression(ctx, h, []expressions.Expression{one, one}))
	// Inject a provider failure after constructing the immutable original.
	ctx.err = functions.ErrUnknownHandle
	rule := &recordingRule{replacements: map[expressions.Expression]foundation.Optional[expressions.Expression]{one: foundation.Some[expressions.Expression](two)}}
	got, err := treeFor(rule).Rewrite(root, scope{})
	if got != nil || !errors.Is(err, functions.ErrUnknownHandle) {
		t.Fatalf("wrong rebinding error: %v", err)
	}
	if root.Children()[0] != one || root.Children()[1] != one {
		t.Fatal("original mutated")
	}
	// Restore the paired provider before checking the preserved graph.
	ctx.err = nil
	if err := utils.ValidateForOptimization(root); err != nil {
		t.Fatal(err)
	}
}
