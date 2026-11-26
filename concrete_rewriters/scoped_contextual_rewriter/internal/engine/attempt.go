package engine

import (
	"fmt"
	"slices"

	"github.com/lie-yan/predicate-optimization/analysis/scoped_reasoning"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/rewriter"
)

// OccurrencePath identifies ordered child edges in one immutable source snapshot.
// An empty path denotes the root; shared nodes at different paths are distinct occurrences.
type OccurrencePath []int

// Clone gives retained paths independent storage.
func (p OccurrencePath) Clone() OccurrencePath { return slices.Clone(p) }

// Extended extends a path without sharing its storage. Resolve checks index validity
// against the source; traversal callers supply indexes from Children().
func (p OccurrencePath) Extended(index int) OccurrencePath {
	child := make(OccurrencePath, len(p)+1)
	copy(child, p)
	child[len(p)] = index
	return child
}

// EqualsTo compares the ordered child indexes of two occurrence paths.
func (p OccurrencePath) EqualsTo(other OccurrencePath) bool { return slices.Equal(p, other) }

// IsPrefixOf includes equality: p equals the guard path at the frozen occurrence,
// is a strict prefix on its protected ancestor path, and is otherwise outside it.
// The guard selector separately establishes that every ancestor is an AND node.
func (p OccurrencePath) IsPrefixOf(other OccurrencePath) bool {
	return len(p) <= len(other) && slices.Equal(p, other[:len(p)])
}

// Resolve follows source edges without rewriting or evaluating expressions.
// Invalid indexes wrap ErrInvalidReplacement; a required nil root asserts.
func (p OccurrencePath) Resolve(source expressions.Expression) (expressions.Expression, error) {
	assertions.NotNil(source)
	current := source
	for depth, index := range p {
		count := current.ChildCount()
		if index < 0 || index >= count {
			return nil, fmt.Errorf("%w: occurrence path %v: index %d at depth %d outside %d children", rewriter.ErrInvalidReplacement, p, index, depth, count)
		}
		current = current.ChildAt(index)
	}
	return current, nil
}

// RewriteAttempt owns one conditional restriction under a retained guard.
// Constructed records are read-only; source expressions and analysis data are shared.
type RewriteAttempt struct {
	rule    *Engine                        // Engine whose analysis and eligibility contracts apply.
	source  *expressions.LogicalExpression // Original AND region for this attempt.
	scope   *scoped_reasoning.Scope        // Fresh identity for this attempt's assumptions.
	guard   GuardCandidate                 // Selected occurrence with an independently owned path.
	premise *scoped_reasoning.Premise      // The single retained occurrence justifying the restriction.
	input   *scoped_reasoning.InputDomain  // Proper nonempty restriction conditional on that premise.
}

// RewriteContext locates a visit within the attempt's original source snapshot.
type RewriteContext struct {
	Attempt *RewriteAttempt // Shared read-only owner and its conditional inputs.
	Path    OccurrencePath  // Owned ordered child edges from the owner's source root.
}

// Child retains the owner and extends the source path with independent storage.
func (c RewriteContext) Child(index int) RewriteContext {
	return RewriteContext{Attempt: c.Attempt, Path: c.Path.Extended(index)}
}

// NewAttempt consumes a guard selected for source; selection establishes its
// AND-only path, complete exact summary and Bool! Type. It neither re-extracts
// nor evaluates the guard. Errors propagate; absence means no useful restriction.
//
// For non-nullable Int64 x in x IN [1, 2] AND NOT(x == 3), selecting the
// first conjunct creates an attempt with inputs x in {1, 2}, conditional on
// retaining that guard at path [0]. A later step can prove x == 3 false under
// these inputs; this constructor neither replaces it nor removes the guard.
//
// With non-nullable x, representative guard outcomes are:
//
//	x IN [1, 2]                -> proper restriction: create an attempt
//	x == 1 OR x != 1           -> unchanged universe: return absence
//	NOT(x == 1 OR x != 1)      -> unreachable inputs: return absence
func (r *Engine) NewAttempt(source *expressions.LogicalExpression, guard GuardCandidate) (foundation.Optional[*RewriteAttempt], error) {
	absent := foundation.Optional[*RewriteAttempt]{}
	assertions.NotNilPointer(r)
	assertions.NotNilPointer(source)

	// Begin with all values admitted by the operand's declared Type. ValueSet
	// covers non-null values only; NULL is tracked independently in InputDomain.
	operand := guard.Summary.Operand()
	universe, err := finite.NewExcludedEquatableValueSet(r.factory, operand.Type(), nil)
	if err != nil {
		return absent, err
	}

	scope := scoped_reasoning.NewScope()
	input, err := scoped_reasoning.NewInputDomain(scope, operand, universe, operand.Type().Nullability() == foundation.MayNull, nil)
	if err != nil {
		return absent, err
	}

	// Each invocation registers a new occurrence token, even for the same source
	// object and path. Only this guard becomes a dependency of the restriction.
	// In the example, {1, 2} is conditional on this particular IN occurrence;
	// retrying the same guard creates fresh identities rather than reusing proof state.
	premise, err := scope.RegisterPremise(guard.Expression)
	if err != nil {
		return absent, err
	}
	conclusion, err := scoped_reasoning.RestrictToTrue(guard.Summary, input, premise)
	if err != nil {
		return absent, err
	}
	// A contradictory guard has no passing inputs. Declining here does not
	// replace the owner with False; contradiction rewriting is a separate task.
	if conclusion.Kind() == scoped_reasoning.Unreachable {
		return absent, nil
	}
	restricted, _ := conclusion.RestrictedDomain().Get()

	// A tautology supplies no narrower inputs. Compare both non-null values and
	// null possibility: removing NULL alone is a proper restriction as well.
	sameValues, err := restricted.PossibleValues().EqualsTo(input.PossibleValues())
	if err != nil {
		return absent, err
	}
	if sameValues && restricted.NullPossible() == input.NullPossible() {
		return absent, nil
	}

	guard.Path = guard.Path.Clone()
	attempt := &RewriteAttempt{rule: r, source: source, scope: scope, guard: guard, premise: premise, input: restricted}
	return foundation.Some(attempt), nil
}

// Returns the shared immutable analysis services.
func (a *RewriteAttempt) Rule() *Engine { return a.rule }

// Returns the original AND region.
func (a *RewriteAttempt) Source() *expressions.LogicalExpression { return a.source }

// Returns the scope of this attempt's assumptions.
func (a *RewriteAttempt) Scope() *scoped_reasoning.Scope { return a.scope }

// Copies the guard path while sharing immutable expression and summary data.
func (a *RewriteAttempt) Guard() GuardCandidate {
	g := a.guard
	g.Path = g.Path.Clone()
	return g
}

// Returns the retained guard occurrence.
func (a *RewriteAttempt) Premise() *scoped_reasoning.Premise { return a.premise }

// Returns the immutable guard-derived input domain.
func (a *RewriteAttempt) Input() *scoped_reasoning.InputDomain { return a.input }
