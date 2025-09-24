package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestState_FileRelationKeys(t *testing.T) {
	getter := objectstore.NewStoreFixture(t).SpaceIndex("space")
	t.Run("no file relations", func(t *testing.T) {
		// given
		s := &State{}

		// when
		keys := s.FileRelationKeys(getter)

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
		keys := s.FileRelationKeys(getter)

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
		keys := s.FileRelationKeys(getter)

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
		keys := s.FileRelationKeys(getter)

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
		keys := s.FileRelationKeys(getter)

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
		keys := s.FileRelationKeys(getter)

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
		keys := s.FileRelationKeys(getter)

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
		keys := s.FileRelationKeys(getter)

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

func TestState_AddRelationKeys(t *testing.T) {
	t.Run("add new keys", func(t *testing.T) {
		// given
		st := NewDoc("root", nil).NewState()
		st.AddDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			// details
			bundle.RelationKeyCoverType: domain.Int64(1),
			bundle.RelationKeyName:      domain.String("name"),
			bundle.RelationKeyAssignee:  domain.String("assignee"),
			// local details
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_todo),
			bundle.RelationKeyType:           domain.String(bundle.TypeKeyTask.URL()),
		}))
		require.Equal(t, 3, st.details.Len())
		require.Equal(t, 2, st.localDetails.Len())

		// when
		st.AddRelationKeys(
			bundle.RelationKeyPicture, // new detail
			bundle.RelationKeyName,    // existing detail
			bundle.RelationKeySpaceId, // new local detail
			bundle.RelationKeyType,    // existing local detail
		)

		// then
		assert.Equal(t, 4, st.details.Len())
		assert.Equal(t, 3, st.localDetails.Len())
		assert.Equal(t, domain.Null(), st.details.Get(bundle.RelationKeyPicture))
		assert.Equal(t, domain.String("name"), st.details.Get(bundle.RelationKeyName))
		assert.Equal(t, domain.Null(), st.localDetails.Get(bundle.RelationKeySpaceId))
		assert.Equal(t, domain.Int64(model.ObjectType_todo), st.localDetails.Get(bundle.RelationKeyResolvedLayout))
	})
}
