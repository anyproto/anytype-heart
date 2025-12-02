package objectcreator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

type objKey interface {
	URL() string
	BundledURL() string
}

func TestInstaller_queryDeletedObjects(t *testing.T) {
	// given
	var (
		sourceObjectIds = []string{}
		validObjectIds  = []string{}
	)

	store := objectstore.NewStoreFixture(t)

	for _, obj := range []struct {
		isDeleted, isArchived bool
		spaceId               string
		key                   objKey
	}{
		{false, false, spaceId, bundle.TypeKeyGoal},
		{false, false, spaceId, bundle.RelationKeyGenre},
		{true, false, spaceId, bundle.TypeKeyTask},
		{true, false, spaceId, bundle.RelationKeyLinkedProjects},
		{false, true, spaceId, bundle.TypeKeyBook},
		{false, true, spaceId, bundle.RelationKeyStarred},
		{true, true, spaceId, bundle.TypeKeyProject},    // not valid, but we should handle this
		{true, true, spaceId, bundle.RelationKeyArtist}, // not valid, but we should handle this
		{false, true, "otherSpaceId", bundle.TypeKeyDiaryEntry},
		{true, false, "otherSpaceId", bundle.RelationKeyAudioAlbum},
	} {
		store.AddObjects(t, obj.spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String(obj.key.URL()),
			bundle.RelationKeySpaceId:        domain.String(obj.spaceId),
			bundle.RelationKeySourceObject:   domain.String(obj.key.BundledURL()),
			bundle.RelationKeyIsDeleted:      domain.Bool(obj.isDeleted),
			bundle.RelationKeyIsArchived:     domain.Bool(obj.isArchived),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_relation),
		}})
		sourceObjectIds = append(sourceObjectIds, obj.key.BundledURL())
		if obj.spaceId == spaceId && (obj.isDeleted || obj.isArchived) {
			validObjectIds = append(validObjectIds, obj.key.URL())
		}
	}

	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().Id().Return(spaceId)

	i := service{objectStore: store}

	// when
	records, err := i.queryDeletedObjects(spc, sourceObjectIds)

	// then
	assert.NoError(t, err)
	assert.Len(t, records, 6)
	for _, det := range records {
		assert.Contains(t, validObjectIds, det.Details.GetString(bundle.RelationKeyId))
	}
}

func TestInstaller_reinstallObject(t *testing.T) {
	t.Run("reinstall archived object", func(t *testing.T) {
		// given
		sourceDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:        domain.String(bundle.TypeKeyProject.BundledURL()),
			bundle.RelationKeySpaceId:   domain.String(addr.AnytypeMarketplaceWorkspace),
			bundle.RelationKeyName:      domain.String(bundle.TypeKeyProject.String()),
			bundle.RelationKeyUniqueKey: domain.String(bundle.TypeKeyProject.URL()),
		})

		sourceObject := smarttest.New(bundle.TypeKeyProject.BundledURL())
		st := sourceObject.NewState()
		st.SetDetails(sourceDetails)
		require.NoError(t, sourceObject.Apply(st))

		market := mock_clientspace.NewMockSpace(t)
		market.EXPECT().Id().Return(addr.AnytypeMarketplaceWorkspace).Maybe()
		market.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(func(id string, apply func(smartblock.SmartBlock) error) error {
			assert.Equal(t, id, bundle.TypeKeyProject.BundledURL())
			return apply(sourceObject)
		})

		oldDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:           domain.String(bundle.TypeKeyProject.URL()),
			bundle.RelationKeySpaceId:      domain.String(spaceId),
			bundle.RelationKeySourceObject: domain.String(bundle.TypeKeyProject.BundledURL()),
			bundle.RelationKeyIsArchived:   domain.Bool(true),
			bundle.RelationKeyIsDeleted:    domain.Bool(false),
			bundle.RelationKeyName:         domain.String("Project name was edited"),
		})

		archivedObject := smarttest.New(bundle.TypeKeyProject.URL())
		st = archivedObject.NewState()
		st.SetDetails(oldDetails)
		require.NoError(t, archivedObject.Apply(st))

		spc := mock_clientspace.NewMockSpace(t)
		spc.EXPECT().Id().Return(spaceId).Maybe()
		spc.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(func(id string, apply func(smartblock.SmartBlock) error) error {
			assert.Equal(t, id, bundle.TypeKeyProject.URL())
			return apply(archivedObject)
		})
		spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, key domain.UniqueKey) (string, error) {
			return domain.RelationKey(key.InternalKey()).URL(), nil
		})
		spc.EXPECT().IsReadOnly().Return(true)

		archiver := mock_detailservice.NewMockService(t)
		archiver.EXPECT().SetIsArchived(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, id string, isArchived bool) error {
			assert.Equal(t, id, bundle.TypeKeyProject.URL())
			assert.False(t, isArchived)
			return nil
		})

		i := service{archiver: archiver}

		// when
		id, _, newDetails, err := i.reinstallObject(nil, market, spc, oldDetails)

		// then
		assert.NoError(t, err)
		assert.Equal(t, bundle.TypeKeyProject.URL(), id)
		assert.Equal(t, bundle.TypeKeyProject.String(), newDetails.GetString(bundle.RelationKeyName))
	})
}
