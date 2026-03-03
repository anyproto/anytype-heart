package spaceindex

import (
	"testing"

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

func TestGetObjectsWithObjectInRelation(t *testing.T) {
	t.Run("returns nil when path relation is not name or pluralName", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		details := makeDetails(TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("myTag"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("tagRel"),
		})
		path := domain.ObjectPath{ObjectId: "tag1", RelationKey: "description"}

		// when
		result := s.getObjectsWithObjectInRelation(details, 1.0, path, 10, emptyFilters(t, s))

		// then
		assert.Nil(t, result)
	})

	t.Run("returns nil when object is deleted", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		details := makeDetails(TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("myTag"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("tagRel"),
			bundle.RelationKeyIsDeleted:      domain.Bool(true),
		})
		path := domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(details, 1.0, path, 10, emptyFilters(t, s))

		// then
		assert.Nil(t, result)
	})

	t.Run("returns nil when object is archived", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		details := makeDetails(TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("myTag"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("tagRel"),
			bundle.RelationKeyIsArchived:     domain.Bool(true),
		})
		path := domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(details, 1.0, path, 10, emptyFilters(t, s))

		// then
		assert.Nil(t, result)
	})

	t.Run("returns nil for unsupported layout", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		details := makeDetails(TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("mySet"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
		})
		path := domain.ObjectPath{ObjectId: "obj1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(details, 1.0, path, 10, emptyFilters(t, s))

		// then
		assert.Nil(t, result)
	})

	t.Run("returns objects with matching tag (relationOption layout)", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Important"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("myTagRel"),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Object 1"),
			domain.RelationKey("myTagRel"):   domain.StringList([]string{"tag1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("Object 2"),
			domain.RelationKey("myTagRel"):   domain.StringList([]string{"tag1", "tag2"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{tagObj, obj1, obj2})

		path := domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(tagObj), 0.8, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 2)
		gotIds := []string{
			result[0].Details.GetString(bundle.RelationKeyId),
			result[1].Details.GetString(bundle.RelationKeyId),
		}
		assert.ElementsMatch(t, []string{"obj1", "obj2"}, gotIds)

		// verify score is propagated
		for _, rec := range result {
			assert.Equal(t, 0.8, rec.Details.GetFloat64(database.RecordScoreField))
		}

		// verify meta
		for _, rec := range result {
			assert.Equal(t, "myTagRel", rec.Meta.RelationKey)
			assert.NotNil(t, rec.Meta.RelationDetails)
		}
	})

	t.Run("returns objects with matching type (objectType layout)", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		typeObj := TestObject{
			bundle.RelationKeyId:             domain.String("type1"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("My task"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("Another task"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj3 := TestObject{
			bundle.RelationKeyId:             domain.String("obj3"),
			bundle.RelationKeyName:           domain.String("A note"),
			bundle.RelationKeyType:           domain.String("type2"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_note)),
		}
		s.AddObjects(t, []TestObject{typeObj, obj1, obj2, obj3})

		path := domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(typeObj), 0.5, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 2)
		gotIds := []string{
			result[0].Details.GetString(bundle.RelationKeyId),
			result[1].Details.GetString(bundle.RelationKeyId),
		}
		assert.ElementsMatch(t, []string{"obj1", "obj2"}, gotIds)

		for _, rec := range result {
			assert.Equal(t, "type", rec.Meta.RelationKey)
		}
	})

	t.Run("returns objects linked via links relation for basic layout", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		linkedObj := TestObject{
			bundle.RelationKeyId:             domain.String("linked1"),
			bundle.RelationKeyName:           domain.String("Linked Object"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Object with link"),
			bundle.RelationKeyLinks:          domain.StringList([]string{"linked1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{linkedObj, obj1})

		path := domain.ObjectPath{ObjectId: "linked1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(linkedObj), 0.7, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 1)
		assert.Equal(t, "obj1", result[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "links", result[0].Meta.RelationKey)
	})

	t.Run("returns objects linked via links relation for note layout", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		noteObj := TestObject{
			bundle.RelationKeyId:             domain.String("note1"),
			bundle.RelationKeyName:           domain.String("My Note"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_note)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Linking object"),
			bundle.RelationKeyLinks:          domain.StringList([]string{"note1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{noteObj, obj1})

		path := domain.ObjectPath{ObjectId: "note1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(noteObj), 0.9, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 1)
		assert.Equal(t, "obj1", result[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "links", result[0].Meta.RelationKey)
	})

	t.Run("returns objects linked via links relation for profile layout", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		profileObj := TestObject{
			bundle.RelationKeyId:             domain.String("profile1"),
			bundle.RelationKeyName:           domain.String("John"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_profile)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Document by John"),
			bundle.RelationKeyLinks:          domain.StringList([]string{"profile1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{profileObj, obj1})

		path := domain.ObjectPath{ObjectId: "profile1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(profileObj), 0.6, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 1)
		assert.Equal(t, "obj1", result[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "links", result[0].Meta.RelationKey)
	})

	t.Run("returns objects linked via links relation for todo layout", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		todoObj := TestObject{
			bundle.RelationKeyId:             domain.String("todo1"),
			bundle.RelationKeyName:           domain.String("Buy groceries"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Shopping plan"),
			bundle.RelationKeyLinks:          domain.StringList([]string{"todo1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{todoObj, obj1})

		path := domain.ObjectPath{ObjectId: "todo1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(todoObj), 0.5, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 1)
		assert.Equal(t, "obj1", result[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "links", result[0].Meta.RelationKey)
	})

	t.Run("returns objects linked via links relation for participant layout", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		participantObj := TestObject{
			bundle.RelationKeyId:             domain.String("participant1"),
			bundle.RelationKeyName:           domain.String("Alice"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Project with Alice"),
			bundle.RelationKeyLinks:          domain.StringList([]string{"participant1"}),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{participantObj, obj1})

		path := domain.ObjectPath{ObjectId: "participant1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(participantObj), 0.4, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 1)
		assert.Equal(t, "obj1", result[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "links", result[0].Meta.RelationKey)
	})

	t.Run("works with pluralName path", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		typeObj := TestObject{
			bundle.RelationKeyId:             domain.String("type1"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyPluralName:     domain.String("Tasks"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("My task"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{typeObj, obj1})

		path := domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyPluralName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(typeObj), 0.5, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 1)
		assert.Equal(t, "obj1", result[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("respects limit parameter", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		typeObj := TestObject{
			bundle.RelationKeyId:             domain.String("type1"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}
		for i := 0; i < 5; i++ {
			obj := TestObject{
				bundle.RelationKeyId:             domain.String("obj" + string(rune('A'+i))),
				bundle.RelationKeyName:           domain.String("Task item"),
				bundle.RelationKeyType:           domain.String("type1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			}
			s.AddObjects(t, []TestObject{obj})
		}
		s.AddObjects(t, []TestObject{typeObj})

		path := domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(typeObj), 0.5, path, 2, emptyFilters(t, s))

		// then
		assert.Len(t, result, 2)
	})

	t.Run("respects params filter", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		typeObj := TestObject{
			bundle.RelationKeyId:             domain.String("type1"),
			bundle.RelationKeyName:           domain.String("Task"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		}
		obj1 := TestObject{
			bundle.RelationKeyId:             domain.String("obj1"),
			bundle.RelationKeyName:           domain.String("Active task"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyIsArchived:     domain.Bool(false),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		obj2 := TestObject{
			bundle.RelationKeyId:             domain.String("obj2"),
			bundle.RelationKeyName:           domain.String("Archived task"),
			bundle.RelationKeyType:           domain.String("type1"),
			bundle.RelationKeyIsArchived:     domain.Bool(true),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}
		s.AddObjects(t, []TestObject{typeObj, obj1, obj2})

		path := domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyName.String()}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyIsArchived,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		}, nil)

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(typeObj), 0.5, path, 10, params)

		// then
		require.Len(t, result, 1)
		assert.Equal(t, "obj1", result[0].Details.GetString(bundle.RelationKeyId))
	})

	t.Run("sets correct score on injected results", func(t *testing.T) {
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

		path := domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(tagObj), 1.5, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 1)
		assert.Equal(t, 1.5, result[0].Details.GetFloat64(database.RecordScoreField))
	})

	t.Run("sets correct meta with filtered relation details", func(t *testing.T) {
		// given
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

		path := domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(tagObj), 1.0, path, 10, emptyFilters(t, s))

		// then
		require.Len(t, result, 1)
		assert.Equal(t, "priority", result[0].Meta.RelationKey)

		wantRelDetails := pbtypes.StructFilterKeys(makeDetails(tagObj).ToProto(), []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyType.String(),
			bundle.RelationKeyResolvedLayout.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		})
		assert.Equal(t, wantRelDetails, result[0].Meta.RelationDetails)
	})

	t.Run("returns empty when no objects match", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		tagObj := TestObject{
			bundle.RelationKeyId:             domain.String("tag1"),
			bundle.RelationKeyName:           domain.String("Unused tag"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			bundle.RelationKeyRelationKey:    domain.String("tagRel"),
		}
		s.AddObjects(t, []TestObject{tagObj})

		path := domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}

		// when
		result := s.getObjectsWithObjectInRelation(makeDetails(tagObj), 1.0, path, 10, emptyFilters(t, s))

		// then
		assert.Empty(t, result)
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
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 0.5},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "document")

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
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 0.5},
		}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyDescription,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String("relevant"),
			},
		}, nil)

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "test")

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
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0},
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "description"}, Score: 0.5},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "test")

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "obj1", recs[0].Details.GetString(bundle.RelationKeyId))
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
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0},
		}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List([]int64{int64(model.ObjectType_relationOption)}),
			},
		}, nil)

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "Urgent")

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "obj1", recs[0].Details.GetString(bundle.RelationKeyId))
		assert.Equal(t, "priority", recs[0].Meta.RelationKey)
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
			{Path: domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyName.String()}, Score: 0.9},
		}
		params := newFilters(t, s, []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List([]int64{int64(model.ObjectType_objectType)}),
			},
		}, nil)

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "Recipe")

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
			{Path: domain.ObjectPath{ObjectId: "linked1", RelationKey: bundle.RelationKeyName.String()}, Score: 0.8},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "Reference")

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
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 3, 0, "Document")

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
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 3.0},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 2.0},
			{Path: domain.ObjectPath{ObjectId: "obj3", RelationKey: "name"}, Score: 1.0},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 1, "test")

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
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 3, 2, "Item")

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
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 100, "test")

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
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0},
			{Path: domain.ObjectPath{ObjectId: "obj2", RelationKey: "name"}, Score: 0.9},
			{Path: domain.ObjectPath{ObjectId: "obj3", RelationKey: "name"}, Score: 0.8},
		}
		params := newFilters(t, s, nil, []database.SortRequest{
			{
				RelationKey: bundle.RelationKeyName,
				Type:        model.BlockContentDataviewSort_Asc,
			},
		})

		// when
		recs, err := s.QueryFromFulltext(results, params, 0, 0, "test")

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
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 2.0},
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "test")

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
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "description")

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
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 3.14},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "Doc")

		// then
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.InDelta(t, 3.14, recs[0].Details.GetFloat64(database.RecordScoreField), 0.001)
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
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "World")

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
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "Items")

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
			{Path: domain.ObjectPath{ObjectId: "direct1", RelationKey: "name"}, Score: 2.0},
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.5},
			{Path: domain.ObjectPath{ObjectId: "type1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "test")

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
		recs, err := s.QueryFromFulltext(nil, emptyFilters(t, s), 0, 0, "test")

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
			{Path: domain.ObjectPath{ObjectId: "nonexistent", RelationKey: "name"}, Score: 2.0},
			{Path: domain.ObjectPath{ObjectId: "obj1", RelationKey: "name"}, Score: 1.0},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "test")

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
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "DeletedTag")

		// then
		require.NoError(t, err)
		for _, rec := range recs {
			assert.NotEqual(t, "obj1", rec.Details.GetString(bundle.RelationKeyId))
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
			{Path: domain.ObjectPath{ObjectId: "tag1", RelationKey: bundle.RelationKeyName.String()}, Score: 1.0},
		}

		// when
		recs, err := s.QueryFromFulltext(results, emptyFilters(t, s), 0, 0, "ArchivedTag")

		// then
		require.NoError(t, err)
		for _, rec := range recs {
			assert.NotEqual(t, "obj1", rec.Details.GetString(bundle.RelationKeyId))
		}
	})
}
