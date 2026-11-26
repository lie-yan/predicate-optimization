package engine

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// ConjunctOccurrence retains one non-AND conjunct and its path in the source tree.
// Equal or shared expressions at different paths remain separate occurrences.
type ConjunctOccurrence struct {
	Path       OccurrencePath         // Ordered child edges from the original owner root.
	Expression expressions.Expression // Complete conjunct, including any OR or NOT subtree.
}

// EnumerateConjuncts walks only AND edges in stored order, without rebuilding the
// tree. Other expressions form whole candidates; discovery establishes no facts.
//
// For (G AND A) AND (B OR C), it records:
//
//	[0, 0] -> G
//	[0, 1] -> A
//	[1]    -> B OR C
//
// The whole OR must be true when the conjunction is true, but neither branch
// must independently be true. NOT subtrees likewise remain whole candidates.
// Shared expressions at different paths remain distinct occurrences.
func EnumerateConjuncts(source expressions.Expression) []ConjunctOccurrence {
	assertions.NotNil(source)
	var result []ConjunctOccurrence
	var visit func(expressions.Expression, OccurrencePath)
	visit = func(node expressions.Expression, path OccurrencePath) {
		if logical, ok := node.(*expressions.LogicalExpression); ok && logical.Operator() == foundation.And {
			for i, count := 0, logical.ChildCount(); i < count; i++ {
				visit(logical.ChildAt(i), path.Extended(i))
			}
			return
		}
		result = append(result, ConjunctOccurrence{Path: path, Expression: node})
	}
	visit(source, nil)
	return result
}

// HasPotentialTarget screens other conjunct subtrees for the selected operand.
// The guard path must identify an enumerated occurrence in the same snapshot.
// A match is only a reason to try analysis, not proof or evaluation eligibility.
func HasPotentialTarget(conjuncts []ConjunctOccurrence, guardPath OccurrencePath, operand *expressions.ReferenceExpression) bool {
	assertions.NotNilPointer(operand)
	for _, conjunct := range conjuncts {
		if !conjunct.Path.EqualsTo(guardPath) && containsOperand(conjunct.Expression, operand) {
			return true
		}
	}
	return false
}

// containsOperand examines exposed children only, using reference equality
// (caller-defined column identity and full declared Type), never diagnostic names.
func containsOperand(source expressions.Expression, operand *expressions.ReferenceExpression) bool {
	switch node := source.(type) {
	case *expressions.ReferenceExpression:
		return operand.EqualsTo(node)
	case *expressions.OpaqueExpression:
		return false
	default:
		for i, count := 0, source.ChildCount(); i < count; i++ {
			if containsOperand(source.ChildAt(i), operand) {
				return true
			}
		}
		return false
	}
}

// GuardCandidate associates a complete two-valued conjunct with its exact summary.
// Selection records possible guards; it neither installs premises nor restricts domains.
type GuardCandidate struct {
	Path       OccurrencePath          // Owned path to the retained occurrence in the original source.
	Expression expressions.Expression  // Complete candidate subtree, shared unchanged.
	Summary    *facts.PredicateSummary // Exact single-operand meaning of that subtree.
}

// SelectGuards retains exact Bool! conjuncts with potential same-operand targets,
// in stored source order. Unsupported candidates are skipped; encountered errors
// abort selection. Actual restriction usefulness is decided by later attempts.
func (r *Engine) SelectGuards(source *expressions.LogicalExpression) ([]GuardCandidate, error) {
	if r == nil || r.extractor == nil {
		panic("scoped_contextual_rewriter: expected constructed ScopedContextualRewriter")
	}
	assertions.NotNilPointer(source)
	if source.Operator() != foundation.And {
		return nil, nil
	}
	// For non-nullable x, x > 0 AND x > 5 AND x > 10 yields candidates at
	// [0], [1] and [2], each with its original expression and exact true set.
	// Keep all three in source order: assuming x > 0 cannot determine either
	// stronger predicate, but a later attempt using x > 5 can prove x > 0 true.
	// Selection itself assumes no guard true and installs no scoped facts.
	conjuncts := EnumerateConjuncts(source)
	var guards []GuardCandidate
	for _, conjunct := range conjuncts {
		// Require declared Bool! before extraction. Comparisons on nullable x
		// ordinarily yield Bool? and cannot supply this two-valued guard contract.
		boolean, ok := conjunct.Expression.Type().(*types.BoolType)
		if !ok || boolean.Nullability() != foundation.NonNull {
			continue
		}
		// Summarize the complete conjunct, including an OR or NOT subtree;
		// an exact inner predicate alone does not establish the whole guard.
		result, err := r.extractor.Extract(conjunct.Expression)
		if err != nil {
			// Preserve operational failures and discard any partial candidate list.
			return nil, fmt.Errorf("guard at %v: %w", conjunct.Path, err)
		}
		// Unsupported forms and mixed-operand predicates cannot provide an
		// exact single-operand summary; skip them and consider later conjuncts.
		if result.Status() != fact_extraction.Exact {
			continue
		}
		// Exact guarantees a summary. Require another conjunct to expose the
		// same column identity and full declared Type: x > 0 AND y > 5 offers
		// no such target. A match suggests usefulness; later attempts decide it.
		summary, _ := result.Summary().Get()
		if HasPotentialTarget(conjuncts, conjunct.Path, summary.Operand()) {
			guards = append(guards, GuardCandidate{Path: conjunct.Path.Clone(), Expression: conjunct.Expression, Summary: summary})
		}
	}
	return guards, nil
}
