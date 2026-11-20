package interpreter_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

type binding struct {
	id    interfaces.ColumnID
	value valueinterfaces.Value
}
type environment struct {
	bindings []binding
	err      error
	calls    int
}

func (e *environment) Lookup(id interfaces.ColumnID) (foundation.Optional[valueinterfaces.Value], error) {
	e.calls++
	if e.err != nil {
		return foundation.Optional[valueinterfaces.Value]{}, e.err
	}
	for _, b := range e.bindings {
		if id.EqualsTo(b.id) {
			return foundation.Some(b.value), nil
		}
	}
	return foundation.Optional[valueinterfaces.Value]{}, nil
}

func TestInputResolution(t *testing.T) {
	nullable := must(types.NewIntType(types.Int64, foundation.MayNull))
	nonnull := must(types.NewIntType(types.Int64, foundation.NonNull))
	a := must(ids.NewColumnID("a", "x"))
	b := must(ids.NewColumnID("b", "x"))
	knownNull := must(testbedvalues.NewNull(nullable))
	env := &environment{bindings: []binding{{a, knownNull}}}
	ref := must(expressions.NewReferenceExpression(a, nullable, "same display"))
	other := must(expressions.NewReferenceExpression(b, nonnull, "same display"))
	got, present := must(interpreter.ResolveReference(env, ref)).Get()
	if !present || got != knownNull {
		t.Fatal("known null lost")
	}
	if _, present := must(interpreter.ResolveReference(env, other)).Get(); present {
		t.Fatal("unavailable non-null input became known")
	}
	renamed := must(expressions.NewReferenceExpression(must(ids.NewColumnID("a", "x")), nullable, "renamed"))
	if got, _ := must(interpreter.ResolveReference(env, renamed)).Get(); got != knownNull {
		t.Fatal("display name or pointer used as identity")
	}
	wrong := must(expressions.NewReferenceExpression(a, nonnull, "x"))
	if _, err := interpreter.ResolveReference(env, wrong); !errors.Is(err, interpreter.ErrInvalidBinding) {
		t.Fatal(err)
	}
	for _, value := range []valueinterfaces.Value{must(testbedvalues.NewInteger(nonnull, 1))} {
		env.bindings[0].value = value
		if _, err := interpreter.ResolveReference(env, ref); !errors.Is(err, interpreter.ErrInvalidBinding) {
			t.Fatalf("invalid binding accepted: %v", err)
		}
	}
	sentinel := errors.New("provider failure")
	env.err = sentinel
	if _, err := interpreter.ResolveReference(env, ref); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}

func TestResultAlternativesAndTypes(t *testing.T) {
	typ := must(types.NewBoolType(foundation.MayNull))
	null := must(testbedvalues.NewNull(typ))
	ref := must(expressions.NewReferenceExpression(must(ids.NewColumnID("t", "p")), typ, "p"))
	known := must(interpreter.NewKnownResult(typ, null))
	residual := must(interpreter.NewResidualResult(typ, ref))
	if !known.IsValid() || !residual.IsValid() || !known.Type().EqualsTo(typ) || !residual.Type().EqualsTo(typ) {
		t.Fatal("result type lost")
	}
	if v, ok := known.Known(); !ok || v != null {
		t.Fatal("known NULL lost")
	}
	if _, ok := known.Residual(); ok {
		t.Fatal("known result also residual")
	}
	if e, ok := residual.Residual(); !ok || e != ref {
		t.Fatal("residual lost")
	}
	if _, ok := residual.Known(); ok {
		t.Fatal("residual result also known")
	}
	var unset interpreter.Result
	if unset.IsValid() {
		t.Fatal("zero result valid")
	}
	nonnull := must(types.NewBoolType(foundation.NonNull))
	value := must(testbedvalues.NewBool(nonnull, true))
	if _, err := interpreter.NewKnownResult(typ, value); !errors.Is(err, interpreter.ErrInvalidResult) {
		t.Fatal("known nullability silently adapted")
	}
	if _, err := interpreter.NewResidualResult(nonnull, ref); !errors.Is(err, interpreter.ErrInvalidResult) {
		t.Fatal("residual nullability changed")
	}
}

// A caller-owned ID may contain a slice and cannot be used as a Go map key.
type pathID struct{ parts []string }

func (id pathID) String() string                          { return "diagnostic" }
func (id pathID) Key() string                             { return fmt.Sprintf("interpreter-tests/column:%q", id.parts) }
func (id pathID) EqualsTo(other interfaces.ColumnID) bool { return id.Key() == other.Key() }

func TestNonComparableColumnIdentity(t *testing.T) {
	typ := must(types.NewBoolType(foundation.NonNull))
	value := must(testbedvalues.NewBool(typ, true))
	env := &environment{bindings: []binding{{pathID{[]string{"table", "x"}}, value}}}
	ref := must(expressions.NewReferenceExpression(pathID{[]string{"table", "x"}}, typ, "x"))
	if got, _ := must(interpreter.ResolveReference(env, ref)).Get(); got != value {
		t.Fatal("non-comparable identity not resolved")
	}
}

func expectNilAssertion(t *testing.T, call func()) {
	t.Helper()
	if !assertions.Enabled {
		return
	}
	defer func() {
		if recover() == nil {
			t.Error("expected non-nil assertion")
		}
	}()
	call()
}

func TestNilBindingAssertions(t *testing.T) {
	typ := must(types.NewBoolType(foundation.NonNull))
	id := must(ids.NewColumnID("table", "x"))
	ref := must(expressions.NewReferenceExpression(id, typ, "x"))
	evaluator := interpreter.NewExpressionInterpreter(testbedvalues.ValueFactory{})
	for _, value := range []valueinterfaces.Value{nil, (*testbedvalues.BoolValue)(nil)} {
		env := &environment{bindings: []binding{{id, value}}}
		expectNilAssertion(t, func() { _, _ = interpreter.ResolveReference(env, ref) })
		expectNilAssertion(t, func() { _, _ = evaluator.Evaluate(ref, env) })
		expectNilAssertion(t, func() { _, _ = evaluator.PartialEvaluate(ref, env) })
	}
}
