package database

import (
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func assertFilter(t *testing.T, f Filter, obj *types.Struct, expected bool) {
	assert.Equal(t, expected, f.FilterObject(obj))
	compiled := f.Compile()
	arena := &fastjson.Arena{}
	val := pbtypes.ProtoToJson(arena, obj)
	ok := assert.Equal(t, expected, compiled.Ok(val))
	if !ok {
		fmt.Println()
		compiled.Ok(val)
	}
}

func TestEq_FilterObject(t *testing.T) {
	t.Run("eq", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("equal test")}}
			assertFilter(t, eq, g, true)
		})
		t.Run("list ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.StringList([]string{"11", "equal test", "other"})}}
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("not equal test")}}
			assertFilter(t, eq, g, false)
		})
		t.Run("not ok list", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.StringList([]string{"11", "not equal test", "other"})}}
			assertFilter(t, eq, g, false)
		})
	})
	t.Run("gt", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_Greater}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_Greater}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
			assertFilter(t, eq, g, false)
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_Greater}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
			assertFilter(t, eq, g, false)
		})
	})
	t.Run("gte", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
			assertFilter(t, eq, g, true)
		})
		t.Run("ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
			assertFilter(t, eq, g, false)
		})
	})
	t.Run("lt", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_Less}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_Less}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
			assertFilter(t, eq, g, false)
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_Less}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
			assertFilter(t, eq, g, false)
		})
	})
	t.Run("lte", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
			assertFilter(t, eq, g, true)
		})
		t.Run("ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
			assertFilter(t, eq, g, false)
		})
	})
}

func TestNot_FilterObject(t *testing.T) {
	eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_Equal}
	g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
	assertFilter(t, eq, g, true)
	assert.False(t, FilterNot{eq}.FilterObject(g))
}

func TestIn_FilterObject(t *testing.T) {
	in := FilterIn{Key: "k", Value: pbtypes.StringList([]string{"1", "2", "3"}).GetListValue()}
	t.Run("ok list -> str", func(t *testing.T) {
		for _, v := range []string{"1", "2", "3"} {
			g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String(v)}}
			assert.True(t, in.FilterObject(g))
		}
	})
	t.Run("not ok list -> str", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("not ok")}}
		assert.False(t, in.FilterObject(g))
	})
	t.Run("ok list -> list", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.StringList([]string{"not ok", "1", "222"})}}
		assert.True(t, in.FilterObject(g))
	})
	t.Run("not ok list -> list", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.StringList([]string{"not ok"})}}
		assert.False(t, in.FilterObject(g))
	})
}

func TestLike_FilterObject(t *testing.T) {
	like := FilterLike{Key: "k", Value: pbtypes.String("sub")}
	t.Run("ok", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("with suBstr")}}
		assertFilter(t, like, g, true)
	})
	t.Run("not ok", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("with str")}}
		assertFilter(t, like, g, false)
	})
}

func TestEmpty_FilterObject(t *testing.T) {
	empty := FilterEmpty{Key: "k"}
	var emptyVals = []*types.Value{
		pbtypes.String(""),
		pbtypes.Bool(false),
		pbtypes.Float64(0),
		nil,
		&types.Value{},
		&types.Value{Kind: &types.Value_NullValue{}},
		&types.Value{Kind: &types.Value_StructValue{}},
		pbtypes.StringList([]string{}),
	}
	for _, ev := range emptyVals {
		g := &types.Struct{Fields: map[string]*types.Value{"k": ev}}
		assertFilter(t, empty, g, true)
	}

	var notEmptyVals = []*types.Value{
		pbtypes.String("1"),
		pbtypes.Bool(true),
		pbtypes.Float64(1),
		pbtypes.StringList([]string{"1"}),
	}
	for _, ev := range notEmptyVals {
		g := &types.Struct{Fields: map[string]*types.Value{"k": ev}}
		assertFilter(t, empty, g, false)
	}
}

func TestAndFilters_FilterObject(t *testing.T) {
	and := FiltersAnd{
		FilterEq{Key: "k1", Value: pbtypes.String("v1"), Cond: model.BlockContentDataviewFilter_Equal},
		FilterEq{Key: "k2", Value: pbtypes.String("v2"), Cond: model.BlockContentDataviewFilter_Equal},
	}
	t.Run("ok", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k1": pbtypes.String("v1"), "k2": pbtypes.String("v2")}}
		assertFilter(t, and, g, true)
	})
	t.Run("not ok", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k1": pbtypes.String("v1"), "k2": pbtypes.String("v3")}}
		assertFilter(t, and, g, false)
	})
	t.Run("not ok all", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k2": pbtypes.String("v3")}}
		assertFilter(t, and, g, false)
	})
}

func TestOrFilters_FilterObject(t *testing.T) {
	or := FiltersOr{
		FilterEq{Key: "k1", Value: pbtypes.String("v1"), Cond: model.BlockContentDataviewFilter_Equal},
		FilterEq{Key: "k2", Value: pbtypes.String("v2"), Cond: model.BlockContentDataviewFilter_Equal},
	}
	t.Run("ok all", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k1": pbtypes.String("v1"), "k2": pbtypes.String("v2")}}
		assertFilter(t, or, g, true)
	})
	t.Run("ok", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k1": pbtypes.String("v1"), "k2": pbtypes.String("v3")}}
		assertFilter(t, or, g, true)
	})
	t.Run("not ok all", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k2": pbtypes.String("v3")}}
		assertFilter(t, or, g, false)
	})
}

func TestAllIn_FilterObject(t *testing.T) {
	allIn := FilterAllIn{Key: "k", Value: pbtypes.StringList([]string{"1", "2", "3"}).GetListValue()}
	t.Run("ok", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.StringList([]string{"2", "1", "3", "4"})}}
		assertFilter(t, allIn, g, true)
	})
	t.Run("not ok", func(t *testing.T) {
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.StringList([]string{"2", "3", "4"})}}
		assertFilter(t, allIn, g, false)
	})

	t.Run("ok string in Object", func(t *testing.T) {
		allIn := FilterAllIn{Key: "k", Value: pbtypes.StringList([]string{"1"}).GetListValue()}
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("1")}}
		assertFilter(t, allIn, g, true)
	})

	t.Run("ok string in Filter", func(t *testing.T) {
		v, err := pbtypes.ValueListWrapper(pbtypes.String("1"))
		assert.NoError(t, err)

		allIn := FilterAllIn{Key: "k", Value: v}
		g := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.StringList([]string{"1", "2", "3"})}}
		assertFilter(t, allIn, g, true)
	})
}

func TestMakeAndFilter(t *testing.T) {
	store := NewMockObjectStore(t)
	t.Run("valid", func(t *testing.T) {
		filters := []*model.BlockContentDataviewFilter{
			{
				RelationKey: "1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("1"),
			},
			{
				RelationKey: "2",
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String("2"),
			},
			{
				RelationKey: "3",
				Condition:   model.BlockContentDataviewFilter_Greater,
				Value:       pbtypes.String("3"),
			},
			{
				RelationKey: "4",
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
				Value:       pbtypes.String("4"),
			},
			{
				RelationKey: "5",
				Condition:   model.BlockContentDataviewFilter_Less,
				Value:       pbtypes.String("5"),
			},
			{
				RelationKey: "6",
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       pbtypes.String("6"),
			},
			{
				RelationKey: "7",
				Condition:   model.BlockContentDataviewFilter_Like,
				Value:       pbtypes.String("7"),
			},
			{
				RelationKey: "8",
				Condition:   model.BlockContentDataviewFilter_NotLike,
				Value:       pbtypes.String("8"),
			},
			{
				RelationKey: "9",
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList([]string{"9"}),
			},
			{
				RelationKey: "10",
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       pbtypes.StringList([]string{"10"}),
			},
			{
				RelationKey: "11",
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
			{
				RelationKey: "12",
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: "13",
				Condition:   model.BlockContentDataviewFilter_AllIn,
				Value:       pbtypes.StringList([]string{"13"}),
			},
			{
				RelationKey: "14",
				Condition:   model.BlockContentDataviewFilter_NotAllIn,
				Value:       pbtypes.StringList([]string{"14"}),
			},
		}
		andFilter, err := MakeFiltersAnd(filters, store)
		require.NoError(t, err)
		assert.Len(t, andFilter, 14)
	})
	t.Run("not valid list", func(t *testing.T) {
		for _, cond := range []model.BlockContentDataviewFilterCondition{
			model.BlockContentDataviewFilter_In,
			model.BlockContentDataviewFilter_NotIn,
			model.BlockContentDataviewFilter_AllIn,
			model.BlockContentDataviewFilter_NotAllIn,
		} {
			_, err := MakeFiltersAnd([]*model.BlockContentDataviewFilter{
				{Condition: cond, Value: pbtypes.Null()},
			}, store)
			assert.Equal(t, ErrValueMustBeListSupporting, err)
		}

	})
	t.Run("unexpected condition", func(t *testing.T) {
		_, err := MakeFiltersAnd([]*model.BlockContentDataviewFilter{
			{Condition: 10000},
		}, store)
		assert.Error(t, err)
	})
	t.Run("replace 'value == false' to 'value != true'", func(t *testing.T) {
		f, err := MakeFiltersAnd([]*model.BlockContentDataviewFilter{
			{
				RelationKey: "b",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(false),
			},
		}, store)
		require.NoError(t, err)

		g := &types.Struct{Fields: map[string]*types.Value{"b": pbtypes.Bool(false)}}
		assertFilter(t, f, g, true)

		g = &types.Struct{Fields: map[string]*types.Value{"not_exists": pbtypes.Bool(false)}}
		assertFilter(t, f, g, true)

		g = &types.Struct{Fields: map[string]*types.Value{"b": pbtypes.Bool(true)}}
		assertFilter(t, f, g, false)
	})
	t.Run("replace 'value != false' to 'value == true'", func(t *testing.T) {
		f, err := MakeFiltersAnd([]*model.BlockContentDataviewFilter{
			{
				RelationKey: "b",
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(false),
			},
		}, store)
		require.NoError(t, err)

		g := &types.Struct{Fields: map[string]*types.Value{"b": pbtypes.Bool(false)}}
		assertFilter(t, f, g, false)

		g = &types.Struct{Fields: map[string]*types.Value{"not_exists": pbtypes.Bool(false)}}
		assertFilter(t, f, g, false)

		g = &types.Struct{Fields: map[string]*types.Value{"b": pbtypes.Bool(true)}}
		assertFilter(t, f, g, true)
	})
}

func TestNestedFilters(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		store := NewMockObjectStore(t)
		// Query will occur while nested filter resolving
		store.EXPECT().QueryRaw(mock.Anything, 0, 0).Return([]Record{
			{
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String(): pbtypes.String("id1"),
						"typeKey":                     pbtypes.String("note"),
					},
				},
			},
			{
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String(): pbtypes.String("id2"),
						"typeKey":                     pbtypes.String("note"),
					},
				},
			},
		}, nil)

		f, err := MakeFilter("", &model.BlockContentDataviewFilter{
			RelationKey: "type.typeKey",
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String("note"),
		}, store)
		require.NoError(t, err)

		obj1 := &types.Struct{Fields: map[string]*types.Value{"type": pbtypes.String("id1")}}
		obj2 := &types.Struct{Fields: map[string]*types.Value{"type": pbtypes.StringList([]string{"id2", "id1"})}}
		assertFilter(t, f, obj1, true)
		assertFilter(t, f, obj2, true)
	})

}
