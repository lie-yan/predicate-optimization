package boolean_cleanup_rewriter_test

import (
	"errors"
	"testing"

	cleanup "github.com/lie-yan/predicate-optimization/concrete_rewriters/boolean_cleanup_rewriter"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

type externalFactory struct{ testbedvalues.ValueFactory }

func (f externalFactory) NewBool(typ *types.BoolType, b bool) (valueinterfaces.Value, error) {
	return fixtures.WrapScalar(must(f.ValueFactory.NewBool(typ, b))), nil
}

func (f externalFactory) NewNull(typ types.Type) (valueinterfaces.Value, error) {
	return fixtures.WrapScalar(must(f.ValueFactory.NewNull(typ))), nil
}

func TestCleanupFactoryAndExternalTruths(t *testing.T) {
	context := must(testbed.NewStandardContext())
	factory := &fixtures.RecordingFactory{Delegate: externalFactory{}}
	tree := rewriter.NewExpressionTreeRewriter[struct{}](cleanup.NewBooleanCleanupRewriter(factory, context))
	oracle := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, n := range []foundation.Nullability{foundation.NonNull, foundation.MayNull} {
		var literals []expressions.Expression
		for _, truth := range []foundation.TruthValue{foundation.True, foundation.False, foundation.Null} {
			if n == foundation.NonNull && truth == foundation.Null {
				continue
			}
			literals = append(literals, must(expressions.NewLiteralExpression(fixtures.WrapScalar(literal(truth, n).Value()))))
		}
		var sources []*expressions.LogicalExpression
		for _, a := range literals {
			sources = append(sources, logical(foundation.Not, a))
			for _, b := range literals {
				sources = append(sources, logical(foundation.And, a, b), logical(foundation.Or, a, b))
			}
		}
		for _, source := range sources {
			factory.Requests = nil
			candidate := must(tree.Rewrite(source, struct{}{}))
			value := candidate.(*expressions.LiteralExpression).Value()
			if _, ok := value.(fixtures.ScalarView); !ok || !value.Type().EqualsTo(source.Type()) {
				t.Fatal("lost factory representation or type")
			}
			// Environment is unused for all-literal expressions.
			expected := must(oracle.Evaluate(source, row{}))
			if !value.EqualsTo(expected) || len(factory.Requests) != 1 || !factory.Requests[0].Type.EqualsTo(source.Type()) {
				t.Fatal("wrong truth or factory request")
			}
			sentinel := errors.New("factory failed")
			factory.Failure = sentinel
			result, err := tree.Rewrite(source, struct{}{})
			if result != nil || !errors.Is(err, sentinel) || !errors.Is(err, rewriter.ErrInvalidReplacement) {
				t.Fatal("factory error became a replacement or refusal")
			}
			factory.Failure = nil
		}
	}
	if !assertions.Enabled {
		return
	}
	for _, invalid := range []valueinterfaces.ValueFactory{nil, (*testbedvalues.ValueFactory)(nil)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("nil factory accepted")
				}
			}()
			cleanup.NewBooleanCleanupRewriter(invalid, context)
		}()
	}
}
