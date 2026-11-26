package expressions

import (
	"fmt"
	"strings"

	functioninterfaces "github.com/lie-yan/predicate-optimization/functions/interfaces"
	functionutils "github.com/lie-yan/predicate-optimization/functions/utils"
	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

// CallExpression invokes a resolved function with ordered expression arguments.
// It retains the caller's FunctionContext for rebinding the existing handle. Use NewCallExpression; zero is invalid.
type CallExpression struct {
	context   functioninterfaces.FunctionContext
	handle    functioninterfaces.FunctionHandle
	signature *functions.FunctionSignature
	arguments []Expression
}

var _ Expression = (*CallExpression)(nil)

// NewCallExpression checks handle/metadata agreement through the paired context,
// then binds actual argument Types without evaluation or overload resolution. Arguments must be valid
// immutable expressions. Nil context, handle or children assert. Binding and
// context errors are returned with their categories preserved. Construction
// establishes local call validity, not graph-wide identity consistency or equivalence.
// Exact non-null arguments reuse the implementation signature; other arguments
// use the binder for compatibility checks and nullable adaptation.
func NewCallExpression(context functioninterfaces.FunctionContext, handle functioninterfaces.FunctionHandle, arguments []Expression) (*CallExpression, error) {
	assertions.NotNil(context)
	assertions.NotNil(handle)
	metadata, err := functionutils.InspectFunction(context, handle)
	if err != nil {
		return nil, fmt.Errorf("call metadata: %w", err)
	}
	signature := metadata.Signature()
	exact := len(arguments) == signature.ArgumentCount()
	for i, argument := range arguments {
		assertions.NotNil(argument)
		if exact {
			typ := argument.Type()
			exact = typ.Nullability() == foundation.NonNull && typ.EqualsTo(signature.ArgumentTypeAt(i))
		}
	}
	// Exact non-null arguments need no null-policy adaptation or signature binding.
	if !exact {
		argumentTypes := make([]types.Type, len(arguments))
		for i, argument := range arguments {
			argumentTypes[i] = argument.Type()
		}
		signature, err = functions.BindFunctionSignature(metadata, argumentTypes)
		if err != nil {
			return nil, fmt.Errorf("call binding: %w", err)
		}
	}
	return &CallExpression{context: context, handle: handle, signature: signature, arguments: append([]Expression(nil), arguments...)}, nil
}

// Handle returns the shared immutable caller-owned function identity.
func (e *CallExpression) Handle() functioninterfaces.FunctionHandle {
	assertions.NotNilPointer(e)
	return e.handle
}

// Signature returns the shared effective signature, including actual argument
// Types and derived result nullability, rather than the implementation signature.
func (e *CallExpression) Signature() *functions.FunctionSignature {
	assertions.NotNilPointer(e)
	return e.signature
}

func (e *CallExpression) Type() types.Type {
	assertions.NotNilPointer(e)
	return e.signature.ResultType()
}

// Arguments returns a copied slice; immutable child expressions are shared.
func (e *CallExpression) Arguments() []Expression {
	assertions.NotNilPointer(e)
	return append([]Expression(nil), e.arguments...)
}

func (e *CallExpression) Children() []Expression { return e.Arguments() }

// ChildAt returns one shared child without copying a slice. Invalid indexes panic.
func (e *CallExpression) ChildAt(index int) Expression {
	assertions.NotNilPointer(e)
	return e.arguments[index]
}

// ChildCount returns the number of ordered expression children.
func (e *CallExpression) ChildCount() int { assertions.NotNilPointer(e); return len(e.arguments) }

func (e *CallExpression) EqualsTo(other Expression) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	// Shared immutable instances need no comparison memo.
	if e == other {
		return true
	}
	return e.equalsTo(other, make(map[[2]Expression]bool))
}

func (e *CallExpression) equalsTo(other Expression, seen map[[2]Expression]bool) bool {
	assertions.NotNilPointer(e)
	assertions.NotNil(other)
	o, ok := other.(*CallExpression)
	if !ok {
		return false
	}
	pair := [2]Expression{e, o}
	if e == o || seen[pair] {
		return true
	}
	if !e.handle.EqualsTo(o.handle) || !e.Type().EqualsTo(o.Type()) || len(e.arguments) != len(o.arguments) {
		return false
	}
	// Valid construction ties signature argument Types to the ordered children;
	// child equality therefore also compares the effective argument Types.
	for i, argument := range e.arguments {
		if !argument.equalsTo(o.arguments[i], seen) {
			return false
		}
	}
	seen[pair] = true
	return true
}

func (e *CallExpression) String() string {
	assertions.NotNilPointer(e)
	parts := make([]string, len(e.arguments))
	for i, argument := range e.arguments {
		parts[i] = argument.String()
	}
	return fmt.Sprintf("CallExpression(function=%q.%q, type=%s, arguments=[%s])", e.handle.Namespace(), e.handle.Name(), e.Type(), strings.Join(parts, ", "))
}

func (*CallExpression) isExpression() {}

// WithChildren implements checked immutable reconstruction.
func (e *CallExpression) WithChildren(children []Expression) (Expression, error) {
	assertions.NotNilPointer(e)
	if unchangedChildren(e.arguments, children) {
		return e, nil
	}
	rebuilt, err := NewCallExpression(e.context, e.handle, children)
	if err != nil {
		return nil, err
	}
	return checkRebuiltType(e, rebuilt)
}

// Context returns the retained provider paired with this call handle.
func (e *CallExpression) Context() functioninterfaces.FunctionContext {
	assertions.NotNilPointer(e)
	return e.context
}
