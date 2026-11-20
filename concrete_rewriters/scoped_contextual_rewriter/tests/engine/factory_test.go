package engine_test

import (
	"errors"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/rewriter"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
	fixtures "github.com/lie-yan/predicate-optimization/values/interfaces/tests/testutil"
)

type proofFactory struct {
	*fixtures.RecordingFactory
	boolCalls int
	failAt    int
	failure   error
}

func (f *proofFactory) NewBool(typ *types.BoolType, b bool) (valueinterfaces.Value, error) {
	f.boolCalls++
	if f.boolCalls == f.failAt {
		return nil, f.failure
	}
	value, err := f.RecordingFactory.NewBool(typ, b)
	if err != nil {
		return nil, err
	}
	return fixtures.WrapScalar(value), nil
}

func TestScopedProofFactoryForwarding(t *testing.T) {
	provider := membershipEligibilityProvider()
	factory := &proofFactory{RecordingFactory: &fixtures.RecordingFactory{Delegate: testbedvalues.ValueFactory{}}, failure: errors.New("proof factory failed")}
	rule := engine.New(factory, provider)
	x := guardReference("x", must(types.NewIntType(types.Int64, foundation.NonNull)))
	one := must(expressions.NewLiteralExpression(fixtures.WrapScalar(guardInteger(1).Value())))
	guard := membershipExpression(x, one, guardInteger(2))
	target := guardComparison(provider, foundation.Equal, x, 3)
	source := guardLogical(foundation.And, guard, target)
	attempt := requireAttempt(t, rule, source, must(rule.SelectGuards(source))[0])
	context := engine.RewriteContext{Attempt: attempt, Path: engine.OccurrencePath{1}}
	result, present := must(attempt.ProveLiteral(target, context)).Get()
	if !present || factory.boolCalls != 2 {
		t.Fatalf("proof construction/validation bypassed factory: %d", factory.boolCalls)
	}
	value := result.Expression.(*expressions.LiteralExpression).Value()
	if _, ok := value.(fixtures.ScalarView); !ok || value.(valueinterfaces.BoolValue).Bool() || !value.Type().EqualsTo(target.Type()) {
		t.Fatal("lost external proof result")
	}
	factory.boolCalls = 0
	factory.failAt = 2
	failed, err := attempt.ProveLiteral(target, context)
	if _, present := failed.Get(); present || !errors.Is(err, factory.failure) || !errors.Is(err, rewriter.ErrInvalidReplacement) {
		t.Fatal("proof validation concealed factory failure")
	}
	factory.failAt = 0
	// A strict integer guard synthesizes bounds during extraction, and a
	// subsequent complement proves that the derived input set retained the factory.
	factory.Requests = nil
	rangeGuard := guardComparison(provider, foundation.LessThan, x, 10)
	rangeSource := guardLogical(foundation.And, rangeGuard, target)
	ranged := requireAttempt(t, rule, rangeSource, must(rule.SelectGuards(rangeSource))[0])
	if len(factory.Requests) == 0 {
		t.Fatal("engine extractor lost factory")
	}
	factory.Requests = nil
	must(ranged.Input.PossibleValues().Complement())
	if len(factory.Requests) == 0 {
		t.Fatal("scoped input domain lost factory")
	}
	factory.Failure = factory.failure
	if _, err := rule.SelectGuards(rangeSource); !errors.Is(err, factory.failure) {
		t.Fatal("engine hid extraction factory failure")
	}
}
