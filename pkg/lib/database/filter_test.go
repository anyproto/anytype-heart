package database

import (
	"testing"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/syncpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

func assertFilter(t *testing.T, f Filter, obj *domain.Details, expected bool) {
	assert.Equal(t, expected, f.FilterObject(obj))
	anystoreFilter := f.AnystoreFilter()
	arena := &anyenc.Arena{}
	val := obj.ToAnyEnc(arena)
	docBuf := &syncpool.DocBuffer{}
	result := anystoreFilter.Ok(val, docBuf)
	assert.Equal(t, expected, result)
}

func TestEq_FilterObject(t *testing.T) {
	t.Run("eq", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("equal test")})
			assertFilter(t, eq, g, true)
		})
		t.Run("list ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"11", "equal test", "other"})})
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("not equal test")})
			assertFilter(t, eq, g, false)
		})
		t.Run("not ok list", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.String("equal test"), Cond: model.BlockContentDataviewFilter_Equal}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"11", "not equal test", "other"})})
			assertFilter(t, eq, g, false)
		})
	})
	t.Run("gt", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(1), Cond: model.BlockContentDataviewFilter_Greater}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_Greater}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
			assertFilter(t, eq, g, false)
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_Greater}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
			assertFilter(t, eq, g, false)

		})
	})
	t.Run("gte", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(1), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
			assertFilter(t, eq, g, true)
		})
		t.Run("ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_GreaterOrEqual}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
			assertFilter(t, eq, g, false)
		})
	})
	t.Run("lt", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_Less}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_Less}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
			assertFilter(t, eq, g, false)
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(1), Cond: model.BlockContentDataviewFilter_Less}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
			assertFilter(t, eq, g, false)
		})
	})
	t.Run("lte", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
			assertFilter(t, eq, g, true)
		})
		t.Run("ok eq", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
			assertFilter(t, eq, g, true)
		})
		t.Run("not ok less", func(t *testing.T) {
			eq := FilterEq{Key: "k", Value: domain.Float64(1), Cond: model.BlockContentDataviewFilter_LessOrEqual}
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
			assertFilter(t, eq, g, false)
		})
	})
	t.Run("not equal", func(t *testing.T) {
		eq := FilterEq{Key: "k", Value: domain.Float64(2), Cond: model.BlockContentDataviewFilter_NotEqual}
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		assertFilter(t, eq, obj, true)

		obj = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
		assertFilter(t, eq, obj, false)
	})

	t.Run("not equal true: no key", func(t *testing.T) {
		eq := FilterEq{Key: "k", Value: domain.Bool(true), Cond: model.BlockContentDataviewFilter_NotEqual}
		obj := domain.NewDetails()
		assertFilter(t, eq, obj, true)

		obj = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Bool(true)})
		assertFilter(t, eq, obj, false)
	})
	t.Run("not equal false: no key", func(t *testing.T) {
		eq := FilterEq{Key: "k", Value: domain.Bool(false), Cond: model.BlockContentDataviewFilter_NotEqual}
		obj := domain.NewDetails()
		assertFilter(t, eq, obj, true)

		obj = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Bool(false)})
		assertFilter(t, eq, obj, false)
	})
}

func TestNot_FilterObject(t *testing.T) {
	eq := FilterEq{Key: "k", Value: domain.Float64(1), Cond: model.BlockContentDataviewFilter_Equal}
	g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
	assertFilter(t, eq, g, true)
	assertFilter(t, FilterNot{eq}, g, false)
}

func TestIn_FilterObject(t *testing.T) {
	in := FilterIn{Key: "k", Value: domain.StringList([]string{"1", "2", "3"}).WrapToList()}
	t.Run("ok list -> str", func(t *testing.T) {
		for _, v := range []string{"1", "2", "3"} {
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String(v)})
			assertFilter(t, in, g, true)
		}
	})
	t.Run("not ok list -> str", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("not ok")})
		assertFilter(t, in, g, false)
	})
	t.Run("ok list -> list", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"not ok", "1", "222"})})
		assertFilter(t, in, g, true)
	})
	t.Run("not ok list -> list", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"not ok"})})
		assertFilter(t, in, g, false)
	})

	t.Run("not in", func(t *testing.T) {
		f := FilterNot{FilterIn{Key: "k", Value: domain.StringList([]string{"1", "2", "3"}).WrapToList()}}
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("4")})
		assertFilter(t, f, obj, true)

		obj = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("1")})
		assertFilter(t, f, obj, false)
	})
}

func TestLike_FilterObject(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		like := FilterLike{Key: "k", Value: "sub"}
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("with suBstr")})
		assertFilter(t, like, g, true)
	})
	t.Run("not ok", func(t *testing.T) {
		like := FilterLike{Key: "k", Value: "sub"}
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("with str")})
		assertFilter(t, like, g, false)
	})
	t.Run("escape regexp", func(t *testing.T) {
		like := FilterLike{Key: "k", Value: "[abc]"}
		t.Run("ok", func(t *testing.T) {

			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("[abc]")})
			assertFilter(t, like, g, true)
		})
		t.Run("not ok", func(t *testing.T) {
			g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
			assertFilter(t, like, g, false)
		})
	})
}

func TestEmpty_FilterObject(t *testing.T) {
	empty := FilterEmpty{Key: "k"}
	var emptyVals = []domain.Value{
		domain.String(""),
		domain.Bool(false),
		domain.Float64(0),
		domain.Invalid(),
		domain.StringList([]string{}),
		domain.Float64List(nil),
		domain.Null(),
	}
	for _, ev := range emptyVals {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": ev})
		assertFilter(t, empty, g, true)
	}

	var notEmptyVals = []domain.Value{
		domain.String("1"),
		domain.Bool(true),
		domain.Float64(1),
		domain.StringList([]string{"1"}),
		domain.Float64List([]float64{1}),
	}
	for _, ev := range notEmptyVals {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": ev})
		assertFilter(t, empty, g, false)
	}
}

func TestAndFilters_FilterObject(t *testing.T) {
	and := FiltersAnd{
		FilterEq{Key: "k1", Value: domain.String("v1"), Cond: model.BlockContentDataviewFilter_Equal},
		FilterEq{Key: "k2", Value: domain.String("v2"), Cond: model.BlockContentDataviewFilter_Equal},
	}
	t.Run("ok", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k1": domain.String("v1"), "k2": domain.String("v2")})
		assertFilter(t, and, g, true)
	})
	t.Run("not ok", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k1": domain.String("v1"), "k2": domain.String("v3")})
		assertFilter(t, and, g, false)
	})
	t.Run("not ok all", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k2": domain.String("v3")})
		assertFilter(t, and, g, false)
	})
}

func TestOrFilters_FilterObject(t *testing.T) {
	or := FiltersOr{
		FilterEq{Key: "k1", Value: domain.String("v1"), Cond: model.BlockContentDataviewFilter_Equal},
		FilterEq{Key: "k2", Value: domain.String("v2"), Cond: model.BlockContentDataviewFilter_Equal},
	}
	t.Run("ok all", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k1": domain.String("v1"), "k2": domain.String("v2")})
		assertFilter(t, or, g, true)
	})
	t.Run("ok", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k1": domain.String("v1"), "k2": domain.String("v3")})
		assertFilter(t, or, g, true)
	})
	t.Run("not ok all", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k2": domain.String("v3")})
		assertFilter(t, or, g, false)
	})
}

func TestAllIn_FilterObject(t *testing.T) {
	allIn := FilterAllIn{Key: "k", Strings: []string{"1", "2", "3"}}
	t.Run("ok", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"2", "1", "3", "4"})})
		assertFilter(t, allIn, g, true)
	})
	t.Run("not ok", func(t *testing.T) {
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"2", "3", "4"})})
		assertFilter(t, allIn, g, false)
	})

	t.Run("ok string in Object", func(t *testing.T) {
		allIn := FilterAllIn{Key: "k", Strings: []string{"1"}}
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("1")})
		assertFilter(t, allIn, g, true)
	})

	t.Run("ok string in Filter", func(t *testing.T) {
		allIn := FilterAllIn{Key: "k", Strings: []string{"1"}}
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"1", "2", "3"})})
		assertFilter(t, allIn, g, true)
	})

	t.Run("not all in", func(t *testing.T) {
		f := FilterNot{FilterAllIn{Key: "k", Strings: []string{"1", "2"}}}

		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"1", "3"})})
		assertFilter(t, f, obj, true)

		obj = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"1", "2", "3"})})
		assertFilter(t, f, obj, false)
	})
}

func TestMakeAndFilter(t *testing.T) {
	store := &stubSpaceObjectStore{}
	t.Run("valid", func(t *testing.T) {
		filters := []FilterRequest{
			{
				RelationKey: "1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String("1"),
			},
			{
				RelationKey: "2",
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.String("2"),
			},
			{
				RelationKey: "3",
				Condition:   model.BlockContentDataviewFilter_Greater,
				Value:       domain.String("3"),
			},
			{
				RelationKey: "4",
				Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
				Value:       domain.String("4"),
			},
			{
				RelationKey: "5",
				Condition:   model.BlockContentDataviewFilter_Less,
				Value:       domain.String("5"),
			},
			{
				RelationKey: "6",
				Condition:   model.BlockContentDataviewFilter_LessOrEqual,
				Value:       domain.String("6"),
			},
			{
				RelationKey: "7",
				Condition:   model.BlockContentDataviewFilter_Like,
				Value:       domain.String("7"),
			},
			{
				RelationKey: "8",
				Condition:   model.BlockContentDataviewFilter_NotLike,
				Value:       domain.String("8"),
			},
			{
				RelationKey: "9",
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList([]string{"9"}),
			},
			{
				RelationKey: "10",
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.StringList([]string{"10"}),
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
				Value:       domain.StringList([]string{"13"}),
			},
			{
				RelationKey: "14",
				Condition:   model.BlockContentDataviewFilter_NotAllIn,
				Value:       domain.StringList([]string{"14"}),
			},
		}
		andFilter, err := MakeFilters(filters, store)
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
			_, err := MakeFilters([]FilterRequest{
				{Condition: cond, Value: domain.Null()},
			}, store)
			assert.Error(t, err)
		}

	})
	t.Run("unexpected condition", func(t *testing.T) {
		_, err := MakeFilters([]FilterRequest{
			{Condition: 10000},
		}, store)
		assert.Error(t, err)
	})
	t.Run("replace 'value == false' to 'value != true'", func(t *testing.T) {
		f, err := MakeFilters([]FilterRequest{
			{
				RelationKey: "b",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(false),
			},
		}, store)
		require.NoError(t, err)

		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"b": domain.Bool(false)})
		assertFilter(t, f, g, true)

		g = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"not_exists": domain.Bool(false)})
		assertFilter(t, f, g, true)

		g = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"b": domain.Bool(true)})
		assertFilter(t, f, g, false)
	})
	t.Run("replace 'value != false' to 'value == true'", func(t *testing.T) {
		f, err := MakeFilters([]FilterRequest{
			{
				RelationKey: "b",
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(false),
			},
		}, store)
		require.NoError(t, err)

		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"b": domain.Bool(false)})
		assertFilter(t, f, g, false)

		g = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"not_exists": domain.Bool(false)})
		assertFilter(t, f, g, false)

		g = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"b": domain.Bool(true)})
		assertFilter(t, f, g, true)
	})
}

func TestNestedFilters(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		store := &stubSpaceObjectStore{}
		// Query will occur while nested filter resolving
		store.queryRawResult = []Record{
			{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId: domain.String("id1"),
					"typeKey":            domain.String("note"),
				}),
			},
			{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId: domain.String("id2"),
					"typeKey":            domain.String("note"),
				}),
			},
		}

		f, err := MakeFilter("spaceId", FilterRequest{
			RelationKey: "type.typeKey",
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String("note"),
		}, store)
		require.NoError(t, err)

		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"type": domain.String("id1")})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"type": domain.StringList([]string{"id2", "id1"})})
		assertFilter(t, f, obj1, true)
		assertFilter(t, f, obj2, true)
	})

	t.Run("not equal", func(t *testing.T) {
		store := &stubSpaceObjectStore{
			queryRawResult: []Record{
				{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyId:        domain.String("id1"),
						bundle.RelationKeyUniqueKey: domain.String("ot-note"),
					}),
				},
				{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyId:        domain.String("id2"),
						bundle.RelationKeyUniqueKey: domain.String("ot-note"),
					}),
				},
			},
		}
		// Query will occur while nested filter resolving

		f, err := MakeFilter("spaceId", FilterRequest{
			RelationKey: "type.uniqueKey",
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.String("ot-note"),
		}, store)
		require.NoError(t, err)

		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyType: domain.StringList([]string{"id1"})})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyType: domain.StringList([]string{"id2", "id1"})})
		obj3 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyType: domain.StringList([]string{"id3"})})
		obj4 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyType: domain.StringList([]string{"id4", "id5"})})
		assertFilter(t, f, obj1, false)
		assertFilter(t, f, obj2, false)
		assertFilter(t, f, obj3, true)
		assertFilter(t, f, obj4, true)
	})
}

func TestFilterExists(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		eq := FilterExists{Key: "k"}
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("equal test")})
		assertFilter(t, eq, obj, true)
	})
	t.Run("not ok", func(t *testing.T) {
		eq := FilterExists{Key: "foo"}
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("equal test")})
		assertFilter(t, eq, obj, false)
	})
}

func TestFilterOptionsEqual(t *testing.T) {
	optionIdToName := map[string]string{
		"optionId1": "1",
		"optionId2": "2",
		"optionId3": "3",
	}
	t.Run("one option, ok", func(t *testing.T) {
		eq := newFilterOptionsEqual(&anyenc.Arena{}, "k", []string{"optionId1"}, optionIdToName)
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"optionId1"})})
		assertFilter(t, eq, obj, true)
	})
	t.Run("two options, ok", func(t *testing.T) {
		eq := newFilterOptionsEqual(&anyenc.Arena{}, "k", []string{"optionId1", "optionId3"}, optionIdToName)
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"optionId1", "optionId3"})})
		assertFilter(t, eq, obj, true)
	})
	t.Run("two options, ok, not existing options are discarded", func(t *testing.T) {
		eq := newFilterOptionsEqual(&anyenc.Arena{}, "k", []string{"optionId1", "optionId3"}, optionIdToName)
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"optionId1", "optionId3", "optionId7000"})})
		assertFilter(t, eq, obj, true)
	})
	t.Run("two options, not ok", func(t *testing.T) {
		eq := newFilterOptionsEqual(&anyenc.Arena{}, "k", []string{"optionId1", "optionId2"}, optionIdToName)
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"optionId1", "optionId3"})})
		assertFilter(t, eq, obj, false)
	})
	t.Run("two options, not ok, because object has 1 option", func(t *testing.T) {
		eq := newFilterOptionsEqual(&anyenc.Arena{}, "k", []string{"optionId1", "optionId2"}, optionIdToName)
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"optionId1"})})
		assertFilter(t, eq, obj, false)
	})
	t.Run("two options, not ok, because object has 3 options", func(t *testing.T) {
		eq := newFilterOptionsEqual(&anyenc.Arena{}, "k", []string{"optionId1", "optionId2"}, optionIdToName)
		obj := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"optionId1", "optionId2", "optionId3"})})
		assertFilter(t, eq, obj, false)
	})
}

func TestMakeFilters(t *testing.T) {
	t.Run("no filters", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}

		// when
		filters, err := MakeFilters(nil, mockStore)

		// then
		assert.Nil(t, err)
		assert.Len(t, filters, 0)
	})
	t.Run("or filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_Or,
				NestedFilters: []FilterRequest{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
				},
			},
		}

		// when
		filters, err := MakeFilters(filter, mockStore)

		// then
		assert.Nil(t, err)
		assert.Len(t, filters, 2)
		assert.NotNil(t, filters.(FiltersOr))
		assert.NotNil(t, filters.(FiltersOr)[0].(FilterEq))
		assert.NotNil(t, filters.(FiltersOr)[1].(FilterEq))
		assert.Equal(t, domain.RelationKey("relationKey"), filters.(FiltersOr)[0].(FilterEq).Key)
		assert.Equal(t, domain.String("option2"), filters.(FiltersOr)[0].(FilterEq).Value)
		assert.Equal(t, domain.RelationKey("name"), filters.(FiltersOr)[1].(FilterEq).Key)
		assert.Equal(t, domain.String("Object 1"), filters.(FiltersOr)[1].(FilterEq).Value)
	})
	t.Run("and filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []FilterRequest{
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("option2"),
						Format:      model.RelationFormat_status,
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: bundle.RelationKeyName,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("Object 1"),
						Format:      model.RelationFormat_shorttext,
					},
				},
			},
		}

		// when
		filters, err := MakeFilters(filter, mockStore)

		// then
		assert.Nil(t, err)
		assert.Len(t, filters, 2)
		assert.NotNil(t, filters.(FiltersAnd))
		assert.NotNil(t, filters.(FiltersAnd)[0].(FilterEq))
		assert.NotNil(t, filters.(FiltersAnd)[1].(FilterEq))
		assert.Equal(t, domain.RelationKey("relationKey"), filters.(FiltersAnd)[0].(FilterEq).Key)
		assert.Equal(t, domain.String("option2"), filters.(FiltersAnd)[0].(FilterEq).Value)
		assert.Equal(t, domain.RelationKey("name"), filters.(FiltersAnd)[1].(FilterEq).Key)
		assert.Equal(t, domain.String("Object 1"), filters.(FiltersAnd)[1].(FilterEq).Value)
	})
	t.Run("none filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator:    model.BlockContentDataviewFilter_No,
				RelationKey: "relationKey",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String("option1"),
				Format:      model.RelationFormat_status,
			},
		}

		// when
		filters, err := MakeFilters(filter, mockStore)

		// then
		assert.Nil(t, err)
		assert.Len(t, filters, 1)
		assert.NotNil(t, filters.(FiltersAnd))
		assert.NotNil(t, filters.(FiltersAnd)[0].(FilterEq))
		assert.Equal(t, domain.RelationKey("relationKey"), filters.(FiltersAnd)[0].(FilterEq).Key)
		assert.Equal(t, domain.String("option1"), filters.(FiltersAnd)[0].(FilterEq).Value)
	})
	t.Run("combined filter", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_And,
				NestedFilters: []FilterRequest{
					{
						Operator: model.BlockContentDataviewFilter_Or,
						NestedFilters: []FilterRequest{
							{
								Operator:    model.BlockContentDataviewFilter_No,
								RelationKey: "relationKey",
								Condition:   model.BlockContentDataviewFilter_Equal,
								Value:       domain.String("option1"),
								Format:      model.RelationFormat_status,
							},
							{
								Operator:    model.BlockContentDataviewFilter_No,
								RelationKey: "relationKey1",
								Condition:   model.BlockContentDataviewFilter_Equal,
								Value:       domain.String("option2"),
								Format:      model.RelationFormat_status,
							},
						},
					},
					{
						Operator:    model.BlockContentDataviewFilter_No,
						RelationKey: "relationKey3",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.String("option3"),
						Format:      model.RelationFormat_status,
					},
				},
			},
		}

		// when
		filters, err := MakeFilters(filter, mockStore)

		// then
		assert.Nil(t, err)
		assert.Len(t, filters, 2)
		assert.NotNil(t, filters.(FiltersAnd))
		assert.NotNil(t, filters.(FiltersAnd)[0].(FiltersOr))
		assert.NotNil(t, filters.(FiltersAnd)[1].(FilterEq))
	})
	t.Run("linear and nested filters", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				RelationKey: "key1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
			{
				Operator: model.BlockContentDataviewFilter_Or,
				NestedFilters: []FilterRequest{
					{
						RelationKey: "key2",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.Bool(true),
					},
				},
			},
		}

		// when
		filters, err := MakeFilters(filter, mockStore)

		// then
		assert.Nil(t, err)
		assert.Len(t, filters, 2)
		assert.NotNil(t, filters.(FiltersAnd))
		assert.NotNil(t, filters.(FiltersAnd)[0].(FilterEq))
		assert.NotNil(t, filters.(FiltersAnd)[1].(FiltersOr))
	})
	t.Run("linear and nested filters", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "key1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "key1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "key1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "key1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "key1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "key1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
			{
				Operator:    model.BlockContentDataviewFilter_And,
				RelationKey: "key1",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
		}

		// when
		_, err := MakeFilters(filter, mockStore)

		// then
		assert.Nil(t, err)
	})
	t.Run("transform quick options", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_Or,
				NestedFilters: []FilterRequest{
					{
						RelationKey: "key2",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Format:      model.RelationFormat_date,
						Value:       domain.Int64(time.Now().Unix()),
						QuickOption: model.BlockContentDataviewFilter_CurrentMonth,
					},
					{
						RelationKey: "key3",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.Bool(true),
					},
				},
			},
		}

		// when
		filters, err := MakeFilters(filter, mockStore)

		// then
		assert.Nil(t, err)
		assert.Len(t, filters, 2)
		assert.NotNil(t, filters.(FiltersOr))
		assert.NotNil(t, filters.(FiltersOr)[0].(FiltersAnd))
		assert.NotNil(t, filters.(FiltersOr)[1].(FilterEq))
	})
	t.Run("transform quick options", func(t *testing.T) {
		// given
		mockStore := &stubSpaceObjectStore{}
		filter := []FilterRequest{
			{
				Operator: model.BlockContentDataviewFilter_Or,
				NestedFilters: []FilterRequest{
					{
						RelationKey: "key2",
						Condition:   model.BlockContentDataviewFilter_Less,
						Value:       domain.Int64(time.Now().Unix()),
						QuickOption: model.BlockContentDataviewFilter_CurrentMonth,
					},
					{
						RelationKey: "key3",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.Bool(true),
					},
				},
			},
		}

		// when
		filters, err := MakeFilters(filter, mockStore)

		// then
		assert.Nil(t, err)
		assert.Len(t, filters, 2)
		assert.NotNil(t, filters.(FiltersOr))
		assert.NotNil(t, filters.(FiltersOr)[0].(FilterEq))
		assert.NotNil(t, filters.(FiltersOr)[1].(FilterEq))
	})
}

func TestFilter2ValuesComp_FilterObject(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		eq := Filter2ValuesComp{
			Key1: "a",
			Key2: "b",
			Cond: model.BlockContentDataviewFilter_Equal,
		}
		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"a": domain.String("x"),
			"b": domain.String("x"),
		})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"a": domain.String("x"),
			"b": domain.String("y"),
		})
		obj3 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"b": domain.String("x"),
		})
		assertFilter(t, eq, obj1, true)
		assertFilter(t, eq, obj2, false)
		assertFilter(t, eq, obj3, false)
	})

	t.Run("greater", func(t *testing.T) {
		eq := Filter2ValuesComp{
			Key1: "a",
			Key2: "b",
			Cond: model.BlockContentDataviewFilter_Greater,
		}
		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"a": domain.Int64(100),
			"b": domain.Int64(200),
		})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"a": domain.Int64(300),
			"b": domain.Int64(-500),
		})
		obj3 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"a": domain.String("xxx"),
			"b": domain.String("ddd"),
		})
		assertFilter(t, eq, obj1, false)
		assertFilter(t, eq, obj2, true)
		assertFilter(t, eq, obj3, true)
	})
}

func TestFilterHasPrefix_FilterObject(t *testing.T) {
	t.Run("date object id", func(t *testing.T) {
		key := bundle.RelationKeyMentions
		now := time.Now()
		f := FilterHasPrefix{
			Key:    key,
			Prefix: dateutil.NewDateObject(now, false).Id(), // _date_YYYY-MM-DD
		}
		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			key: domain.StringList([]string{"obj2", dateutil.NewDateObject(now.Add(30*time.Minute), true).Id(), "obj3"}), // _date_YYYY-MM-DD-hh-mm-ssZ-zzzz
		})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			key: domain.StringList([]string{dateutil.NewDateObject(now.Add(24*time.Hour), true).Id(), "obj1", "obj3"}), // same format, but next day
		})
		obj3 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			key: domain.StringList([]string{"obj2", "obj3", dateutil.NewDateObject(now.Add(30*time.Minute), true).Id()}), // _date_YYYY-MM-DD
		})
		assertFilter(t, f, obj1, true)
		assertFilter(t, f, obj2, false)
		assertFilter(t, f, obj3, true)
	})

	t.Run("string", func(t *testing.T) {
		key := bundle.RelationKeyName
		f := FilterHasPrefix{
			Key:    key,
			Prefix: "Let's",
		}
		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			key: domain.String("Let's do it"),
		})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			key: domain.String("Lets do it"),
		})
		obj3 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			key: domain.String("Let's fix it :("),
		})
		assertFilter(t, f, obj1, true)
		assertFilter(t, f, obj2, false)
		assertFilter(t, f, obj3, true)
	})

	t.Run("string list", func(t *testing.T) {
		toys := domain.RelationKey("my favorite toys")
		f := FilterHasPrefix{
			Key:    toys,
			Prefix: "Fluffy",
		}
		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			toys: domain.StringList([]string{"Teddy bear", "Fluffy giraffe"}),
		})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			toys: domain.StringList([]string{"Barbie doll", "Peppa Pig"}),
		})
		obj3 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			toys: domain.StringList([]string{"T Rex", "Fluffy Rabbit the Murderer"}),
		})
		assertFilter(t, f, obj1, true)
		assertFilter(t, f, obj2, false)
		assertFilter(t, f, obj3, true)
	})
}

func TestFilterDate(t *testing.T) {
	t.Run("filter by date only", func(t *testing.T) {
		store := &stubSpaceObjectStore{}

		now := time.Unix(1736332001, 0).UTC()
		f, err := MakeFilter("spaceId", FilterRequest{
			RelationKey: bundle.RelationKeyCreatedDate,
			Format:      model.RelationFormat_date,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(now.Unix()),
		}, store)
		require.NoError(t, err)

		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyCreatedDate: domain.Int64(now.Add(-time.Hour * 24).Unix())})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyCreatedDate: domain.Int64(now.Unix())})
		obj3 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyCreatedDate: domain.Int64(now.Add(24 * time.Hour).Unix())})
		assertFilter(t, f, obj1, false)
		assertFilter(t, f, obj2, true)
		assertFilter(t, f, obj3, false)
	})
}
