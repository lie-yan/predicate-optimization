package touchstone

import (
	"fmt"

	shared "github.com/lie-yan/predicate-optimization/expressions"
	"github.com/lie-yan/predicate-optimization/foundation"
	"github.com/lie-yan/predicate-optimization/functions"
	ids "github.com/lie-yan/predicate-optimization/testbed/expressions"
	testfunctions "github.com/lie-yan/predicate-optimization/testbed/functions"
	testbedvalues "github.com/lie-yan/predicate-optimization/testbed/values"
	"github.com/lie-yan/predicate-optimization/types"
	valueinterfaces "github.com/lie-yan/predicate-optimization/values/interfaces"
)

// Touchstone holds independently assembled input and expected-output trees.
// Expected is a proposed replacement, not optimizer output;
// its equivalence requires non-nullable ArticleType and the admitted semantics.
type Touchstone struct {
	Context  *testfunctions.Context
	Columns  TouchstoneColumns
	Input    shared.Expression
	Expected shared.Expression
}

// TouchstoneColumns shares immutable field references across both trees.
type TouchstoneColumns struct {
	ArticleType *shared.ReferenceExpression
	IsReview    *shared.ReferenceExpression
	BrandID     *shared.ReferenceExpression
	TagIDs      *shared.ReferenceExpression
	CategoryIDs *shared.ReferenceExpression
	MallIDs     *shared.ReferenceExpression
}

// NewPrimaryTouchstone uses non-null article_type and nullable remaining fields.
func NewPrimaryTouchstone() (*Touchstone, error) {
	return NewTouchstone(foundation.NonNull, foundation.MayNull)
}

// NewTouchstone chooses article nullability independently of the other fields.
// Integer carriers are Int64, VARCHAR is unbounded, and array elements are
// non-null Int64. All literal candidates are non-null, regardless of field Type.
// MayNull article_type constructs the counterexample schema; it does not assert
// equivalence to the expected output.
func NewTouchstone(articleNullability, otherNullability foundation.Nullability) (*Touchstone, error) {
	provider, err := testfunctions.NewStandardContext()
	if err != nil {
		return nil, err
	}
	b := touchstoneBuilder{provider: provider}
	b.integer = b.intType(foundation.NonNull)
	b.text = b.textType(foundation.NonNull)
	b.array = b.arrayType(b.integer, foundation.NonNull)
	text := b.textType(articleNullability)
	number := b.intType(otherNullability)
	array := b.arrayType(b.integer, otherNullability)
	if b.err != nil {
		return nil, b.err
	}
	columns := TouchstoneColumns{
		ArticleType: b.reference("article_type", text),
		IsReview:    b.reference("is_review", number),
		BrandID:     b.reference("brand_id", number),
		TagIDs:      b.reference("tag_ids", array),
		CategoryIDs: b.reference("category_ids", array),
		MallIDs:     b.reference("mall_ids", array),
	}
	if b.err != nil {
		return nil, b.err
	}
	a := columns.ArticleType
	brandCandidates := []int64{8794001, 8794002, 8794003, 8794004, 8794005, 8794006, 8794007, 8794008, 8794009, 8794010, 8794011}
	input := b.and(
		b.in(a, b.strings("tsa_01", "tsa_02", "tsa_03")...),
		b.not(b.and(b.containsAny(columns.TagIDs, 8791001, 8791002), b.equal(a, b.string("tsa_04")))),
		b.in(columns.IsReview, b.integers(8795001, 8795002, 8795003, 8795004, 8795005)...),
		b.not(b.containsAny(columns.CategoryIDs, 8792001)),
		b.not(b.in(columns.BrandID, b.integers(brandCandidates...)...)),
		b.not(b.containsAny(columns.MallIDs, 8793001)),
		b.and(
			b.not(b.contains(columns.TagIDs, 8791003)),
			b.not(b.and(b.contains(columns.TagIDs, 8791004), b.in(a, b.strings("tsa_01", "tsa_05", "tsa_03", "tsa_04", "tsa_06", "tsa_02", "tsa_07", "tsa_08", "tsa_09", "tsa_10", "tsa_11", "tsa_12", "tsa_13", "tsa_14", "tsa_15")...))),
			b.not(b.and(b.contains(columns.TagIDs, 8791005), b.in(a, b.strings("tsa_03", "tsa_02", "tsa_01", "tsa_04")...))),
			b.not(b.contains(columns.TagIDs, 8791006)),
			b.not(b.and(b.contains(columns.TagIDs, 8791007), b.in(a, b.strings("tsa_03", "tsa_02", "tsa_01", "tsa_04")...))),
			b.not(b.and(b.contains(columns.TagIDs, 8791008), b.in(a, b.strings("tsa_03", "tsa_02", "tsa_01", "tsa_04")...))),
			b.not(b.contains(columns.TagIDs, 8791009)),
		),
	)
	// Assemble the documented output directly, without transforming Input.
	expected := b.and(
		b.in(a, b.strings("tsa_01", "tsa_02", "tsa_03")...),
		b.in(columns.IsReview, b.integers(8795001, 8795002, 8795003, 8795004, 8795005)...),
		b.not(b.contains(columns.CategoryIDs, 8792001)),
		b.not(b.in(columns.BrandID, b.integers(brandCandidates...)...)),
		b.not(b.contains(columns.MallIDs, 8793001)),
		b.not(b.containsAny(columns.TagIDs, 8791003, 8791004, 8791005, 8791006, 8791007, 8791008, 8791009)),
	)
	if b.err != nil {
		return nil, fmt.Errorf("touchstone construction: %w", b.err)
	}
	return &Touchstone{Context: provider, Columns: columns, Input: input, Expected: expected}, nil
}

// Builder helpers accumulate constructor errors; no partially built fixture escapes.
type touchstoneBuilder struct {
	provider *testfunctions.Context
	integer  *types.IntType
	text     *types.VarCharType
	array    *types.ArrayType
	err      error
}

func (b *touchstoneBuilder) intType(n foundation.Nullability) *types.IntType {
	if b.err != nil {
		return nil
	}
	v, err := types.NewIntType(types.Int64, n)
	b.err = err
	return v
}

func (b *touchstoneBuilder) textType(n foundation.Nullability) *types.VarCharType {
	if b.err != nil {
		return nil
	}
	v, err := types.NewVarCharType(nil, n)
	b.err = err
	return v
}

func (b *touchstoneBuilder) arrayType(element types.Type, n foundation.Nullability) *types.ArrayType {
	if b.err != nil {
		return nil
	}
	v, err := types.NewArrayType(element, n)
	b.err = err
	return v
}

func (b *touchstoneBuilder) reference(name string, typ types.Type) *shared.ReferenceExpression {
	if b.err != nil {
		return nil
	}
	id, err := ids.NewColumnID("touchstone", name)
	if err != nil {
		b.err = err
		return nil
	}
	ref, err := shared.NewReferenceExpression(id, typ, name)
	b.err = err
	return ref
}

func (b *touchstoneBuilder) literal(value valueinterfaces.Value, err error) shared.Expression {
	if b.err != nil {
		return nil
	}
	if err != nil {
		b.err = err
		return nil
	}
	node, err := shared.NewLiteralExpression(value)
	b.err = err
	return node
}

func (b *touchstoneBuilder) string(value string) shared.Expression {
	if b.err != nil {
		return nil
	}
	return b.literal(testbedvalues.NewVarChar(b.text, value))
}

func (b *touchstoneBuilder) strings(items ...string) []shared.Expression {
	result := make([]shared.Expression, len(items))
	for i, v := range items {
		result[i] = b.string(v)
	}
	return result
}

func (b *touchstoneBuilder) integers(items ...int64) []shared.Expression {
	result := make([]shared.Expression, len(items))
	for i, v := range items {
		if b.err != nil {
			return nil
		}
		result[i] = b.literal(testbedvalues.NewInteger(b.integer, v))
	}
	return result
}

func (b *touchstoneBuilder) in(target shared.Expression, candidates ...shared.Expression) shared.Expression {
	if b.err != nil {
		return nil
	}
	typ, err := types.NewBoolType(target.Type().Nullability())
	if err != nil {
		b.err = err
		return nil
	}
	node, err := shared.NewSpecialFormExpression(foundation.SpecialFormIn, typ, append([]shared.Expression{target}, candidates...))
	b.err = err
	return node
}

func (b *touchstoneBuilder) logical(op foundation.LogicalOperator, children ...shared.Expression) shared.Expression {
	if b.err != nil {
		return nil
	}
	node, err := shared.NewLogicalExpression(op, children)
	b.err = err
	return node
}

func (b *touchstoneBuilder) and(children ...shared.Expression) shared.Expression {
	return b.logical(foundation.And, children...)
}

func (b *touchstoneBuilder) not(child shared.Expression) shared.Expression {
	return b.logical(foundation.Not, child)
}

func (b *touchstoneBuilder) equal(left, right shared.Expression) shared.Expression {
	if b.err != nil {
		return nil
	}
	h, err := b.provider.ComparisonFunction(foundation.Equal, left.Type(), right.Type())
	if err != nil {
		b.err = err
		return nil
	}
	node, err := shared.NewCallExpression(b.provider, h, []shared.Expression{left, right})
	b.err = err
	return node
}

func (b *touchstoneBuilder) contains(array shared.Expression, item int64) shared.Expression {
	args := b.integers(item)
	if b.err != nil {
		return nil
	}
	return b.call(functions.BuiltInArrayContains, array, args[0])
}

func (b *touchstoneBuilder) containsAny(array shared.Expression, items ...int64) shared.Expression {
	if b.err != nil {
		return nil
	}
	elements := make([]valueinterfaces.Value, len(items))
	for i, v := range items {
		value, err := testbedvalues.NewInteger(b.integer, v)
		if err != nil {
			b.err = err
			return nil
		}
		elements[i] = value
	}
	candidate := b.literal(testbedvalues.NewArray(b.array, elements))
	return b.call(functions.BuiltInArrayContainsAny, array, candidate)
}

func (b *touchstoneBuilder) call(name functions.BuiltInFunctionName, left, right shared.Expression) shared.Expression {
	if b.err != nil {
		return nil
	}
	h, err := b.provider.LookupBuiltInFunction(name, []types.Type{left.Type(), right.Type()})
	if err != nil {
		b.err = err
		return nil
	}
	node, err := shared.NewCallExpression(b.provider, h, []shared.Expression{left, right})
	b.err = err
	return node
}
