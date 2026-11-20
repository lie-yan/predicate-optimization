package expressions_test

import (
	"math"
	"testing"

	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// diagnosticValue is a caller-owned view exposing every payload accessor. Its
// source supplies storage and equality; only the accessor selected by semantic
// Type is valid. The slice makes the view non-comparable as a Go value.
type diagnosticValue struct {
	source valueinterfaces.Value
	marker []byte
}

func (v diagnosticValue) Type() types.Type { return v.source.Type() }
func (v diagnosticValue) IsNull() bool     { return v.source.IsNull() }
func (v diagnosticValue) EqualsTo(other valueinterfaces.Value) bool {
	return v.source.EqualsTo(other)
}

func (v diagnosticValue) WithNullability(n foundation.Nullability) (valueinterfaces.Value, error) {
	source, err := v.source.WithNullability(n)
	if err != nil {
		return nil, err
	}
	return diagnosticValue{source: source}, nil
}

func (v diagnosticValue) Bool() bool {
	return v.source.(valueinterfaces.BoolValue).Bool()
}

func (v diagnosticValue) Integer() int64 {
	return v.source.(valueinterfaces.IntegerValue).Integer()
}

func (v diagnosticValue) Float() float64 {
	return v.source.(valueinterfaces.FloatValue).Float()
}

func (v diagnosticValue) VarChar() string {
	return v.source.(valueinterfaces.VarCharValue).VarChar()
}

func (v diagnosticValue) ElementCount() int {
	return v.source.(valueinterfaces.ArrayValue).ElementCount()
}

func (v diagnosticValue) ElementAt(index int) valueinterfaces.Value {
	return diagnosticValue{source: v.source.(valueinterfaces.ArrayValue).ElementAt(index)}
}

func (v diagnosticValue) Elements() []valueinterfaces.Value {
	children := v.source.(valueinterfaces.ArrayValue).Elements()
	for i, child := range children {
		children[i] = diagnosticValue{source: child}
	}
	return children
}

func (v diagnosticValue) Fields() []valueinterfaces.Value {
	children := v.source.(valueinterfaces.StructValue).Fields()
	for i, child := range children {
		children[i] = diagnosticValue{source: child}
	}
	return children
}

func TestLiteralExternalValueEqualityAndDiagnostics(t *testing.T) {
	integer := fixture(types.NewIntType(types.Int64, foundation.MayNull))
	boolean := fixture(types.NewBoolType(foundation.NonNull))
	f32 := fixture(types.NewFloatType(types.Float32, foundation.NonNull))
	f64 := fixture(types.NewFloatType(types.Float64, foundation.NonNull))
	text := fixture(types.NewVarCharType(nil, foundation.NonNull))
	array := fixture(types.NewArrayType(integer, foundation.MayNull))
	one := fixture(testbedvalues.NewInteger(integer, 1))
	null := fixture(testbedvalues.NewNull(integer))
	items := fixture(testbedvalues.NewArray(array, []valueinterfaces.Value{one, null, one}))
	record := fixture(types.NewStructType([]types.StructField{{Name: "items", Type: array}}, foundation.NonNull))
	for _, tc := range []struct {
		name  string
		value valueinterfaces.Value
	}{
		{"integer", one},
		{"null", null},
		{"bool", fixture(testbedvalues.NewBool(boolean, true))},
		{"float32", fixture(testbedvalues.NewFloat(f32, float64(float32(1.234567))))},
		{"float64", fixture(testbedvalues.NewFloat(f64, 1.23456789012345))},
		{"negative zero", fixture(testbedvalues.NewFloat(f64, math.Copysign(0, -1)))},
		{"NaN", fixture(testbedvalues.NewFloat(f64, math.NaN()))},
		{"infinity", fixture(testbedvalues.NewFloat(f64, math.Inf(-1)))},
		{"string bytes", fixture(testbedvalues.NewVarChar(text, "é\n\"\xff"))},
		{"array", items},
		{"empty array", fixture(testbedvalues.NewArray(array, nil))},
		{"null array", fixture(testbedvalues.NewNull(array))},
		{"nested struct", fixture(testbedvalues.NewStruct(record, []valueinterfaces.Value{items}))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundled := fixture(expressions.NewLiteralExpression(tc.value))
			external := fixture(expressions.NewLiteralExpression(diagnosticValue{source: tc.value}))
			wantString, wantPretty := bundled.String(), expressions.PrettyPrint(bundled)
			if external.String() != wantString || expressions.PrettyPrint(external) != wantPretty {
				t.Fatalf("representation changed diagnostics: %s / %s", external.String(), expressions.PrettyPrint(external))
			}
			if !external.EqualsTo(bundled) || !bundled.EqualsTo(external) || !external.EqualsTo(external) {
				t.Fatal("literal equality depends on value representation")
			}
			if bundled.String() != wantString || expressions.PrettyPrint(bundled) != wantPretty {
				t.Fatal("diagnostics mutated source storage")
			}
		})
	}
	other := fixture(expressions.NewLiteralExpression(diagnosticValue{source: fixture(testbedvalues.NewInteger(integer, 2))}))
	original := fixture(expressions.NewLiteralExpression(one))
	if original.EqualsTo(other) || other.EqualsTo(original) {
		t.Fatal("literal equality ignored differing payloads")
	}
}
