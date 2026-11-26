package utils

import (
	"fmt"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/functions"
	functionutils "github.com/lie-yan/predicate-optimization/functions/utils"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
	"github.com/lie-yan/predicate-optimization/types"
)

// Validate checks an entire immutable graph, including binding consistency.
// Nil nodes, malformed package-owned nodes and cycles return errors.
// Caller-owned IDs and handles must honor their own validity/equality contracts.
func Validate(root expressions.Expression) error {
	return (&graphValidator{states: make(map[expressions.Expression]uint8)}).visit(root)
}

// ValidateForOptimization checks graph structure and every stored call using its
// own paired context. It verifies handle/metadata agreement and effective binding
// without lookup, evaluation or runtime reference access. Valid unsupported calls
// and opaque expressions remain valid; success does not imply rewrite eligibility.
// Providers must return valid immutable metadata and retain stable meanings.
func ValidateForOptimization(root expressions.Expression) error {
	return (&graphValidator{states: make(map[expressions.Expression]uint8), useCallContexts: true}).visit(root)
}

type graphValidator struct {
	// states tracks unseen (0), visiting (1) and completed (2) nodes to detect cycles and visit shared nodes once.
	states map[expressions.Expression]uint8
	// useCallContexts enables metadata inspection using each call's retained context.
	useCallContexts bool
	// references records each canonical column key's full declared Type to reject conflicting declarations.
	references map[string]types.Type
	// opaques records each canonical opaque-region key's full declared Type independently of column identities.
	opaques map[string]types.Type
}

func (v *graphValidator) visit(node expressions.Expression) error {
	if assertions.IsNil(node) {
		return fmt.Errorf("nil expression")
	}
	// Reject embedded/foreign implementations before using a node as a map key.
	switch node.(type) {
	case *expressions.LiteralExpression, *expressions.ReferenceExpression, *expressions.OpaqueExpression, *expressions.CallExpression, *expressions.LogicalExpression, *expressions.SpecialFormExpression:
	default:
		return fmt.Errorf("unsupported expression implementation %T", node)
	}
	switch v.states[node] {
	case 1:
		return fmt.Errorf("expression cycle at %T", node)
	case 2:
		return nil
	}
	v.states[node] = 1
	for i, child := range node.Children() {
		if err := v.visit(child); err != nil {
			return fmt.Errorf("%T child %d: %w", node, i, err)
		}
	}
	if err := v.local(node); err != nil {
		return fmt.Errorf("%T: %w", node, err)
	}
	v.states[node] = 2
	return nil
}

func (v *graphValidator) local(node expressions.Expression) error {
	switch e := node.(type) {
	case *expressions.LiteralExpression:
		if assertions.IsNil(e.Value()) {
			return fmt.Errorf("invalid literal value")
		}
	case *expressions.ReferenceExpression:
		if assertions.IsNil(e.ID()) || assertions.IsNil(e.Type()) {
			return fmt.Errorf("invalid reference identity or type")
		}
		key := e.ID().Key()
		if previous, present := v.references[key]; present {
			if !e.Type().EqualsTo(previous) {
				return fmt.Errorf("conflicting types for column identity")
			}
		} else {
			if v.references == nil {
				v.references = make(map[string]types.Type)
			}
			v.references[key] = e.Type()
		}
	case *expressions.OpaqueExpression:
		if assertions.IsNil(e.ID()) || assertions.IsNil(e.Type()) {
			return fmt.Errorf("invalid opaque identity or type")
		}
		key := e.ID().Key()
		if previous, present := v.opaques[key]; present {
			if !e.Type().EqualsTo(previous) {
				return fmt.Errorf("conflicting types for opaque identity")
			}
		} else {
			if v.opaques == nil {
				v.opaques = make(map[string]types.Type)
			}
			v.opaques[key] = e.Type()
		}
	case *expressions.CallExpression:
		if assertions.IsNil(e.Context()) || assertions.IsNil(e.Handle()) || e.Signature() == nil {
			return fmt.Errorf("missing call context, handle or signature")
		}
		args := e.Signature().ArgumentTypes()
		if assertions.IsNil(e.Signature().ResultType()) {
			return fmt.Errorf("missing call result type")
		}
		for _, arg := range args {
			if assertions.IsNil(arg) {
				return fmt.Errorf("missing call argument type")
			}
		}
		if len(args) != e.ChildCount() {
			return fmt.Errorf("call signature arity mismatch")
		}
		children := e.Children()
		for i, arg := range args {
			if !arg.EqualsTo(children[i].Type()) {
				return fmt.Errorf("call signature argument %d type mismatch", i)
			}
		}
		if v.useCallContexts {
			metadata, err := functionutils.InspectFunction(e.Context(), e.Handle())
			if err != nil {
				return fmt.Errorf("call metadata: %w", err)
			}
			if err := functions.ValidateCallSignature(metadata, e.Signature()); err != nil {
				return fmt.Errorf("call signature: %w", err)
			}
		}
	case *expressions.LogicalExpression:
		if assertions.IsNil(e.Type()) {
			return fmt.Errorf("invalid logical result type")
		}
		checked, err := expressions.NewLogicalExpression(e.Operator(), e.Children())
		if err != nil {
			return err
		}
		if !checked.Type().EqualsTo(e.Type()) {
			return fmt.Errorf("logical result type mismatch")
		}
	case *expressions.SpecialFormExpression:
		if assertions.IsNil(e.Type()) {
			return fmt.Errorf("invalid special-form result type")
		}
		if _, err := expressions.NewSpecialFormExpression(e.Form(), e.Type(), e.Arguments()); err != nil {
			return err
		}
	}
	return nil
}
