package functions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/internal/assertions"
)

func TestNullPolicyIndexedAccess(t *testing.T) {
	signature := metadataSignature(t)
	policies := []functions.NullPolicy{functions.PropagateNull, functions.PassNull}
	metadata := bindingMust(functions.NewFunctionMetadata(signature, "indexed", noBuiltIn, noOperator, policies))
	policies[0] = functions.NullPolicyUnspecified
	returned := metadata.NullPolicies()
	returned[1] = functions.NullPolicyUnspecified
	if metadata.NullPolicyAt(0) != functions.PropagateNull || metadata.NullPolicyAt(1) != functions.PassNull {
		t.Fatal("policy order or ownership changed")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if metadata.NullPolicyAt(0) != functions.PropagateNull || metadata.NullPolicyAt(1) != functions.PassNull {
			panic("policy changed")
		}
	}); allocations != 0 {
		t.Fatalf("policy access allocated %g times", allocations)
	}
	zeroSignature := bindingMust(functions.NewFunctionSignature(nil, signature.ResultType()))
	zero := bindingMust(functions.NewFunctionMetadata(zeroSignature, "zero", noBuiltIn, noOperator, nil))
	for _, m := range []*functions.FunctionMetadata{metadata, zero} {
		for _, index := range []int{-1, m.Signature().ArgumentCount(), m.Signature().ArgumentCount() + 1} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("invalid index %d did not panic", index)
					}
				}()
				m.NullPolicyAt(index)
			}()
		}
	}
}

func TestNullPolicyAtNilReceiver(t *testing.T) {
	if !assertions.Enabled {
		t.Skip("nil receiver assertions require debug build")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("nil receiver did not panic")
		}
	}()
	var metadata *functions.FunctionMetadata
	metadata.NullPolicyAt(0)
}
