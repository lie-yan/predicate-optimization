package functions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/internal/assertions"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/types"
)

func TestFunctionSignatureOwnership(t *testing.T) {
	element, err := types.NewIntType(types.Int64, foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	array, err := types.NewArrayType(element, foundation.NonNull)
	if err != nil {
		t.Fatal(err)
	}
	result, err := types.NewStructType([]types.StructField{{Name: "items", Type: array}}, foundation.MayNull)
	if err != nil {
		t.Fatal(err)
	}
	args := []types.Type{array, element}
	signature, err := functions.NewFunctionSignature(args, result)
	if err != nil {
		t.Fatal(err)
	}
	args[0] = element
	got := signature.ArgumentTypes()
	if len(got) != 2 || got[0] != array || got[1] != element || signature.ResultType() != result {
		t.Fatal("signature must copy the argument slice and share its type descriptors")
	}
	got[1] = array
	if signature.ArgumentTypes()[1] != element {
		t.Fatal("mutating returned arguments changed the signature")
	}
	if signature.ArgumentTypes()[0].String() != "Array<Int64?>!" ||
		signature.ResultType().String() != "Struct<\"items\": Array<Int64?>!>?" {
		t.Fatal("nested structure or nullability was lost")
	}
}

func TestFunctionSignatureZeroArguments(t *testing.T) {
	result := &types.BoolType{}
	for _, args := range [][]types.Type{nil, {}} {
		signature, err := functions.NewFunctionSignature(args, result)
		if err != nil {
			t.Fatal(err)
		}
		if len(signature.ArgumentTypes()) != 0 || signature.ResultType() != result {
			t.Fatal("zero-argument signature changed")
		}
	}
}

func TestFunctionSignatureNilReceiver(t *testing.T) {
	if !assertions.Enabled {
		t.Skip("nil receiver assertions require debug build")
	}
	var signature *functions.FunctionSignature
	for name, call := range map[string]func(){
		"arguments": func() { signature.ArgumentTypes() },
		"count":     func() { signature.ArgumentCount() },
		"indexed":   func() { signature.ArgumentTypeAt(0) },
		"result":    func() { signature.ResultType() },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected nil-receiver assertion")
				}
			}()
			call()
		})
	}
}

func TestFunctionSignatureNilTypeAssertions(t *testing.T) {
	valid, _ := types.NewBoolType(foundation.NonNull)
	for _, typ := range []types.Type{nil, (*types.IntType)(nil)} {
		for _, call := range []func(){
			func() { _, _ = functions.NewFunctionSignature([]types.Type{typ}, valid) },
			func() { _, _ = functions.NewFunctionSignature(nil, typ) },
		} {
			func() {
				if !assertions.Enabled {
					return
				}
				defer func() {
					if recover() == nil {
						t.Error("expected non-nil assertion")
					}
				}()
				call()
			}()
		}
	}
}
