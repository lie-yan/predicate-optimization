package array_membership_normalization_rewriter

import (
	"github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// singletonCandidate inspects literal payloads only. Nullable input arrays are
// allowed; candidate containers and their elements must be non-nullable.
func singletonCandidate(arguments []expressions.Expression) foundation.Optional[valueinterfaces.Value] {
	absent := foundation.Optional[valueinterfaces.Value]{}
	if len(arguments) != 2 {
		return absent
	}
	input, ok := arguments[0].Type().(*types.ArrayType)
	if !ok || input.Element().Nullability() != foundation.NonNull {
		return absent
	}
	literal, ok := arguments[1].(*expressions.LiteralExpression)
	if !ok {
		return absent
	}
	typ, ok := literal.Type().(*types.ArrayType)
	if !ok || literal.Value().IsNull() {
		return absent
	}
	if typ.Nullability() != foundation.NonNull || !typ.Element().EqualsTo(input.Element()) {
		return absent
	}
	array := literal.Value().(valueinterfaces.ArrayValue)
	if array.ElementCount() != 1 {
		return absent
	}
	element := array.ElementAt(0)
	if element.IsNull() || !element.Type().EqualsTo(input.Element()) {
		return absent
	}
	return foundation.Some(element)
}
