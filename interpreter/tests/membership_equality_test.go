package interpreter_test

import (
	"errors"
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	"github.com/lie-yan/predicate-optimization/interpreter"
	testbed "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func TestMembershipEquality(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	boolean := must(types.NewBoolType(foundation.NonNull))
	text := must(types.NewVarCharType(nil, foundation.NonNull))
	cases := []struct {
		left, right valueinterfaces.Value
		equal       bool
	}{
		{intLiteral(math.MinInt64, foundation.NonNull).Value(), intLiteral(math.MaxInt64, foundation.NonNull).Value(), false},
		{intLiteral(-1, foundation.NonNull).Value(), intLiteral(-1, foundation.NonNull).Value(), true},
		{must(testbedvalues.NewVarChar(text, "tsa_01")), must(testbedvalues.NewVarChar(text, "tsa_01")), true},
		{must(testbedvalues.NewVarChar(text, "A")), must(testbedvalues.NewVarChar(text, "a")), false},
	}
	for _, a := range []bool{false, true} {
		for _, b := range []bool{false, true} {
			cases = append(cases, struct {
				left, right valueinterfaces.Value
				equal       bool
			}{must(testbedvalues.NewBool(boolean, a)), must(testbedvalues.NewBool(boolean, b)), a == b})
		}
	}
	for _, tc := range cases {
		got := must(interpreter.EvaluateMembershipEquality(provider, tc.left, tc.right))
		if got.IsNull() || got.(*testbedvalues.BoolValue).Bool() != tc.equal || got.Type().Nullability() != foundation.NonNull {
			t.Fatal("wrong equality")
		}
		nullable := must(tc.left.WithNullability(foundation.MayNull))
		got = must(interpreter.EvaluateMembershipEquality(provider, nullable, tc.right))
		if got.IsNull() || got.(*testbedvalues.BoolValue).Bool() != tc.equal || got.Type().Nullability() != foundation.MayNull {
			t.Fatal("nullable payload equality wrong")
		}
		null := must(testbedvalues.NewNull(nullable.Type()))
		for _, pair := range [][2]valueinterfaces.Value{{null, tc.right}, {tc.right, null}, {null, null}} {
			if got := must(interpreter.EvaluateMembershipEquality(provider, pair[0], pair[1])); !got.IsNull() {
				t.Fatal("NULL equality became structural equality")
			}
		}
	}
}

func TestMembershipEqualityUnsupportedAndErrors(t *testing.T) {
	provider := must(testbed.NewStandardContext())
	integer := intLiteral(1, foundation.NonNull).Value()
	narrow := must(testbedvalues.NewInteger(must(types.NewIntType(types.Int8, foundation.NonNull)), 1))
	bound := uint64(8)
	text := must(testbedvalues.NewVarChar(must(types.NewVarCharType(&bound, foundation.NonNull)), "x"))
	array := must(testbedvalues.NewArray(must(types.NewArrayType(integer.Type(), foundation.NonNull)), []valueinterfaces.Value{integer}))
	for _, pair := range [][2]valueinterfaces.Value{{integer, narrow}, {narrow, narrow}, {text, text}, {array, array}} {
		if _, err := interpreter.EvaluateMembershipEquality(provider, pair[0], pair[1]); !errors.Is(err, functions.ErrUnsupportedLookup) {
			t.Fatal("unsupported domain coerced", err)
		}
	}
}
