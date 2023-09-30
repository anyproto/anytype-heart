package database

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type testGetter map[string]*types.Value

func (m testGetter) Get(key string) *types.Value {
	return m[key]
}

func TestEq_FilterObject(t *testing.T) {
	t.Run("eq", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := testGetter{"k": pbtypes.String("equal test")}
			assert.True(t, eq.FilterObject(g))
		})
		t.Run("list ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := testGetter{"k": pbtypes.StringList([]string{"11", "equal test", "other"})}
			assert.True(t, eq.FilterObject(g))
		})
		t.Run("not ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := testGetter{"k": pbtypes.String("not equal test")}
			assert.False(t, eq.FilterObject(g))
		})
		t.Run("not ok list", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := testGetter{"k": pbtypes.StringList([]string{"11", "not equal test", "other"})}
			assert.False(t, eq.FilterObject(g))
		})
	})
	t.Run("gt", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_Greater}
			g := testGetter{"k": pbtypes.Float64(2)}
			assert.True(t, eq.FilterObject(g))
		})
		t.Run("not ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_Greater}
			g := testGetter{"k": pbtypes.Float64(2)}
			assert.False(t, eq.FilterObject(g))
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_Greater}
			g := testGetter{"k": pbtypes.Float64(1)}
			assert.False(t, eq.FilterObject(g))
		})
	})
	t.Run("gte", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := testGetter{"k": pbtypes.Float64(2)}
			assert.True(t, eq.FilterObject(g))
		})
		t.Run("ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := testGetter{"k": pbtypes.Float64(2)}
			assert.True(t, eq.FilterObject(g))
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := testGetter{"k": pbtypes.Float64(1)}
			assert.False(t, eq.FilterObject(g))
		})
	})
	t.Run("lt", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_Less}
			g := testGetter{"k": pbtypes.Float64(1)}
			assert.True(t, eq.FilterObject(g))
		})
		t.Run("not ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_Less}
			g := testGetter{"k": pbtypes.Float64(2)}
			assert.False(t, eq.FilterObject(g))
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_Less}
			g := testGetter{"k": pbtypes.Float64(2)}
			assert.False(t, eq.FilterObject(g))
		})
	})
	t.Run("lte", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := testGetter{"k": pbtypes.Float64(1)}
			assert.True(t, eq.FilterObject(g))
		})
		t.Run("ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(2), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := testGetter{"k": pbtypes.Float64(2)}
			assert.True(t, eq.FilterObject(g))
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := testGetter{"k": pbtypes.Float64(2)}
			assert.False(t, eq.FilterObject(g))
		})
	})
}

func TestNot_FilterObject(t *testing.T) {
	eq := FilterEq{Key: "k", Value: pbtypes.Float64(1), Cond: model.BlockContentDataviewFilter_Equal}
	g := testGetter{"k": pbtypes.Float64(1)}
	assert.True(t, eq.FilterObject(g))
	assert.False(t, FilterNot{eq}.FilterObject(g))
}

func TestIn_FilterObject(t *testing.T) {
	in := FilterIn{Key: "k", Value: pbtypes.StringList([]string{"1", "2", "3"}).GetListValue()}
	t.Run("ok list -> str", func(t *testing.T) {
		for _, v := range []string{"1", "2", "3"} {
			g := testGetter{"k": pbtypes.String(v)}
			assert.True(t, in.FilterObject(g))
		}
	})
	t.Run("not ok list -> str", func(t *testing.T) {
		g := testGetter{"k": pbtypes.String("not ok")}
		assert.False(t, in.FilterObject(g))
	})
	t.Run("ok list -> list", func(t *testing.T) {
		g := testGetter{"k": pbtypes.StringList([]string{"not ok", "1", "222"})}
		assert.True(t, in.FilterObject(g))
	})
	t.Run("not ok list -> list", func(t *testing.T) {
		g := testGetter{"k": pbtypes.StringList([]string{"not ok"})}
		assert.False(t, in.FilterObject(g))
	})
}

func TestLike_FilterObject(t *testing.T) {
	like := FilterLike{Key: "k", Value: pbtypes.String("sub")}
	t.Run("ok", func(t *testing.T) {
		g := testGetter{"k": pbtypes.String("with suBstr")}
		assert.True(t, like.FilterObject(g))
	})
	t.Run("not ok", func(t *testing.T) {
		g := testGetter{"k": pbtypes.String("with str")}
		assert.False(t, like.FilterObject(g))
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
		g := testGetter{"k": ev}
		assert.True(t, empty.FilterObject(g), ev)
	}

	var notEmptyVals = []*types.Value{
		pbtypes.String("1"),
		pbtypes.Bool(true),
		pbtypes.Float64(1),
		pbtypes.StringList([]string{"1"}),
	}
	for _, ev := range notEmptyVals {
		g := testGetter{"k": ev}
		assert.False(t, empty.FilterObject(g), ev)
	}
}

func TestAndFilters_FilterObject(t *testing.T) {
	and := FiltersAnd{
		FilterEq{Key: "k1", Value: pbtypes.String("v1"), Cond: model.BlockContentDataviewFilter_Equal},
		FilterEq{Key: "k2", Value: pbtypes.String("v2"), Cond: model.BlockContentDataviewFilter_Equal},
	}
	t.Run("ok", func(t *testing.T) {
		g := testGetter{"k1": pbtypes.String("v1"), "k2": pbtypes.String("v2")}
		assert.True(t, and.FilterObject(g))
	})
	t.Run("not ok", func(t *testing.T) {
		g := testGetter{"k1": pbtypes.String("v1"), "k2": pbtypes.String("v3")}
		assert.False(t, and.FilterObject(g))
	})
	t.Run("not ok all", func(t *testing.T) {
		g := testGetter{"k2": pbtypes.String("v3")}
		assert.False(t, and.FilterObject(g))
	})
}

func TestOrFilters_FilterObject(t *testing.T) {
	or := FiltersOr{
		FilterEq{Key: "k1", Value: pbtypes.String("v1"), Cond: model.BlockContentDataviewFilter_Equal},
		FilterEq{Key: "k2", Value: pbtypes.String("v2"), Cond: model.BlockContentDataviewFilter_Equal},
	}
	t.Run("ok all", func(t *testing.T) {
		g := testGetter{"k1": pbtypes.String("v1"), "k2": pbtypes.String("v2")}
		assert.True(t, or.FilterObject(g))
	})
	t.Run("ok", func(t *testing.T) {
		g := testGetter{"k1": pbtypes.String("v1"), "k2": pbtypes.String("v3")}
		assert.True(t, or.FilterObject(g))
	})
	t.Run("not ok all", func(t *testing.T) {
		g := testGetter{"k2": pbtypes.String("v3")}
		assert.False(t, or.FilterObject(g))
	})
}

func TestAllIn_FilterObject(t *testing.T) {
	allIn := FilterAllIn{Key: "k", Value: pbtypes.StringList([]string{"1", "2", "3"}).GetListValue()}
	t.Run("ok", func(t *testing.T) {
		g := testGetter{"k": pbtypes.StringList([]string{"2", "1", "3", "4"})}
		assert.True(t, allIn.FilterObject(g))
	})
	t.Run("not ok", func(t *testing.T) {
		g := testGetter{"k": pbtypes.StringList([]string{"2", "3", "4"})}
		assert.False(t, allIn.FilterObject(g))
	})

	t.Run("ok string in Object", func(t *testing.T) {
		allIn := FilterAllIn{Key: "k", Value: pbtypes.StringList([]string{"1"}).GetListValue()}
		g := testGetter{"k": pbtypes.String("1")}
		assert.True(t, allIn.FilterObject(g))
	})

	t.Run("ok string in Filter", func(t *testing.T) {
		v, err := pbtypes.ValueListWrapper(pbtypes.String("1"))
		assert.NoError(t, err)

		allIn := FilterAllIn{Key: "k", Value: v}

		g := testGetter{"k": pbtypes.StringList([]string{"1", "2", "3"})}
		assert.True(t, allIn.FilterObject(g))
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

		g := testGetter{"b": pbtypes.Bool(false)}
		assert.True(t, f.FilterObject(g))

		g = testGetter{"not_exists": pbtypes.Bool(false)}
		assert.True(t, f.FilterObject(g))

		g = testGetter{"b": pbtypes.Bool(true)}
		assert.False(t, f.FilterObject(g))
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

		g := testGetter{"b": pbtypes.Bool(false)}
		assert.False(t, f.FilterObject(g))

		g = testGetter{"not_exists": pbtypes.Bool(false)}
		assert.False(t, f.FilterObject(g))

		g = testGetter{"b": pbtypes.Bool(true)}
		assert.True(t, f.FilterObject(g))
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

		assert.True(t, f.FilterObject(testGetter{"type": pbtypes.String("id1")}))
		assert.True(t, f.FilterObject(testGetter{"type": pbtypes.StringList([]string{"id2", "id1"})}))
	})

}
