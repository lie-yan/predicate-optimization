package engine_test

import (
	"fmt"
	"testing"

	"github.com/lie-yan/predicate-optimization/concrete_rewriters/scoped_contextual_rewriter/internal/engine"
	"github.com/lie-yan/predicate-optimization/expressions"

	expressioninterfaces "github.com/lie-yan/predicate-optimization/expressions/interfaces"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"

	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
)

func guardLogical(op foundation.LogicalOperator, children ...expressions.Expression) *expressions.LogicalExpression {
	return must(expressions.NewLogicalExpression(op, children))
}

// Slice-backed IDs establish that neither discovery nor screening needs Go map keys.
type guardColumnID []string

func (id guardColumnID) Key() string { return fmt.Sprintf("guard-tests/column:%q", []string(id)) }
func (id guardColumnID) EqualsTo(other expressioninterfaces.ColumnID) bool {
	return id.Key() == other.Key()
}
func (guardColumnID) String() string { return "same diagnostic name" }

func guardReference(name string, typ types.Type) *expressions.ReferenceExpression {
	return must(expressions.NewReferenceExpression(guardColumnID{"table", name}, typ, name))
}

func TestConjunctOccurrenceDiscovery(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	g, a, p := guardReference("g", boolean), guardReference("a", boolean), guardReference("p", boolean)
	disjunction := guardLogical(foundation.Or, g, a)
	negation := guardLogical(foundation.Not, guardLogical(foundation.And, g, p))
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "hidden")), boolean, "g AND p"))
	for _, tc := range []struct {
		name   string
		source expressions.Expression
		paths  []engine.OccurrencePath
		nodes  []expressions.Expression
	}{
		{"left nested", guardLogical(foundation.And, guardLogical(foundation.And, g, a), p), []engine.OccurrencePath{{0, 0}, {0, 1}, {1}}, []expressions.Expression{g, a, p}},
		{"right nested", guardLogical(foundation.And, a, guardLogical(foundation.And, g, p)), []engine.OccurrencePath{{0}, {1, 0}, {1, 1}}, []expressions.Expression{a, g, p}},
		{"OR NOT opaque boundaries", guardLogical(foundation.And, disjunction, negation, opaque), []engine.OccurrencePath{{0}, {1}, {2}}, []expressions.Expression{disjunction, negation, opaque}},
		{"shared node", guardLogical(foundation.And, g, g), []engine.OccurrencePath{{0}, {1}}, []expressions.Expression{g, g}},
		{"non AND root", disjunction, []engine.OccurrencePath{nil}, []expressions.Expression{disjunction}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := expressions.PrettyPrint(tc.source)
			got := engine.EnumerateConjuncts(tc.source)
			if len(got) != len(tc.nodes) {
				t.Fatalf("got %d occurrences, want %d", len(got), len(tc.nodes))
			}
			for i, occurrence := range got {
				if !occurrence.Path.EqualsTo(tc.paths[i]) || occurrence.Expression != tc.nodes[i] || must(occurrence.Path.Resolve(tc.source)) != tc.nodes[i] {
					t.Fatalf("occurrence %d has wrong path or source: %v", i, occurrence.Path)
				}
			}
			// Every retained path owns its storage, including shared-prefix siblings.
			if len(got[0].Path) > 0 {
				got[0].Path[0] = 99
				for i := 1; i < len(got); i++ {
					if !got[i].Path.EqualsTo(tc.paths[i]) {
						t.Fatal("sibling paths alias")
					}
				}
				again := engine.EnumerateConjuncts(tc.source)
				if !again[0].Path.EqualsTo(tc.paths[0]) {
					t.Fatal("paths leak between scans")
				}
			}
			if expressions.PrettyPrint(tc.source) != before {
				t.Fatal("discovery mutated source")
			}
		})
	}
}

func TestPotentialTargetReferenceIdentity(t *testing.T) {
	integer := must(types.NewIntType(types.Int64, foundation.NonNull))
	boolean := must(types.NewBoolType(foundation.NonNull))
	x := guardReference("x", integer)
	same := must(expressions.NewReferenceExpression(guardColumnID{"table", "x"}, must(types.NewIntType(types.Int64, foundation.NonNull)), "different display name"))
	one := must(expressions.NewLiteralExpression(must(testbedvalues.NewInteger(integer, 1))))
	in := func(ref *expressions.ReferenceExpression) expressions.Expression {
		result := must(types.NewBoolType(ref.Type().Nullability()))
		return must(expressions.NewSpecialFormExpression(foundation.SpecialFormIn, result, []expressions.Expression{ref, one}))
	}
	guard := in(x)
	opaque := must(expressions.NewOpaqueExpression(must(ids.NewOpaqueID("fixture", "x")), boolean, "x"))
	for _, tc := range []struct {
		name   string
		target expressions.Expression
		want   bool
	}{
		{"equal identity and full type", in(same), true},
		{"different column same diagnostics", in(guardReference("y", integer)), false},
		{"different nullability", guardReference("x", must(types.NewIntType(types.Int64, foundation.MayNull))), false},
		{"different width", guardReference("x", must(types.NewIntType(types.Int32, foundation.NonNull))), false},
		{"OR and NOT expose targets", guardLogical(foundation.Not, guardLogical(foundation.Or, in(same), in(guardReference("y", integer)))), true},
		{"shared guard object at another path", guard, true},
		{"opaque diagnostic is not contents", opaque, false},
		{"literal", one, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The screen consumes occurrences rather than validating or admitting predicates.
			conjuncts := []engine.ConjunctOccurrence{{Path: engine.OccurrencePath{0}, Expression: guard}, {Path: engine.OccurrencePath{1}, Expression: tc.target}}
			if got := engine.HasPotentialTarget(conjuncts, engine.OccurrencePath{0}, x); got != tc.want {
				t.Fatalf("screen = %v, want %v", got, tc.want)
			}
		})
	}
	// The whole selected OR subtree is excluded, including both references to x.
	wholeGuard := guardLogical(foundation.Or, guard, guard)
	source := guardLogical(foundation.And, wholeGuard, in(guardReference("y", integer)))
	if engine.HasPotentialTarget(engine.EnumerateConjuncts(source), engine.OccurrencePath{0}, x) {
		t.Fatal("guard's own references counted as targets")
	}
	if engine.HasPotentialTarget(engine.EnumerateConjuncts(wholeGuard), nil, x) {
		t.Fatal("sole candidate counted itself")
	}
}

func TestPotentialTargetScansCallsWithoutProviderQueries(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	x := guardReference("x", boolean)
	metadata := eligibilityMetadata([]types.Type{boolean}, boolean, foundation.Optional[foundation.OperatorType]{}, foundation.Optional[functions.BuiltInFunctionName]{}, []functions.NullPolicy{functions.PropagateNull})
	context, call := eligibilityCall(metadata, []expressions.Expression{x})
	context.queries = 0
	context.metadata = func(functioninterfaces.FunctionHandle) (*functions.FunctionMetadata, error) {
		panic("discovery must not query metadata")
	}
	source := guardLogical(foundation.And, x, call)
	if !engine.HasPotentialTarget(engine.EnumerateConjuncts(source), engine.OccurrencePath{0}, x) {
		t.Fatal("call argument was not screened")
	}
	if context.queries != 0 {
		t.Fatal("discovery queried provider")
	}
}
