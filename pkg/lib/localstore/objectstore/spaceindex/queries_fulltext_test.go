package spaceindex

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func newFilters(t *testing.T, s *StoreFixture, filters []database.FilterRequest, sorts []database.SortRequest) database.Filters {
	t.Helper()
	f, err := database.NewFilters(database.Query{
		Filters: filters,
		Sorts:   sorts,
	}, s, &anyenc.Arena{}, &collate.Buffer{})
	require.NoError(t, err)
	return *f
}

func emptyFilters(t *testing.T, s *StoreFixture) database.Filters {
	t.Helper()
	return newFilters(t, s, nil, nil)
}

func TestInjectionRelationKey(t *testing.T) {
	t.Run("skips non-name path", func(t *testing.T) {
		details := makeDetails(TestObject{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		})
		_, ok := injectionRelationKey(details, false)
		assert.False(t, ok)
	})

	t.Run("skips deleted object", func(t *testing.T) {
		details := makeDetails(TestObject{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyIsDeleted:      domain.Bool(true),
		})
		_, ok := injectionRelationKey(details, true)
		assert.False(t, ok)
	})

	t.Run("skips archived object", func(t *testing.T) {
		details := makeDetails(TestObject{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyIsArchived:     domain.Bool(true),
		})
		_, ok := injectionRelationKey(details, true)
		assert.False(t, ok)
	})

	t.Run("skips unsupported layout", func(t *testing.T) {
		details := makeDetails(TestObject{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
		})
		_, ok := injectionRelationKey(details, true)
		assert.False(t, ok)
	})

	for _, layout := range []model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_note,
		model.ObjectType_profile,
		model.ObjectType_todo,
		model.ObjectType_participant,
	} {
		t.Run("returns links for "+layout.String(), func(t *testing.T) {
			details := makeDetails(TestObject{
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(layout)),
			})
			key, ok := injectionRelationKey(details, true)
			require.True(t, ok)
			assert.Equal(t, bundle.RelationKeyLinks, key)
		})
	}

	t.Run("returns type for objectType layout", func(t *testing.T) {
		details := makeDetails(TestObject{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		})
		key, ok := injectionRelationKey(details, true)
		require.True(t, ok)
		assert.Equal(t, bundle.RelationKeyType, key)
	})

	t.Run("skips relationOption without relation key", func(t *testing.T) {
		details := makeDetails(TestObject{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		})
		_, ok := injectionRelationKey(details, true)
		assert.False(t, ok)
	})

	t.Run("returns custom relation key for relationOption layout", func(t *testing.T) {
		details := makeDetails(TestObject{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("priority"),
		})
		key, ok := injectionRelationKey(details, true)
		require.True(t, ok)
		assert.Equal(t, domain.RelationKey("priority"), key)
	})

	t.Run("works with pluralName-derived name match", func(t *testing.T) {
		details := makeDetails(TestObject{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		})
		// NameMatch is true for both name and pluralName best docs
		_, ok := injectionRelationKey(details, true)
		assert.True(t, ok)
	})
}

func TestMatchHit(t *testing.T) {
	relKey := domain.RelationKey("tags")

	t.Run("returns false when no value matches", func(t *testing.T) {
		details := makeDetails(TestObject{
			relKey: domain.StringList([]string{"x", "y"}),
		})
		hitMap := map[string]injectionHit{
			"other": {id: "other", score: 1.0},
		}

		_, ok := matchHit(details, relKey, hitMap)
		assert.False(t, ok)
	})

	t.Run("picks highest-scoring hit regardless of value order", func(t *testing.T) {
		// given the lower-scoring hit appears first in the relation value list
		details := makeDetails(TestObject{
			relKey: domain.StringList([]string{"low", "high", "mid"}),
		})
		hitMap := map[string]injectionHit{
			"low":  {id: "low", score: 0.5},
			"mid":  {id: "mid", score: 1.0},
			"high": {id: "high", score: 3.0},
		}

		hit, ok := matchHit(details, relKey, hitMap)

		require.True(t, ok)
		assert.Equal(t, "high", hit.id)
		assert.Equal(t, 3.0, hit.score)
	})

	t.Run("ties are broken deterministically by id", func(t *testing.T) {
		// given two hits with equal score, listed in arbitrary order
		details := makeDetails(TestObject{
			relKey: domain.StringList([]string{"zebra", "apple"}),
		})
		hitMap := map[string]injectionHit{
			"zebra": {id: "zebra", score: 1.0},
			"apple": {id: "apple", score: 1.0},
		}

		hit, ok := matchHit(details, relKey, hitMap)

		require.True(t, ok)
		assert.Equal(t, "apple", hit.id)
	})
}

func TestQueryFromFulltext(t *testing.T) {
	t.Run("returns matched objects from fulltext results", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("First document"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("Second document"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1, obj2})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 0.5, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "document", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 2)
		assert.Equal(t, "obj1", recs[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "obj2", recs[1].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("filters are applied to fulltext results", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Keep"),
			bundle.RelationKeyDescription:    domain.String("relevant"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("Filter out"),
			bundle.RelationKeyDescription:    domain.String("irrelevant"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1, obj2})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 0.5, NameMatch: true},
		}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyDescription,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String("relevant"),
			},
		}, nil)

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "test", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "obj1", recs[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("deduplicates fulltext results by object id", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Document"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "description"}, Score: 0.5},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "test", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "obj1", recs[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("withInjections=false skips the related-object injection queries", func(t *testing.T) {
		// given: a matched tag whose objects would be injected
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{
			{
				bundle.RelationKeyId:             domain.String("tag1"),
				bundle.RelationKeyName:           domain.String("Urgent"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeyRelationKey:    domain.String("priority"),
			},
			{
				bundle.RelationKeyId:             domain.String("obj1"),
				bundle.RelationKeyName:           domain.String("Task with tag"),
				domain.RelationKey("priority"):   domain.StringList([]string{"tag1"}),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
		})
		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		// when: the cross-space path disables injections
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "Urgent", false)

		// then: only the direct match, no injected records
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "tag1", recs[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("injects objects found by tag name", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Urgent"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("priority"),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Task with tag"),
			domain.RelationKey("priority"):   domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List([]int64{int64(model.ObjectType_relationOption)}),
			},
		}, nil)

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "Urgent", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "obj1", recs[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "priority", recs[0].Meta.RelationKey)
	})

	t.Run("injected record carries hit score and recomputed final_score", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Priority"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("priority"),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Task 1"),
			domain.RelationKey("priority"):   domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.5, NameMatch: true},
		}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List([]int64{int64(model.ObjectType_relationOption)}),
			},
		}, nil)

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "Priority", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		injected := recs[0]
		assert.Equal(t, "obj1", injected.Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, 1.5, injected.Details.GetFloat64(bundle.RelationKey_score))
		// final_score is recomputed from the hit score against the injected record details (no name match path)
		assert.InDelta(t,
			database.ComputeFinalScore(1.5, injected.Details, false),
			injected.Details.GetFloat64(bundle.RelationKey_final_score),
			1e-9,
		)
	})

	t.Run("injected Meta.RelationDetails is filtered to whitelisted keys", func(t *testing.T) {
		// given a tag with extra fields that should NOT leak into RelationDetails
		s := NewStoreFixture(t)
		tagObj := TestObject{
			bundle.RelationKeyId:                  domain.String("tag1"),
			bundle.RelationKeyName:                domain.String("Urgent"),
			bundle.RelationKeyType:                domain.String("optionType"),
			bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:         domain.String("priority"),
			bundle.RelationKeyRelationOptionColor: domain.String("red"),
			bundle.RelationKeyDescription:         domain.String("should not be included"),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Task 1"),
			domain.RelationKey("priority"):   domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List([]int64{int64(model.ObjectType_relationOption)}),
			},
		}, nil)

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "Urgent", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "priority", recs[0].Meta.RelationKey)

		wantRelDetails := pbtypes.StructFilterKeys(makeDetails(tagObj).ToProto(), []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyType.String(),
			bundle.RelationKeyResolvedLayout.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		})
		assert.Equal(t, wantRelDetails, recs[0].Meta.RelationDetails)
	})

	t.Run("injects objects found by type name", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		typeObj := TestObject{
			bundle.RelationKeyId:             domain.String("type1"),
			bundle.RelationKeyName:           domain.String("Recipe"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Pasta recipe"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("Cake recipe"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{typeObj, obj1, obj2})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyName.String()}, Score: 0.9, NameMatch: true},
		}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List([]int64{int64(model.ObjectType_objectType)}),
			},
		}, nil)

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "Recipe", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 2)
		gotIds := []string{
			recs[0].Details.GetString(bundle.RelationKeyId),
			recs[1].Details.GetString(bundle.RelationKeyId),
		}
		assert.ElementsMatch(t, []string{"obj1", "obj2"}, gotIds)
		for _, rec := range recs {
			assert.Equal(t, "type", rec.Meta.RelationKey)
		}
	})

	t.Run("injects objects found by linked basic object name", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		linkedObj := TestObject{
			bundle.RelationKeyId:             domain.String("linked1"),
			bundle.RelationKeyName:           domain.String("Reference doc"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Main doc"),
			bundle.RelationKeyLinks:          domain.StringList([]string{"linked1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{linkedObj, obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "linked1", RelationKey: bundle.RelationKeyName.String()}, Score: 0.8, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "Reference", true)

		// then
		require.NoError(t, err)
		gotIds := make([]string, len(recs))
		for i, rec := range recs {
			gotIds[i] = rec.Details.GetString(bundle.RelationKeyId)
		}
		assert.Contains(t, gotIds, "linked1")
		assert.Contains(t, gotIds, "obj1")
	})

	t.Run("limit is applied to final results", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		for i := 0; i < 10; i++ {
			obj := TestObject{
				bundle.RelationKeyId:             domain.String("obj" + string(rune('A'+i))),
				bundle.RelationKeyName:           domain.String("Document"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}
			s.AddObjects(t, []TestObject{obj})
		}

		var results []database.FulltextResult
		for i := 0; i < 10; i++ {
			results = append(results, database.FulltextResult{
				Path:  domain.ObjectPath{ObjectId: "obj" + string(rune('A'+i)), RelationKey: "name"},
				Score: float64(10 - i),
			})
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 3, 0, "Document", true)

		// then
		require.NoError(t, err)
		assert.Len(t, recs, 3)
	})

	t.Run("offset is applied to final results", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("First"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("Second"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:             domain.String("obj3"),
			bundle.RelationKeyName:           domain.String("Third"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 3.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 2.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj3", RelationKey: "name"}, Score: 1.0, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 1, "test", true)

		// then
		require.NoError(t, err)
		assert.Len(t, recs, 2)
	})

	t.Run("limit and offset together", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		for i := 0; i < 10; i++ {
			obj := TestObject{
				bundle.RelationKeyId:             domain.String("obj" + string(rune('A'+i))),
				bundle.RelationKeyName:           domain.String("Item"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}
			s.AddObjects(t, []TestObject{obj})
		}

		var results []database.FulltextResult
		for i := 0; i < 10; i++ {
			results = append(results, database.FulltextResult{
				Path:  domain.ObjectPath{ObjectId: "obj" + string(rune('A'+i)), RelationKey: "name"},
				Score: float64(10 - i),
			})
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 3, 2, "Item", true)

		// then
		require.NoError(t, err)
		assert.Len(t, recs, 3)
	})

	t.Run("offset exceeding results returns nil", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Only one"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 100, "test", true)

		// then
		require.NoError(t, err)
		assert.Nil(t, recs)
	})

	t.Run("order is applied to results", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Banana"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("Apple"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:             domain.String("obj3"),
			bundle.RelationKeyName:           domain.String("Cherry"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, obj3})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 0.9, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj3", RelationKey: "name"}, Score: 0.8, NameMatch: true},
		}
		params := newFilters(t, s, nil, []database.SortRequest{
			{
				RelationKey: bundle.RelationKeyName,
				Type:        model.BlockContentDataviewSort_Asc,
			},
		})

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "test", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 3)
		assert.Equal(t, "Apple", recs[0].Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "Banana", recs[1].Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "Cherry", recs[2].Details.GetString(bundle.RelationKeyName))
	})

	t.Run("injected results are deduplicated against direct results", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Important"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("tagRel"),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Tagged object"),
			domain.RelationKey("tagRel"):     domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, obj1})

		// obj1 appears both as direct result and as injected result from tag
		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 2.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "test", true)

		// then
		require.NoError(t, err)
		idCount := map[string]int{}
		for _, rec := range recs {
			id := rec.Details.GetString(bundle.RelationKeyId)
			idCount[id]++
		}
		assert.Equal(t, 1, idCount["obj1"], "obj1 should appear exactly once")
	})

	t.Run("no injected results for non-name relation matches", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Tag"),
			bundle.RelationKeyDescription:    domain.String("A description"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("myRel"),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Object"),
			domain.RelationKey("myRel"):      domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, obj1})

		// match is on description, not name - should not inject
		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: "description"}, Score: 1.0},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "description", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "tag1", recs[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("score is set on fulltext result details", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Doc"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 3.14, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "Doc", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.InDelta(t, 3.14, recs[0].Details.GetFloat64(bundle.RelationKey_score), 0.001)
	})

	t.Run("highlight is generated from title when not provided", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Hello World"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1})

		results := []database.FulltextResult{
			{
				Path:  domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"},
				Score: 1.0,
			},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "World", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "Hello World", recs[0].Meta.Highlight)
		require.Len(t, recs[0].Meta.HighlightRanges, 1)
		assert.Equal(t, int32(6), recs[0].Meta.HighlightRanges[0].From)
		assert.Equal(t, int32(11), recs[0].Meta.HighlightRanges[0].To)
	})

	t.Run("highlight from pluralName when name is empty", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyPluralName:     domain.String("Items"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1})

		results := []database.FulltextResult{
			{
				Path:  domain.ObjectPath{ObjectId: "obj1", RelationKey: "pluralName"},
				Score: 1.0,
			},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "Items", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "Items", recs[0].Meta.Highlight)
	})

	t.Run("mixed direct and injected results with multiple layouts", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Status"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("status"),
		}
		typeObj := TestObject{
			bundle.RelationKeyId:             domain.String("type1"),
			bundle.RelationKeyName:           domain.String("Document"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}
		directObj := TestObject{
			bundle.RelationKeyId:             domain.String("direct1"),
			bundle.RelationKeyName:           domain.String("My Document"),
			bundle.RelationKeyType:           domain.String("type1"),
			domain.RelationKey("status"):     domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		otherObj := TestObject{
			bundle.RelationKeyId:             domain.String("other1"),
			bundle.RelationKeyName:           domain.String("Another doc"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, typeObj, directObj, otherObj})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "direct1", RelationKey: "name"}, Score: 2.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.5, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "test", true)

		// then
		require.NoError(t, err)

		gotIds := make([]string, len(recs))
		for i, rec := range recs {
			gotIds[i] = rec.Details.GetString(bundle.RelationKeyId)
		}
		assert.Contains(t, gotIds, "direct1")
		assert.Contains(t, gotIds, "tag1")
		assert.Contains(t, gotIds, "type1")
		assert.Contains(t, gotIds, "other1")
	})

	t.Run("empty fulltext results returns empty", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		// when
		recs, err := s.QueryFromFulltext(nil, emptyFilters(t, s), 0, 0, "test", true)

		// then
		require.NoError(t, err)
		assert.Empty(t, recs)
	})

	t.Run("missing object in store is skipped", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Exists"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "nonexistent", RelationKey: "name"}, Score: 2.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "test", true)

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "obj1", recs[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("deleted tag does not inject results", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("DeletedTag"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("myRel"),
			bundle.RelationKeyIsDeleted:      domain.Bool(true),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Object with deleted tag"),
			domain.RelationKey("myRel"):      domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "DeletedTag", true)

		// then
		require.NoError(t, err)
		for _, rec := range recs {
			assert.NotEqual(t, "obj1", rec.Details.GetString(bundle.RelationKeyId))
		}
	})

	t.Run("no limit allows unlimited injection from tags", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Color"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("color"),
		}
		objects := []TestObject{tagObj}
		for i := 0; i < 5; i++ {
			objects = append(objects, TestObject{
				bundle.RelationKeyId:             domain.String("colored" + string(rune('A'+i))),
				bundle.RelationKeyName:           domain.String("Colored object"),
				domain.RelationKey("color"):      domain.StringList([]string{"tag1"}),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			})
		}
		s.AddObjects(t, objects)

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		// when — no limit (upperBound = 0, injectLimit stays 0 = unlimited)
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "Color", true)

		// then — tag1 + all 5 colored objects
		require.NoError(t, err)
		assert.Len(t, recs, 6)
	})

	t.Run("injection is capped to remaining limit capacity", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Priority"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("priority"),
		}
		objects := []TestObject{tagObj}
		for i := 0; i < 5; i++ {
			objects = append(objects, TestObject{
				bundle.RelationKeyId:             domain.String("task" + string(rune('A'+i))),
				bundle.RelationKeyName:           domain.String("Task item"),
				domain.RelationKey("priority"):   domain.StringList([]string{"tag1"}),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			})
		}
		s.AddObjects(t, objects)

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		// when — the injection budget is request-independent, so all 5 tagged
		// objects join the sequence; the page is produced by slicing it
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 3, 0, "Priority", true)

		// then — page of 3 out of tag1 + 5 injected
		require.NoError(t, err)
		assert.Len(t, recs, 3)
	})

	t.Run("injection is independent of the requested page", func(t *testing.T) {
		// Injections must not depend on limit/offset: a page-derived injection
		// budget would make different offsets paginate different sequences,
		// producing duplicates across pages.
		// given
		s := NewStoreFixture(t)

		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("ZZZ First"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("ZZZ Second"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("MyTag"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("tagRel"),
		}
		taggedObj := TestObject{
			bundle.RelationKeyId:             domain.String("tagged1"),
			bundle.RelationKeyName:           domain.String("AAA Tagged"),
			domain.RelationKey("tagRel"):     domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj1, obj2, tagObj, taggedObj})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 3.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 2.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		params := newFilters(t, s, nil, []database.SortRequest{
			{RelationKey: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc},
		})
		// full sequence sorted by name: tagged1 (injected), tag1, obj1, obj2
		want := []string{"tagged1", "tag1", "obj1", "obj2"}

		// when: pages with different limits/offsets
		page1, err := s.QueryFromFulltext(results, params, 2, 0, "test", true)
		require.NoError(t, err)
		page2, err := s.QueryFromFulltext(results, params, 2, 2, "test", true)
		require.NoError(t, err)

		// then: pages are consistent prefixes/slices of the same sequence
		var got []string
		for _, recs := range [][]database.Record{page1, page2} {
			for _, rec := range recs {
				got = append(got, rec.Details.GetString(bundle.RelationKeyId))
			}
		}
		assert.Equal(t, want, got)
	})

	t.Run("higher-scoring group wins injection budget deterministically", func(t *testing.T) {
		// given two injection groups (tag relation and type) competing for a single budget slot
		s := NewStoreFixture(t)
		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Match"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("color"),
		}
		typeObj := TestObject{
			bundle.RelationKeyId:             domain.String("type1"),
			bundle.RelationKeyName:           domain.String("Match"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}
		taggedObj := TestObject{
			bundle.RelationKeyId:             domain.String("objA"),
			bundle.RelationKeyName:           domain.String("Tagged"),
			domain.RelationKey("color"):      domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		typedObj := TestObject{
			bundle.RelationKeyId:             domain.String("objB"),
			bundle.RelationKeyName:           domain.String("Typed"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, typeObj, taggedObj, typedObj})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 2.0, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		// when — limit=3: tag1 and type1 fill 2 slots, budget=1; the "color" group
		// has the higher-scoring hit, so objA must be injected, never objB
		want := []string{"objA", "tag1", "type1"}
		for i := 0; i < 20; i++ {
			recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 3, 0, "Match", true)

			// then
			require.NoError(t, err)
			got := make([]string, 0, len(recs))
			for _, rec := range recs {
				got = append(got, rec.Details.GetString(bundle.RelationKeyId))
			}
			sort.Strings(got)
			require.Equal(t, want, got, "iteration %d: injection must be deterministic", i)
		}
	})

	t.Run("archived tag does not inject results", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("ArchivedTag"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("myRel"),
			bundle.RelationKeyIsArchived:     domain.Bool(true),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Object with archived tag"),
			domain.RelationKey("myRel"):      domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, obj1})

		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0, NameMatch: true},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "ArchivedTag", true)

		// then
		require.NoError(t, err)
		for _, rec := range recs {
			assert.NotEqual(t, "obj1", rec.Details.GetString(bundle.RelationKeyId))
		}
	})
}

func TestQueryFromFulltext_FinalScore(t *testing.T) {
	t.Run("_final_score equals ln(1+score) when dates are zero and no name match", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Test Object"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			// no date fields set, so recency contribution is zero
		}
		s.AddObjects(t, []TestObject{obj})

		score := 2.77
		results := []database.FulltextResult{
			{
				Path:  domain.ObjectPath{ObjectId: "obj1", RelationKey: "description"}, // not "name"
				Score: score,
			},
		}

		// when
		records, err := s.QueryFromFulltext(results, emptyFilters(t, s), 10, 0, "test", true)

		// then
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.InDelta(t, math.Log1p(score), records[0].Details.GetFloat64(bundle.RelationKey_final_score), 1e-9)
	})

	t.Run("participant layout wins an otherwise-equal tie", func(t *testing.T) {
		// given: two objects with the same match and no other signals
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{
			{
				bundle.RelationKeyId:             domain.String("teammate"),
				bundle.RelationKeyName:           domain.String("Sergey Fuksman"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
			{
				bundle.RelationKeyId:             domain.String("member"),
				bundle.RelationKeyName:           domain.String("Sergey Fuksman"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
			},
		})

		score := 2.77
		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "teammate", RelationKey: bundle.RelationKeyName.String()}, Score: score, NameMatch: true},
			{Path: domain.ObjectPath{ObjectId: "member", RelationKey: bundle.RelationKeyName.String()}, Score: score, NameMatch: true},
		}

		// when: a text query injects the default _final_score desc order that
		// production search paths re-rank the head with
		textFilters, err := database.NewFilters(database.Query{TextQuery: "fuksman"}, s, &anyenc.Arena{}, &collate.Buffer{})
		require.NoError(t, err)
		records, err := s.QueryFromFulltext(results, *textFilters, 10, 0, "fuksman", true)

		// then: the space member ranks first on the tie-break boost
		require.NoError(t, err)
		require.Len(t, records, 2)
		assert.Equal(t, "member", records[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "teammate", records[1].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("_final_score gets name_boost when match is in name field", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		obj := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Test Object"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{obj})

		score := 2.77
		nameResults := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: bundle.RelationKeyName.String()}, Score: score, NameMatch: true},
		}
		otherResults := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "description"}, Score: score},
		}

		// when
		nameRecords, err := s.QueryFromFulltext(nameResults, emptyFilters(t, s), 10, 0, "test", true)
		require.NoError(t, err)
		otherRecords, err := s.QueryFromFulltext(otherResults, emptyFilters(t, s), 10, 0, "test", true)
		require.NoError(t, err)

		// then
		require.Len(t, nameRecords, 1)
		require.Len(t, otherRecords, 1)
		nameScore := nameRecords[0].Details.GetFloat64(bundle.RelationKey_final_score)
		otherScore := otherRecords[0].Details.GetFloat64(bundle.RelationKey_final_score)
		assert.InDelta(t, otherScore+1.0, nameScore, 1e-9, "name match should add exactly 1.0 to _final_score")
	})

	t.Run("recently opened object scores higher than stale with same BM25", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		now := time.Now().Unix()
		sixtyDaysAgo := now - 60*86400

		fresh := TestObject{
			bundle.RelationKeyId:             domain.String("fresh"),
			bundle.RelationKeyName:           domain.String("Object"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastOpenedDate: domain.Int64(now),
		}
		stale := TestObject{
			bundle.RelationKeyId:             domain.String("stale"),
			bundle.RelationKeyName:           domain.String("Object"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyLastOpenedDate: domain.Int64(sixtyDaysAgo),
		}
		// lastModifiedDate recency signal is verified at the unit level in TestComputeFinalScore
		s.AddObjects(t, []TestObject{fresh, stale})

		sameScore := 2.77
		results := []database.FulltextResult{
			{Path: domain.ObjectPath{ObjectId: "fresh", RelationKey: "description"}, Score: sameScore},
			{Path: domain.ObjectPath{ObjectId: "stale", RelationKey: "description"}, Score: sameScore},
		}

		// when
		records, err := s.QueryFromFulltext(results, emptyFilters(t, s), 10, 0, "query", true)

		// then
		require.NoError(t, err)
		require.Len(t, records, 2)
		byId := map[string]float64{}
		for _, r := range records {
			id := r.Details.GetString(bundle.RelationKeyId)
			byId[id] = r.Details.GetFloat64(bundle.RelationKey_final_score)
		}
		assert.Greater(t, byId["fresh"], byId["stale"], "fresh object should outscore stale one with same BM25")
	})
}
