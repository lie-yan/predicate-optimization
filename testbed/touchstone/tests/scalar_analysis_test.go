package touchstone_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/interpreter"
	fixture "github.com/lie-yan/predicate-optimization/testbed/touchstone"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

type scalarTarget struct {
	name   string
	source expressions.Expression
	want   foundation.TruthValue
}

// Select actual source occurrences, including the three structurally equal
// four-item memberships. Expected is never consulted to manufacture a proof.
func touchstoneScalarTargets(t *testing.T, f *fixture.Touchstone) (expressions.Expression, []scalarTarget) {
	t.Helper()
	root, ok := f.Input.(*expressions.LogicalExpression)
	if !ok || root.Operator() != foundation.And || len(root.Children()) != 7 {
		t.Fatal("unexpected touchstone root")
	}
	children := root.Children()
	nestedScalar := func(source expressions.Expression) expressions.Expression {
		t.Helper()
		not, ok := source.(*expressions.LogicalExpression)
		if !ok || not.Operator() != foundation.Not {
			t.Fatal("expected negated guarded term")
		}
		and, ok := not.Children()[0].(*expressions.LogicalExpression)
		if !ok || and.Operator() != foundation.And || len(and.Children()) != 2 {
			t.Fatal("expected array/scalar conjunction")
		}
		return and.Children()[1]
	}
	tags, ok := children[6].(*expressions.LogicalExpression)
	if !ok || tags.Operator() != foundation.And || len(tags.Children()) != 7 {
		t.Fatal("unexpected tag group")
	}
	tagChildren := tags.Children()
	return children[0], []scalarTarget{
		{"tsa_04", nestedScalar(children[1]), foundation.False},
		{"tag_566844", nestedScalar(tagChildren[1]), foundation.True},
		{"tag_569075", nestedScalar(tagChildren[2]), foundation.True},
		{"tag_2607471", nestedScalar(tagChildren[4]), foundation.True},
		{"tag_2752371", nestedScalar(tagChildren[5]), foundation.True},
	}
}

func touchstoneSummary(t *testing.T, f *fixture.Touchstone, source expressions.Expression) *facts.PredicateSummary {
	t.Helper()
	result := touchstoneMust(fact_extraction.NewScalarExtractor(testbedvalues.ValueFactory{}, f.Context).Extract(source))
	summary, present := result.Summary().Get()
	if result.Status() != fact_extraction.Exact || !present || summary.Source() != source || !summary.Operand().EqualsTo(f.Columns.ArticleType) {
		t.Fatal("expected exact article summary retaining source occurrence")
	}
	return summary
}

// Restrict the declared article universe by one retained outer-guard occurrence.
// The API supplies conditional evidence, without changing either fixture tree.
func touchstoneGuardDomain(t *testing.T, f *fixture.Touchstone, guard expressions.Expression) (*scoped_reasoning.InputDomain, *scoped_reasoning.InputDomain, *scoped_reasoning.Premise) {
	t.Helper()
	context := scoped_reasoning.NewScopedFactContext()
	scope := context.NewScope()
	article := f.Columns.ArticleType
	all := touchstoneMust(finite.NewExcludedEquatableValueSet(testbedvalues.ValueFactory{}, article.Type(), nil))
	base := touchstoneMust(scoped_reasoning.NewInputDomain(scope, article, all, article.Type().Nullability() == foundation.MayNull, nil))
	premise := touchstoneMust(context.RegisterPremise(scope, guard))
	conclusion, present := touchstoneMust(scoped_reasoning.RestrictToTrue(touchstoneSummary(t, f, guard), base, premise)).Get()
	if !present || conclusion.Kind() != scoped_reasoning.RestrictedDomain {
		t.Fatal("guard restriction missing")
	}
	domain, present := conclusion.RestrictedDomain().Get()
	if !present || domain.NullPossible() || domain.Operand() != article || domain.Scope() != scope {
		t.Fatal("guard domain changed identity or admitted NULL")
	}
	var expected []valueinterfaces.Value
	for _, articleValue := range []string{"tsa_01", "tsa_02", "tsa_03"} {
		expected = append(expected, touchstoneMust(testbedvalues.NewVarChar(article.Type().(*types.VarCharType), articleValue)))
	}
	if !touchstoneMust(domain.PossibleValues().EqualsTo(touchstoneMust(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, article.Type(), expected)))) {
		t.Fatal("incorrect outer article restriction")
	}
	if dependencies := domain.Premises(); len(dependencies) != 1 || dependencies[0] != premise || premise.Expression() != guard {
		t.Fatal("guard occurrence dependency lost")
	}
	return base, domain, premise
}

func checkTouchstoneScalarProof(t *testing.T, summary *facts.PredicateSummary, input *scoped_reasoning.InputDomain, premise *scoped_reasoning.Premise, want foundation.TruthValue) {
	t.Helper()
	conclusion, present := touchstoneMust(scoped_reasoning.ProveTruth(summary, input)).Get()
	if !present || conclusion.Kind() != scoped_reasoning.KnownTruth {
		t.Fatal("expected uniform conditional truth")
	}
	truth, present := conclusion.Truth().Get()
	if !present || truth != want {
		t.Fatalf("proof truth=%v, want=%v", truth, want)
	}
	if conclusion.Summary() != summary || conclusion.InputDomain() != input || conclusion.Scope() != input.Scope() {
		t.Fatal("proof lost its source/context")
	}
	dependencies := conclusion.Premises()
	if len(dependencies) != 1 || dependencies[0] != premise {
		t.Fatal("proof did not retain the outer guard")
	}
	// ProveTruth preserves the assumption; its result never discharges that premise.
	dependencies[0] = nil
	if conclusion.Premises()[0] != premise {
		t.Fatal("proof dependencies escaped mutably")
	}
}

func TestTouchstoneScalarProofs(t *testing.T) {
	f := touchstoneMust(fixture.NewPrimaryTouchstone())
	originalInput, originalExpected := f.Input, f.Expected
	guard, targets := touchstoneScalarTargets(t, f)
	base, domain, premise := touchstoneGuardDomain(t, f, guard)
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			summary := touchstoneSummary(t, f, target.source)
			if _, present := summary.NullTruth().Get(); present {
				t.Fatal("non-null article acquired null-input outcome")
			}
			if _, present := touchstoneMust(scoped_reasoning.ProveTruth(summary, base)).Get(); present {
				t.Fatal("conditional scalar fact became global")
			}
			// Check the concrete set relation as well as the public scoped query.
			if target.want == foundation.False {
				if !touchstoneMust(domain.PossibleValues().IsDisjointFrom(summary.TrueSet())) {
					t.Fatal("tsa_04 was not disjoint")
				}
			} else if !touchstoneMust(domain.PossibleValues().IsSubsetOf(summary.TrueSet())) {
				t.Fatal("outer article set was not contained")
			}
			checkTouchstoneScalarProof(t, summary, domain, premise, target.want)
			// All values in the finite guard domain are also checked independently
			// through interpretation of the original nested source occurrence.
			for _, article := range []string{"tsa_01", "tsa_02", "tsa_03"} {
				result := touchstoneMust(evaluator.Evaluate(target.source, (inputRow{review: 8795001, article: article}).environment(f)))
				if result.IsNull() || result.(*testbedvalues.BoolValue).Bool() != (target.want == foundation.True) {
					t.Fatalf("source disagrees at article=%s", article)
				}
			}
		})
	}
	// Equal membership propositions occupy three distinct source positions.
	for _, index := range []int{3, 4} {
		if targets[2].source == targets[index].source || !targets[2].source.EqualsTo(targets[index].source) {
			t.Fatal("expected separate equivalent membership occurrences")
		}
	}
	if f.Input != originalInput || f.Expected != originalExpected {
		t.Fatal("proof generation replaced a fixture tree")
	}
}

func TestTouchstoneNullableScalarProofsDoNotAuthorizeReplacement(t *testing.T) {
	f := touchstoneMust(fixture.NewTouchstone(foundation.MayNull, foundation.MayNull))
	guard, targets := touchstoneScalarTargets(t, f)
	_, domain, premise := touchstoneGuardDomain(t, f, guard)
	// These are independently specified diagnostic input domains, not restrictions
	// inferred from a true guard. They deliberately admit NULL article testbedvalues.
	withNull := touchstoneMust(scoped_reasoning.NewInputDomain(domain.Scope(), f.Columns.ArticleType, domain.PossibleValues(), true, nil))
	empty := touchstoneMust(finite.NewEquatableValueSet(testbedvalues.ValueFactory{}, f.Columns.ArticleType.Type(), nil))
	nullOnly := touchstoneMust(scoped_reasoning.NewInputDomain(domain.Scope(), f.Columns.ArticleType, empty, true, nil))
	evaluator := interpreter.NewExpressionInterpreterWithFunctionContext(testbedvalues.ValueFactory{}, f.Context)
	nullRow := inputRow{review: 8795001, tags: []int64{8791004}, nullColumns: map[string]bool{"article": true}}
	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			summary := touchstoneSummary(t, f, target.source)
			truth, present := summary.NullTruth().Get()
			if !present || truth != foundation.Null {
				t.Fatal("nullable scalar summary lost Null outcome")
			}
			// All five proofs still hold on rows where the retained guard is true.
			checkTouchstoneScalarProof(t, summary, domain, premise, target.want)
			// They stop being uniform when NULL article values are also possible.
			if _, present := touchstoneMust(scoped_reasoning.ProveTruth(summary, withNull)).Get(); present {
				t.Fatal("guarded truth was generalized to NULL input")
			}
			proof, present := touchstoneMust(scoped_reasoning.ProveTruth(summary, nullOnly)).Get()
			if !present || proof.Kind() != scoped_reasoning.KnownTruth {
				t.Fatal("null-only outcome missing")
			}
			truth, present = proof.Truth().Get()
			if !present || truth != foundation.Null {
				t.Fatal("null-only proof was not known NULL")
			}
			if !touchstoneMust(evaluator.Evaluate(target.source, nullRow.environment(f))).IsNull() {
				t.Fatal("source did not evaluate to NULL")
			}
		})
	}
	if f.Columns.ArticleType.Type().Nullability() != foundation.MayNull {
		t.Fatal("contextual restriction changed declared Type")
	}
	// Original: NULL guard AND NOT(true AND NULL) remains NULL.
	// Proposed output: NULL guard AND NOT(true) becomes False.
	// The expected tree is independently assembled; no rewrite is applied here.
	assertPair(t, f, nullRow, foundation.Null, foundation.False)
	assertPair(t, f, inputRow{review: 8795001, nullColumns: map[string]bool{"article": true}}, foundation.Null, foundation.Null)
}
