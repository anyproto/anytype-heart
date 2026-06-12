package participants

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testSpaceId = "space1"

func participantDetails(t *testing.T, store *objectstore.StoreFixture, id string) *domain.Details {
	details, err := store.SpaceIndex(testSpaceId).GetDetails(id)
	require.NoError(t, err)
	return details
}

func fulltextQueueIds(t *testing.T, store *objectstore.StoreFixture) []string {
	queued, err := store.ListIdsFromFullTextQueue([]string{testSpaceId}, 100)
	require.NoError(t, err)
	ids := make([]string, 0, len(queued))
	for _, obj := range queued {
		ids = append(ids, obj.ObjectId)
	}
	return ids
}

func addParticipantRecord(t *testing.T, store *objectstore.StoreFixture, id string) {
	store.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:                     domain.String(id),
		bundle.RelationKeySpaceId:                domain.String(testSpaceId),
		bundle.RelationKeyResolvedLayout:         domain.Int64(model.ObjectType_participant),
		bundle.RelationKeyParticipantPermissions: domain.Int64(model.ParticipantPermissions_Writer),
	}})
}

func TestModifyDetails(t *testing.T) {
	t.Run("identity profile merges into existing record and enqueues fulltext", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		id := domain.NewParticipantId(testSpaceId, "identity1")
		addParticipantRecord(t, store, id)

		// when
		err := ModifyDetails(context.Background(), store, testSpaceId, id, nil, BuildIdentityDetails(&model.IdentityProfile{
			Identity:    "identity1",
			Name:        "John",
			Description: "description",
			IconCid:     "icon",
			GlobalName:  "john.any",
		}))

		// then
		require.NoError(t, err)
		details := participantDetails(t, store, id)
		assert.Equal(t, "John", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "description", details.GetString(bundle.RelationKeyDescription))
		assert.Equal(t, "icon", details.GetString(bundle.RelationKeyIconImage))
		assert.Equal(t, "john.any", details.GetString(bundle.RelationKeyGlobalName))
		// existing details are preserved by the merge
		assert.Equal(t, int64(model.ParticipantPermissions_Writer), details.GetInt64(bundle.RelationKeyParticipantPermissions))
		assert.Equal(t, []string{id}, fulltextQueueIds(t, store))
	})

	t.Run("update-only call leaves missing record absent", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		id := domain.NewParticipantId(testSpaceId, "identity1")

		// when
		err := ModifyDetails(context.Background(), store, testSpaceId, id, nil, BuildIdentityDetails(&model.IdentityProfile{
			Identity: "identity1",
			Name:     "John",
		}))

		// then
		require.NoError(t, err)
		assert.Zero(t, participantDetails(t, store, id).Len())
		assert.Empty(t, fulltextQueueIds(t, store))
	})

	t.Run("missing record is created from base details", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		id := domain.NewParticipantId(testSpaceId, "identity1")
		base := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String(id),
			bundle.RelationKeySpaceId:        domain.String(testSpaceId),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
		})
		newDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("John"),
		})

		// when
		err := ModifyDetails(context.Background(), store, testSpaceId, id, base, newDetails)

		// then
		require.NoError(t, err)
		details := participantDetails(t, store, id)
		assert.Equal(t, "John", details.GetString(bundle.RelationKeyName))
		assert.Equal(t, int64(model.ObjectType_participant), details.GetInt64(bundle.RelationKeyResolvedLayout))
		assert.Equal(t, []string{id}, fulltextQueueIds(t, store))
		boundSpaceId, err := store.GetSpaceId(id)
		require.NoError(t, err)
		assert.Equal(t, testSpaceId, boundSpaceId)
	})

	t.Run("global name only change does not enqueue fulltext", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		id := domain.NewParticipantId(testSpaceId, "identity1")
		addParticipantRecord(t, store, id)
		profile := &model.IdentityProfile{Identity: "identity1", Name: "John", GlobalName: "john.any"}
		require.NoError(t, ModifyDetails(context.Background(), store, testSpaceId, id, nil, BuildIdentityDetails(profile)))
		require.NoError(t, store.ClearFullTextQueue(nil))
		profile.GlobalName = "johnny.any"

		// when
		err := ModifyDetails(context.Background(), store, testSpaceId, id, nil, BuildIdentityDetails(profile))

		// then
		require.NoError(t, err)
		assert.Equal(t, "johnny.any", participantDetails(t, store, id).GetString(bundle.RelationKeyGlobalName))
		assert.Empty(t, fulltextQueueIds(t, store))
	})

	t.Run("identical update is a no-op", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		id := domain.NewParticipantId(testSpaceId, "identity1")
		addParticipantRecord(t, store, id)
		details := BuildIdentityDetails(&model.IdentityProfile{Identity: "identity1", Name: "John"})
		require.NoError(t, ModifyDetails(context.Background(), store, testSpaceId, id, nil, details))
		require.NoError(t, store.ClearFullTextQueue(nil))

		// when
		err := ModifyDetails(context.Background(), store, testSpaceId, id, nil, details)

		// then
		require.NoError(t, err)
		assert.Empty(t, fulltextQueueIds(t, store))
	})
}
