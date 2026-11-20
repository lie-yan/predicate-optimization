// Package errors defines structured error classification and cause handling.
// Domain packages supply diagnostic payloads and defer formatting until Error.
// This package must not depend on domain packages such as functions or types.
package errors

// Code identifies a precise failure independently of its behavioral category.
// Values and symbolic names are stable; new codes receive explicit unused values.
// These identifiers do not define a wire serialization protocol.
type Code uint32

const (
	CodeUnknown Code = 0

	// Binding codes occupy 0x0100–0x01ff. Existing assignments must not be reused.
	CodeBindingArityMismatch         Code = 0x0101
	CodeBindingArgumentTypeMismatch  Code = 0x0102
	CodeBindingNullPolicyUnspecified Code = 0x0103
	CodeBindingResultNullability     Code = 0x0104
	CodeInvalidCallBinding           Code = 0x0105
	CodeBindingResultTypeMismatch    Code = 0x0106
	// Analysis diagnostic codes occupy 0x0200–0x02ff. Assignments are stable.
	CodeAnalysisComparisonUnsupported Code = 0x0201
	CodeAnalysisLogicalUnsupported    Code = 0x0202
	CodeAnalysisConstantOnly          Code = 0x0203
)

// String returns a static symbolic name, or "unknown" for unassigned codes.
func (c Code) String() string {
	switch c {
	case CodeBindingArityMismatch:
		return "binding_arity_mismatch"
	case CodeBindingArgumentTypeMismatch:
		return "binding_argument_type_mismatch"
	case CodeBindingNullPolicyUnspecified:
		return "binding_null_policy_unspecified"
	case CodeBindingResultNullability:
		return "binding_result_nullability"
	case CodeInvalidCallBinding:
		return "invalid_call_binding"
	case CodeBindingResultTypeMismatch:
		return "binding_result_type_mismatch"
	case CodeAnalysisComparisonUnsupported:
		return "analysis_comparison_unsupported"
	case CodeAnalysisLogicalUnsupported:
		return "analysis_logical_unsupported"
	case CodeAnalysisConstantOnly:
		return "analysis_constant_only"
	default:
		return "unknown"
	}
}
