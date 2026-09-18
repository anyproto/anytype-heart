package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils/mock_relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSetDetailClearsExactJSONIntegerMetadata(t *testing.T) {
	const exact = "9007199254740993"
	key := domain.RelationKey("precision_number")
	metadataKey, lexeme, ok := anyblockjson.ExactJSONIntegerMetadata(string(key), json.Number(exact))
	require.True(t, ok)

	newDoc := func() *State {
		doc := NewDoc("root", nil).(*State)
		doc.details = domain.NewDetails()
		doc.details.Set(key, domain.Float64(9007199254740992))
		doc.details.Set(domain.RelationKey(metadataKey), domain.String(lexeme))
		return doc
	}

	t.Run("same rounded native value", func(t *testing.T) {
		next := newDoc().NewState()
		next.SetDetail(key, domain.Float64(9007199254740992))
		assert.False(t, next.Details().Has(domain.RelationKey(metadataKey)))
		assert.True(t, next.Details().Get(key).IsFloat64())
	})

	t.Run("removal", func(t *testing.T) {
		next := newDoc().NewState()
		next.RemoveDetail(key)
		assert.False(t, next.Details().Has(key))
		assert.False(t, next.Details().Has(domain.RelationKey(metadataKey)))
	})
}

func TestState_FileRelationKeys(t *testing.T) {
	fetcher := mock_relationutils.NewMockRelationFormatFetcher(t)
	fetcher.EXPECT().GetRelationFormatByKey(mock.Anything, mock.Anything).RunAndReturn(func(_ string, key domain.RelationKey) (model.RelationFormat, error) {
		rel, err := bundle.GetRelation(key)
		if err != nil {
			return 0, err
		}
		return rel.Format, nil
	})

	t.Run("no file relations", func(t *testing.T) {
		// given
		s := &State{}

		// when
		keys := s.FileRelationKeys(fetcher)

		// then
		assert.Empty(t, keys)
	})
	t.Run("there are file relations", func(t *testing.T) {
		// given
		s := &State{
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyIconImage: domain.String("face_palm"),
				bundle.RelationKeyPicture:   domain.String("Machu Picchu"),
			}),
		}

		// when
		keys := s.FileRelationKeys(fetcher)

		// then
		expectedKeys := []domain.RelationKey{bundle.RelationKeyIconImage, bundle.RelationKeyPicture}
		assert.ElementsMatch(t, keys, expectedKeys)
	})
	t.Run("coverId relation", func(t *testing.T) {
		// given
		s := &State{
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyCoverId:   domain.String("cover1"),
				bundle.RelationKeyCoverType: domain.Int64(1),
			}),
		}

		// when
		keys := s.FileRelationKeys(fetcher)

		// then
		expectedKeys := []domain.RelationKey{bundle.RelationKeyCoverId}
		assert.ElementsMatch(t, keys, expectedKeys)
	})
	t.Run("skip coverId relation", func(t *testing.T) {
		// given
		s := &State{
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyCoverId:   domain.String("cover2"),
				bundle.RelationKeyCoverType: domain.Int64(2),
			}),
		}

		// when
		keys := s.FileRelationKeys(fetcher)

		// then
		assert.Len(t, keys, 0)
	})
	t.Run("skip gradient coverId relation", func(t *testing.T) {
		// given
		s := &State{
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyCoverId:   domain.String("cover3"),
				bundle.RelationKeyCoverType: domain.Int64(3),
			}),
		}

		// when
		keys := s.FileRelationKeys(fetcher)

		// then
		assert.Len(t, keys, 0)
	})
	t.Run("mixed relations", func(t *testing.T) {
		// given
		s := &State{
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyIconImage: domain.String("smile"),
				bundle.RelationKeyCoverId:   domain.String("cover4"),
				bundle.RelationKeyCoverType: domain.Int64(4),
			}),
		}

		// when
		keys := s.FileRelationKeys(fetcher)

		// then
		expectedKeys := []domain.RelationKey{bundle.RelationKeyIconImage, bundle.RelationKeyCoverId}
		assert.ElementsMatch(t, keys, expectedKeys, "Expected both file keys and cover ID")
	})
	t.Run("coverType not in details", func(t *testing.T) {
		// given
		s := &State{
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyCoverId: domain.String("cover?"),
			}),
		}

		// when
		keys := s.FileRelationKeys(fetcher)

		// then
		assert.Len(t, keys, 0)
	})
	t.Run("unsplash cover", func(t *testing.T) {
		// given
		s := &State{
			details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyCoverId:   domain.String("unsplash cover"),
				bundle.RelationKeyCoverType: domain.Int64(5),
			}),
		}

		// when
		keys := s.FileRelationKeys(fetcher)

		// then
		assert.Len(t, keys, 1)
	})
}

func TestState_AllRelationKeys(t *testing.T) {
	t.Run("keys are aggregated from details and localDetails", func(t *testing.T) {
		// given
		s := NewDoc("root", nil).NewState()
		s.AddDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			// details
			bundle.RelationKeyCoverType: domain.Int64(1),
			bundle.RelationKeyName:      domain.String("name"),
			bundle.RelationKeyAssignee:  domain.String("assignee"),
			// local details
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_todo),
			bundle.RelationKeyType:           domain.String(bundle.TypeKeyTask.URL()),
		}))
		require.Equal(t, 3, s.details.Len())
		require.Equal(t, 2, s.localDetails.Len())

		// when
		keys := s.AllRelationKeys()

		// then
		assert.Len(t, keys, 5)
	})

	t.Run("no details", func(t *testing.T) {
		s := NewDoc("root", nil).NewState()
		require.Zero(t, s.details.Len())
		require.Zero(t, s.localDetails.Len())

		// when
		keys := s.AllRelationKeys()

		// then
		assert.Empty(t, keys)
	})

	t.Run("keys are aggregated from parent state", func(t *testing.T) {
		// given
		s := NewDoc("root", nil).NewState()
		s.AddDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			// details
			bundle.RelationKeyCoverType: domain.Int64(1),
			bundle.RelationKeyName:      domain.String("name"),
			bundle.RelationKeyAssignee:  domain.String("assignee"),
			// local details
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_todo),
			bundle.RelationKeyType:           domain.String(bundle.TypeKeyTask.URL()),
		}))
		require.Equal(t, 3, s.details.Len())
		require.Equal(t, 2, s.localDetails.Len())

		newS := s.NewState()
		require.Empty(t, newS.details)
		require.Empty(t, newS.localDetails)
		require.Equal(t, 3, newS.parent.details.Len())
		require.Equal(t, 2, newS.parent.localDetails.Len())

		// when
		keys := s.AllRelationKeys()

		// then
		assert.Len(t, keys, 5)
	})
}

func TestState_ApplyNullDetailDoesNotGenerateDuplicateChange(t *testing.T) {
	t.Run("apply detailsSet with nil proto value then SetDetail with Null should not produce change", func(t *testing.T) {
		// given
		// Simulate building state from changes: change sets iconOption to null (nil *types.Value in proto)
		doc := NewDoc("root", nil).(*State)
		doc.details = domain.NewDetails()
		doc.details.Set("iconOption", domain.Int64(1))

		// Apply a change that sets iconOption to null (nil proto value, as seen in the tree)
		err := doc.ApplyChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfDetailsSet{
				DetailsSet: &pb.ChangeDetailsSet{
					Key:   "iconOption",
					Value: nil, // null in proto/JSON
				},
			},
		})
		require.NoError(t, err)

		// After applying the change, iconOption should still be present as Null, not deleted
		assert.True(t, doc.Details().Has("iconOption"), "iconOption should still be present in details after applying null change")
		assert.True(t, doc.Details().Get("iconOption").IsNull(), "iconOption should be Null value")

		// Now simulate what happens on startup: SetDetail is called with domain.Null()
		s := doc.NewState()
		s.SetDetail("iconOption", domain.Null())

		// This should NOT produce any changes because the value is already Null
		msgs, _, err := ApplyState("space1", s, false)
		require.NoError(t, err)
		assert.Empty(t, msgs, "setting Null on already-Null detail should not produce events")
		assert.Empty(t, s.GetChanges(), "setting Null on already-Null detail should not produce changes")
	})

	t.Run("repeated null changes should not generate duplicate detailsSet", func(t *testing.T) {
		// given
		// Build state from a sequence of changes like seen in the bug report:
		// change 608: detailsSet iconOption=null
		// change 609: detailsSet iconOption=null (duplicate!)
		doc := NewDoc("root", nil).(*State)
		doc.details = domain.NewDetails()
		doc.details.Set("iconOption", domain.Int64(5))

		// First null change
		err := doc.ApplyChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfDetailsSet{
				DetailsSet: &pb.ChangeDetailsSet{
					Key:   "iconOption",
					Value: nil,
				},
			},
		})
		require.NoError(t, err)

		// After first null change, SetDetail with Null should be a no-op
		s := doc.NewState()
		s.SetDetail("iconOption", domain.Null())

		msgs, _, err := ApplyState("space1", s, false)
		require.NoError(t, err)
		assert.Empty(t, msgs, "first SetDetail(Null) after null change should not produce events")

		// And again — still should be a no-op
		s2 := doc.NewState()
		s2.SetDetail("iconOption", domain.Null())

		msgs, _, err = ApplyState("space1", s2, false)
		require.NoError(t, err)
		assert.Empty(t, msgs, "second SetDetail(Null) should not produce events")
	})
}
