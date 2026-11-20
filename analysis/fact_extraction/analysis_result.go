package fact_extraction

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/analysis/facts"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
)

// AnalysisStatus distinguishes an exact summary from unsupported extraction.
// Zero is invalid; neither status is a predicate truth value.
type AnalysisStatus uint8

const (
	Exact AnalysisStatus = iota + 1
	Unsupported
)

// AnalysisResult retains the original predicate in both states. Only Exact has
// a summary; only Unsupported has a diagnostic reason. Invalid input and
// provider failures are returned errors, not additional result states. Fields
// are private so the constructors establish these combinations. The zero value
// is invalid; accessors require a constructed result.
type AnalysisResult struct {
	status  AnalysisStatus
	source  expressions.Expression
	summary *facts.PredicateSummary
	reason  string
}

// NewExactAnalysisResult shares a constructed summary and its source. It trusts
// the producer's exactness claim; nil/unconstructed summaries assert.
func NewExactAnalysisResult(summary *facts.PredicateSummary) AnalysisResult {
	return AnalysisResult{status: Exact, source: summary.Source(), summary: summary}
}

// NewUnsupportedAnalysisResult preserves a valid immutable Boolean predicate
// and a nonempty diagnostic reason. It performs no extraction and has no summary.
// Nil sources assert; non-Boolean sources and empty reasons return errors.
func NewUnsupportedAnalysisResult(source expressions.Expression, reason string) (AnalysisResult, error) {
	assertions.NotNil(source)
	if _, ok := source.Type().(*types.BoolType); !ok {
		return AnalysisResult{}, fmt.Errorf("analysis source must have Boolean type")
	}
	if reason == "" {
		return AnalysisResult{}, fmt.Errorf("unsupported analysis requires a diagnostic reason")
	}
	return AnalysisResult{status: Unsupported, source: source, reason: reason}, nil
}

func (r AnalysisResult) Status() AnalysisStatus         { r.assertConstructed(); return r.status }
func (r AnalysisResult) Source() expressions.Expression { r.assertConstructed(); return r.source }
func (r AnalysisResult) Type() types.Type               { r.assertConstructed(); return r.source.Type() }
func (r AnalysisResult) Summary() foundation.Optional[*facts.PredicateSummary] {
	r.assertConstructed()
	if r.status == Exact {
		return foundation.Some(r.summary)
	}
	return foundation.Optional[*facts.PredicateSummary]{}
}

func (r AnalysisResult) Reason() foundation.Optional[string] {
	r.assertConstructed()
	if r.status == Unsupported {
		return foundation.Some(r.reason)
	}
	return foundation.Optional[string]{}
}

func (r AnalysisResult) assertConstructed() {
	if r.status != Exact && r.status != Unsupported {
		panic("fact_extraction: expected constructed AnalysisResult")
	}
}
