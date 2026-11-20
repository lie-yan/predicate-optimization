package internal_test

import (
	"math"
	"strconv"
	"testing"

	valuesetinternal "github.com/lie-yan/predicate-optimization/analysis/value_set/internal"
	"github.com/lie-yan/predicate-optimization/foundation"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestFormatBoolAndInteger(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.NonNull))
	for _, tc := range []struct {
		value bool
		want  string
	}{{false, "false"}, {true, "true"}} {
		if got := valuesetinternal.FormatValue(must(testbedvalues.NewBool(boolean, tc.value))); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
	for _, tc := range []struct {
		width     types.IntWidth
		min, max  int64
		low, high string
	}{
		{types.Int8, -128, 127, "-128", "127"},
		{types.Int16, -32768, 32767, "-32768", "32767"},
		{types.Int32, -2147483648, 2147483647, "-2147483648", "2147483647"},
		{types.Int64, math.MinInt64, math.MaxInt64, "-9223372036854775808", "9223372036854775807"},
	} {
		typ := must(types.NewIntType(tc.width, foundation.NonNull))
		for _, example := range []struct {
			value int64
			want  string
		}{{tc.min, tc.low}, {0, "0"}, {tc.max, tc.high}} {
			if got := valuesetinternal.FormatValue(must(testbedvalues.NewInteger(typ, example.value))); got != example.want {
				t.Fatalf("%s: got %q, want %q", typ, got, example.want)
			}
		}
	}
}

func TestFormatFloat(t *testing.T) {
	type testCase struct {
		value float64
		want  string
	}
	for _, width := range []types.FloatWidth{types.Float32, types.Float64} {
		typ := must(types.NewFloatType(width, foundation.NonNull))
		cases := []testCase{
			{0, "0"}, {math.Copysign(0, -1), "0"}, {-1.5, "-1.5"}, {math.Inf(-1), "-Inf"}, {math.Inf(1), "+Inf"}, {math.NaN(), "NaN"},
		}
		if width == types.Float32 {
			cases = append(cases, testCase{float64(float32(0.1)), "0.1"}, testCase{math.SmallestNonzeroFloat32, "1e-45"}, testCase{math.MaxFloat32, "3.4028235e+38"})
		} else {
			cases = append(cases, testCase{0.1, "0.1"}, testCase{math.SmallestNonzeroFloat64, "5e-324"}, testCase{math.MaxFloat64, "1.7976931348623157e+308"})
		}
		for _, tc := range cases {
			value := must(testbedvalues.NewFloat(typ, tc.value))
			before := math.Float64bits(value.Float())
			got := valuesetinternal.FormatValue(value)
			if got != tc.want {
				t.Fatalf("%s: got %q, want %q", typ, got, tc.want)
			}
			if value.Type() != typ || math.Float64bits(value.Float()) != before {
				t.Fatal("formatting mutated Type or payload")
			}
			parsed := must(strconv.ParseFloat(got, int(width)))
			if math.IsNaN(tc.value) {
				if !math.IsNaN(parsed) {
					t.Fatal("NaN round trip failed")
				}
			} else if parsed != tc.value {
				t.Fatalf("float round trip: %v != %v", parsed, tc.value)
			}
		}
	}
}

func TestFormatVarChar(t *testing.T) {
	typ := must(types.NewVarCharType(nil, foundation.NonNull))
	for _, tc := range []struct{ value, want string }{
		{"", `""`},
		{"tsa_01", `"tsa_01"`},
		{"NaN", `"NaN"`},
		{"a\"b\\c", `"a\"b\\c"`},
		{"\x00\n\r\t", `"\x00\n\r\t"`},
		{"\xff\x80", `"\xff\x80"`},
		{"你好", `"你好"`},
	} {
		value := must(testbedvalues.NewVarChar(typ, tc.value))
		got := valuesetinternal.FormatValue(value)
		if got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
		if must(strconv.Unquote(got)) != tc.value {
			t.Fatal("string bytes did not round trip")
		}
		if value.Type() != typ || value.VarChar() != tc.value {
			t.Fatal("formatting mutated value")
		}
	}
	// A concrete non-null payload can have a nullable declared Type.
	nullable := must(types.NewVarCharType(nil, foundation.MayNull))
	if got := valuesetinternal.FormatValue(must(testbedvalues.NewVarChar(nullable, "a"))); got != `"a"` {
		t.Fatal(got)
	}
}

func TestFormatValuePreconditions(t *testing.T) {
	boolean := must(types.NewBoolType(foundation.MayNull))
	array := must(types.NewArrayType(boolean, foundation.NonNull))
	for _, value := range []valueinterfaces.Value{nil, (*testbedvalues.IntegerValue)(nil), must(testbedvalues.NewNull(boolean)), must(testbedvalues.NewArray(array, nil))} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected assertion for unsupported input")
				}
			}()
			valuesetinternal.FormatValue(value)
		}()
	}
}
