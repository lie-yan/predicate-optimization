// Package boolean_cleanup shares local Boolean identities and their evaluation
// eligibility checks between concrete rewriters. It carries no scoped assumptions.
package boolean_cleanup

import (
	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Cleaner supplies local cleanup and admission under the caller's provider
// contracts. It is immutable after construction; use New before calling methods.
type Cleaner struct {
	factory         valueinterfaces.ValueFactory
	functionContext functioninterfaces.FunctionContext
}

// New shares a non-nil function context without querying it. Supported
// comparisons, Boolean NOT and array membership must have stable, effect-free,
// successful evaluation over valid effective inputs, including null policies.
// Reference lookup must be stable, successful and effect-free on complete inputs.
// Calls retain their own paired contexts; arbitrary calls and arithmetic are not
// admitted by these guarantees. Expressions and providers remain immutable in use.
func New(factory valueinterfaces.ValueFactory, functionContext functioninterfaces.FunctionContext) *Cleaner {
	assertions.NotNil(factory)
	assertions.NotNil(functionContext)
	return &Cleaner{factory: factory, functionContext: functionContext}
}
