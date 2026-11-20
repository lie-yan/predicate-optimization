package equatable_value_set

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// valueArray owns unique normalized Values. Orderable domains use ascending
// storage, binary search and merge operations; other domains use equality scans.
// Currently Boolean is the only unordered domain, with at most two entries.
// Arrays are immutable after construction. Binary methods require compatible
// element Types and ordering modes, as checked by EquatableValueSet.
type valueArray struct {
	values  []valueinterfaces.Value
	ordered bool
}

func newValueArray(factory valueinterfaces.ValueFactory, typ types.Type, input []valueinterfaces.Value) (valueArray, error) {
	result := valueArray{ordered: typ.IsOrderable()}
	if result.ordered {
		result.values = make([]valueinterfaces.Value, 0, len(input))
	} else {
		result.values = make([]valueinterfaces.Value, 0, 2)
	}
	for i, v := range input {
		normalized, err := normalizeSetElement(factory, typ, v)
		if err != nil {
			return valueArray{}, fmt.Errorf("element %d: %w", i, err)
		}
		if result.ordered || !result.contains(normalized) {
			result.values = append(result.values, normalized)
		}
	}
	if result.ordered {
		slices.SortFunc(result.values, compareOrderedValues)
		result.values = slices.CompactFunc(result.values, func(a, b valueinterfaces.Value) bool { return compareOrderedValues(a, b) == 0 })
		if len(result.values) < len(input) {
			// Retain only distinct entries, not a backing array sized for duplicates.
			result.values = slices.Clone(result.values)
		}
	}
	return result, nil
}

func (a valueArray) contains(v valueinterfaces.Value) bool {
	if a.ordered {
		_, found := slices.BinarySearchFunc(a.values, v, compareOrderedValues)
		return found
	}
	for _, existing := range a.values {
		if existing.EqualsTo(v) {
			return true
		}
	}
	return false
}

func (a valueArray) union(b valueArray) valueArray {
	result := valueArray{ordered: a.ordered, values: make([]valueinterfaces.Value, 0, len(a.values)+len(b.values))}
	if !a.ordered {
		result.values = append(result.values, a.values...)
		for _, v := range b.values {
			if !a.contains(v) {
				result.values = append(result.values, v)
			}
		}
		return result
	}
	i, j := 0, 0
	for i < len(a.values) && j < len(b.values) {
		switch c := compareOrderedValues(a.values[i], b.values[j]); {
		case c < 0:
			result.values = append(result.values, a.values[i])
			i++
		case c > 0:
			result.values = append(result.values, b.values[j])
			j++
		default:
			result.values = append(result.values, a.values[i])
			i++
			j++
		}
	}
	result.values = append(result.values, a.values[i:]...)
	result.values = append(result.values, b.values[j:]...)
	return result
}

func (a valueArray) intersect(b valueArray) valueArray {
	result := valueArray{ordered: a.ordered, values: make([]valueinterfaces.Value, 0, min(len(a.values), len(b.values)))}
	if !a.ordered {
		for _, v := range a.values {
			if b.contains(v) {
				result.values = append(result.values, v)
			}
		}
		return result
	}
	i, j := 0, 0
	for i < len(a.values) && j < len(b.values) {
		switch c := compareOrderedValues(a.values[i], b.values[j]); {
		case c < 0:
			i++
		case c > 0:
			j++
		default:
			result.values = append(result.values, a.values[i])
			i++
			j++
		}
	}
	return result
}

func (a valueArray) difference(b valueArray) valueArray {
	result := valueArray{ordered: a.ordered, values: make([]valueinterfaces.Value, 0, len(a.values))}
	if !a.ordered {
		for _, v := range a.values {
			if !b.contains(v) {
				result.values = append(result.values, v)
			}
		}
		return result
	}
	i, j := 0, 0
	for i < len(a.values) && j < len(b.values) {
		switch c := compareOrderedValues(a.values[i], b.values[j]); {
		case c < 0:
			result.values = append(result.values, a.values[i])
			i++
		case c > 0:
			j++
		default:
			i++
			j++
		}
	}
	result.values = append(result.values, a.values[i:]...)
	return result
}

// compareOrderedValues orders normalized, non-null Values of compatible Types.
// NaN follows all numeric values and compares equal to itself for storage only;
// this does not define SQL comparisons or permit NaN range endpoints. Signed
// zeros have already been normalized to positive zero by normalizeSetElement.
func compareOrderedValues(a, b valueinterfaces.Value) int {
	switch a.Type().(type) {
	case *types.IntType:
		left := a.(valueinterfaces.IntegerValue)
		return cmp.Compare(left.Integer(), b.(valueinterfaces.IntegerValue).Integer())
	case *types.VarCharType:
		left := a.(valueinterfaces.VarCharValue)
		return cmp.Compare(left.VarChar(), b.(valueinterfaces.VarCharValue).VarChar())
	case *types.FloatType:
		left := a.(valueinterfaces.FloatValue)
		x, y := left.Float(), b.(valueinterfaces.FloatValue).Float()
		if math.IsNaN(x) {
			if math.IsNaN(y) {
				return 0
			}
			return 1
		}
		if math.IsNaN(y) {
			return -1
		}
		return cmp.Compare(x, y)
	default:
		panic("value_set: unsupported ordered value")
	}
}
