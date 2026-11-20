// Package engine implements retained-guard scalar rewriting behind the public rule.
// Its exported operations and records are internal implementation APIs, shared
// with tests; they are not accessible to callers outside this rewriter subtree.
package engine

import (
	"github.com/lie-yan/predicate-optimization/analysis/fact_extraction"
	"github.com/lie-yan/predicate-optimization/concrete_rewriters/internal/boolean_cleanup"
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Engine owns the shared analysis services for retained-guard scalar rewriting.
// Construct it with New before use. Its configuration is read-only thereafter;
// tentative changes are confined to separately constructed RewriteAttempts.
type Engine struct {
	factory   valueinterfaces.ValueFactory
	Extractor *fact_extraction.ScalarExtractor // Shared scalar analysis service; read-only after construction.
	cleanup   *boolean_cleanup.Cleaner
}

// New shares the function context without
// querying or evaluating it. Nil, including typed nil, violates the precondition
// and panics. Comparison calls retain their own paired contexts.
//
// Providers must supply correct, stable, effect-free and successful evaluation
// for supported comparisons, Boolean NOT and array-membership signatures over
// all valid effective inputs, including the declared null behavior. Reference
// lookup must be stable, successful and effect-free on complete valid inputs.
// These guarantees do not admit arbitrary calls, arithmetic or opaque regions.
//
// Expressions, values and provider semantics remain immutable during rewriting.
// The engine retains no mutable attempt state; concurrent use requires matching
// guarantees from the supplied providers.
func New(factory valueinterfaces.ValueFactory, functionContext functioninterfaces.FunctionContext) *Engine {
	return &Engine{
		factory:   factory,
		Extractor: fact_extraction.NewScalarExtractor(factory, functionContext),
		cleanup:   boolean_cleanup.New(factory, functionContext),
	}
}
