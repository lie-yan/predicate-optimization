package functions_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func varcharPattern(t *testing.T, name string, n foundation.Nullability) types.TypePattern {
	t.Helper()
	pattern, err := types.NewVarCharTypePattern(name, n)
	if err != nil {
		t.Fatal(err)
	}
	return pattern
}

func declaration(t *testing.T, args []types.TypePattern, result types.TypePattern) *functions.DeclarationSignature {
	t.Helper()
	signature, err := functions.NewDeclarationSignature(args, result)
	if err != nil {
		t.Fatal(err)
	}
	return signature
}

func TestDeclarationParameterScope(t *testing.T) {
	x := varcharPattern(t, "x", foundation.NonNull)
	y := varcharPattern(t, "y", foundation.NonNull)
	z := varcharPattern(t, "z", foundation.NonNull)
	xy := declaration(t, []types.TypePattern{x, y}, x)
	renamed := declaration(t, []types.TypePattern{y, z}, y)
	if !xy.EqualsTo(renamed) || !renamed.EqualsTo(xy) {
		t.Fatal("consistent parameter renaming must preserve declaration identity")
	}
	for _, different := range []*functions.DeclarationSignature{
		declaration(t, []types.TypePattern{x, y}, y),
		declaration(t, []types.TypePattern{x, x}, x),
		declaration(t, []types.TypePattern{x, varcharPattern(t, "y", foundation.MayNull)}, x),
	} {
		if xy.EqualsTo(different) || different.EqualsTo(xy) {
			t.Fatal("result binding, repeated-variable constraints and nullability must remain significant")
		}
	}
	if _, err := functions.NewDeclarationSignature([]types.TypePattern{x}, y); err == nil {
		t.Fatal("result-only parameter accepted")
	}
	if _, err := functions.NewDeclarationSignature(nil, x); err == nil {
		t.Fatal("unbound zero-argument result accepted")
	}
	if _, err := types.NewVarCharTypePattern("", foundation.NonNull); err == nil {
		t.Fatal("empty parameter accepted")
	}
	if _, err := types.NewVarCharTypePattern("x", foundation.Nullability(255)); err == nil {
		t.Fatal("invalid nullability accepted")
	}
}

func TestDeclarationFixedTypesAndUnboundedVarchar(t *testing.T) {
	boolean, err := types.NewBoolType(foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	result := types.NewConcreteTypePattern(boolean)
	zero, max := uint64(0), uint64(math.MaxUint64)
	var signatures []*functions.DeclarationSignature
	for _, bound := range []*uint64{nil, &zero, &max} {
		text, err := types.NewVarCharType(bound, foundation.NonNull)
		if err != nil {
			t.Fatal(err)
		}
		fixed, err := functions.NewFunctionSignature([]types.Type{text, text}, boolean)
		if err != nil {
			t.Fatal(err)
		}
		converted := functions.DeclarationFromSignature(fixed)
		explicit := declaration(t, []types.TypePattern{types.NewConcreteTypePattern(text), types.NewConcreteTypePattern(text)}, result)
		if !converted.EqualsTo(explicit) {
			t.Fatal("fixed adaptation changed the declaration")
		}
		if converted.ArgumentAt(0).(*types.ConcreteTypePattern).Type() != text {
			t.Fatal("fixed adaptation must share immutable concrete descriptors")
		}
		signatures = append(signatures, converted)
	}
	variable := varcharPattern(t, "x", foundation.NonNull)
	signatures = append(signatures, declaration(t, []types.TypePattern{variable, variable}, result))
	for i, a := range signatures {
		for j, b := range signatures {
			if a.EqualsTo(b) != (i == j) {
				t.Fatal("unbounded, zero, maximum finite bound and variable must be distinct")
			}
		}
	}
	fixed, err := functions.NewFunctionSignature(nil, boolean)
	if err != nil {
		t.Fatal(err)
	}
	if !functions.DeclarationFromSignature(fixed).EqualsTo(declaration(t, nil, result)) {
		t.Fatal("fixed zero-argument adaptation changed")
	}
}

func TestDeclarationOwnershipAndTypeSeparation(t *testing.T) {
	x := varcharPattern(t, "x", foundation.NonNull)
	y := varcharPattern(t, "y", foundation.NonNull)
	args := []types.TypePattern{x, x}
	signature := declaration(t, args, x)
	args[0] = y
	returned := signature.Arguments()
	returned[1] = y
	names := x.Parameters()
	names[0] = "changed"
	if !signature.EqualsTo(declaration(t, []types.TypePattern{x, x}, x)) {
		t.Fatal("caller mutation changed declaration parameter identity")
	}
	if _, ok := any(x).(types.Type); ok {
		t.Fatal("unresolved pattern must not be usable as an executable Type")
	}
}
