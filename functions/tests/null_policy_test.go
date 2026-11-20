package functions_test

import (
	"testing"

	"github.com/lie-yan/predicate-optimization/functions"
)

func TestNullPolicyConservativeDefault(t *testing.T) {
	var policy functions.NullPolicy
	if !policy.IsValid() || policy != functions.NullPolicyUnspecified {
		t.Fatal("zero-value policy must be valid but unspecified")
	}
	if functions.NullPolicy(255).IsValid() {
		t.Fatal("out-of-range policy must not be accepted as unspecified")
	}
}
