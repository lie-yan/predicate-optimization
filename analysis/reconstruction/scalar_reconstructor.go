// Package reconstruction builds equivalent expression candidates from exact
// predicate summaries. Candidate selection and rewrite safety belong to callers.
package reconstruction

import (
	"github.com/lie-yan/predicate-optimization/analysis/facts"
	finite "github.com/lie-yan/predicate-optimization/analysis/value_set/equatable_value_set"
	sets "github.com/lie-yan/predicate-optimization/analysis/value_set/sorted_range_set"
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// ScalarReconstructor builds candidates using an admitted target FunctionContext.
// It shares immutable inputs and never evaluates expressions. Use
// NewScalarReconstructor; the zero value is invalid.
type ScalarReconstructor struct {
	factory valueinterfaces.ValueFactory
	// context resolves comparison handles and admits equality used by IN.
	context functioninterfaces.FunctionContext
}

// NewScalarReconstructor retains the factory for new literals and range
// normalization. The factory and target context must be non-nil.
func NewScalarReconstructor(factory valueinterfaces.ValueFactory, context functioninterfaces.FunctionContext) *ScalarReconstructor {
	assertions.NotNil(factory)
	assertions.NotNil(context)
	return &ScalarReconstructor{factory: factory, context: context}
}

// Reconstruct returns an optional equivalent candidate with the source's exact
// result Type. Absence asks the caller to retain the source; it is distinct from
// a lookup, provider or construction error. It supports EquatableValueSet and SortedRangeSet.
// Singleton entries use = or != (Boolean exclusions use NOT(=)); larger lists
// use IN or NOT(IN), preserving Values() order. Empty/universal regions become
// literals only when NULL agrees.
// Nontrivial candidates require null propagation and matching declared result
// nullability. Range candidates preserve float NaN membership using negation
// where needed; discrete NaN entries remain unsupported. Required inputs must
// be constructed and immutable. The producer establishes source-graph validity
// (for example with expressions/utils.ValidateForOptimization at entry) and summary
// correctness. Reconstruction checks new calls without revalidating the whole source
// graph; new identities must preserve surrounding identity/Type consistency.
// MeasureExpression reports candidate/source sizes.
func (r *ScalarReconstructor) Reconstruct(summary *facts.PredicateSummary) (foundation.Optional[expressions.Expression], error) {
	if r == nil || assertions.IsNil(r.context) || assertions.IsNil(r.factory) {
		panic("reconstruction: expected constructed ScalarReconstructor")
	}
	sourceType := summary.Type().(*types.BoolType) // Also asserts summary construction.
	absent := foundation.Optional[expressions.Expression]{}
	set := summary.TrueSet()
	switch set.(type) {
	case *finite.EquatableValueSet, *sets.SortedRangeSet:
	default:
		return absent, nil
	}
	empty, err := set.IsEmpty()
	if err != nil {
		return absent, err
	}
	universal, err := set.IsUniversal()
	if err != nil {
		return absent, err
	}
	nullTruth, nullable := summary.NullTruth().Get()
	if empty || universal {
		// A Boolean constant must agree at both non-null and admitted null inputs.
		truth := foundation.False
		if universal {
			truth = foundation.True
		}
		if nullable && nullTruth != truth {
			return absent, nil
		}
		value, err := r.factory.NewBool(sourceType, universal)
		if err != nil {
			return absent, err
		}
		literal, err := expressions.NewLiteralExpression(value)
		if err != nil {
			return absent, err
		}
		return foundation.Some[expressions.Expression](literal), nil
	}

	// These predicate forms propagate null and derive their result nullability
	// from the operand. Preserve conservative source Types by declining mismatches.
	if (nullable && nullTruth != foundation.Null) || sourceType.Nullability() != summary.Operand().Type().Nullability() {
		return absent, nil
	}
	switch set := set.(type) {
	case *finite.EquatableValueSet:
		return r.reconstructDiscrete(summary, set.Values(), set.IsInclusion())
	case *sets.SortedRangeSet:
		return r.reconstructRanges(summary, set)
	default:
		return absent, nil
	}
}
