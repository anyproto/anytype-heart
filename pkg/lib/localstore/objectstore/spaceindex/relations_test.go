package spaceindex

import (
	context2 "context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TODO Decide what to do with it
// func TestGetAggregatedOptions(t *testing.T) {
// 	t.Run("with no options", func(t *testing.T) {
// 		s := newFixture(t)
//
// 		got, err := s.GetAggregatedOptions(bundle.RelationKeyTag.String())
// 		require.NoError(t, err)
// 		assert.Empty(t, got)
// 	})
//
// 	t.Run("with options", func(t *testing.T) {
// 		s := newFixture(t)
// 		opt1 := makeRelationOptionObject("id1", "name1", "color1", bundle.RelationKeyTag.String())
// 		opt2 := makeRelationOptionObject("id2", "name2", "color2", bundle.RelationKeyStatus.String())
// 		opt3 := makeRelationOptionObject("id3", "name3", "color3", bundle.RelationKeyTag.String())
// 		s.AddObjects(t, []objectstore.TestObject{opt1, opt2, opt3})
//
// 		got, err := s.GetAggregatedOptions(bundle.RelationKeyTag.String())
// 		require.NoError(t, err)
// 		want := []*model.RelationOption{
// 			{
// 				Id:          "id1",
// 				Text:        "name1",
// 				Color:       "color1",
// 				RelationKey: bundle.RelationKeyTag.String(),
// 			},
// 			{
// 				Id:          "id3",
// 				Text:        "name3",
// 				Color:       "color3",
// 				RelationKey: bundle.RelationKeyTag.String(),
// 			},
// 		}
// 		assert.Equal(t, want, got)
// 	})
// }

func TestGetRelationById(t *testing.T) {
	t.Run("relation is not found", func(t *testing.T) {
		s := NewStoreFixture(t)

		_, err := s.GetRelationById("relationID")
		require.Error(t, err)
	})

	t.Run("requested object is not relation", func(t *testing.T) {
		s := NewStoreFixture(t)

		obj := TestObject{
			bundle.RelationKeyId:      domain.String("id1"),
			bundle.RelationKeyName:    domain.String("name1"),
			bundle.RelationKeySpaceId: domain.String("space1"),
		}
		s.AddObjects(t, []TestObject{obj})

		_, err := s.GetRelationById("id1")
		require.Error(t, err)
	})

	t.Run("relation is found", func(t *testing.T) {
		s := NewStoreFixture(t)

		relation := &relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyName)}
		relationID := "derivedFrom(name)"
		relation.Id = relationID
		relObject := relation.ToDetails()
		err := s.UpdateObjectDetails(context2.Background(), relation.Id, relObject)
		require.NoError(t, err)

		got, err := s.GetRelationById(relationID)
		require.NoError(t, err)
		assert.Equal(t, relationutils.RelationFromDetails(relObject).Relation, got)
	})
}

// A deleted property keeps its data, and the by-id arm has always returned it
// (GetRelationById reads details directly). The by-key arm used to run through
// Query, which injects `isDeleted != true`, so the same relation resolved by id
// and vanished by key — an export could name a property in a type document and
// then fail to define it in the dictionary.
func TestGetRelationByKey_DeletedRelationStillResolves(t *testing.T) {
	newDeleted := func(t *testing.T, key string) *StoreFixture {
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{{
			bundle.RelationKeyId:             domain.String("rel-" + key),
			bundle.RelationKeySpaceId:        domain.String("space1"),
			bundle.RelationKeyRelationKey:    domain.String(key),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			bundle.RelationKeyIsDeleted:      domain.Bool(true),
		}})
		return s
	}

	t.Run("resolves by key", func(t *testing.T) {
		s := newDeleted(t, "68cda76ee9223c9dc7ce5e92")

		got, err := s.GetRelationByKey("68cda76ee9223c9dc7ce5e92")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "68cda76ee9223c9dc7ce5e92", got.Key)
		assert.Equal(t, model.RelationFormat_date, got.Format)
	})

	t.Run("both arms agree", func(t *testing.T) {
		s := newDeleted(t, "68cda76ee9223c9dc7ce5e92")

		byId, err := s.GetRelationById("rel-68cda76ee9223c9dc7ce5e92")
		require.NoError(t, err)
		byKey, err := s.GetRelationByKey("68cda76ee9223c9dc7ce5e92")
		require.NoError(t, err)
		assert.Equal(t, byId, byKey)
	})

	t.Run("a missing key is still not found", func(t *testing.T) {
		s := newDeleted(t, "68cda76ee9223c9dc7ce5e92")

		_, err := s.GetRelationByKey("nosuchkey")
		require.Error(t, err)
	})
}

// Dropping the default filters lets a removed row and a live row both match, so
// the live one must still win — otherwise fixing the deleted case silently
// regresses every ordinary lookup that has a stale twin.
func TestGetRelationByKey_LiveRowWinsOverRemoved(t *testing.T) {
	for _, removed := range []domain.RelationKey{bundle.RelationKeyIsDeleted, bundle.RelationKeyIsArchived} {
		t.Run(removed.String(), func(t *testing.T) {
			s := NewStoreFixture(t)
			s.AddObjects(t, []TestObject{
				{
					bundle.RelationKeyId:             domain.String("rel-aaa-removed"),
					bundle.RelationKeySpaceId:        domain.String("space1"),
					bundle.RelationKeyRelationKey:    domain.String("dupkey"),
					bundle.RelationKeyName:           domain.String("Old"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
					bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
					removed:                          domain.Bool(true),
				},
				{
					bundle.RelationKeyId:             domain.String("rel-zzz-live"),
					bundle.RelationKeySpaceId:        domain.String("space1"),
					bundle.RelationKeyRelationKey:    domain.String("dupkey"),
					bundle.RelationKeyName:           domain.String("Live"),
					bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
					bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				},
			})

			got, err := s.GetRelationByKey("dupkey")
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "Live", got.Name)
			assert.Equal(t, model.RelationFormat_longtext, got.Format)
		})
	}
}
