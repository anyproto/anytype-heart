package objectcreator

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type objKey interface {
	URL() string
	BundledURL() string
}

func TestInstaller_queryDeletedObjects(t *testing.T) {
	// given
	var (
		spaceId         = "spaceId"
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
			bundle.RelationKeyId:           pbtypes.String(obj.key.URL()),
			bundle.RelationKeySpaceId:      pbtypes.String(obj.spaceId),
			bundle.RelationKeySourceObject: pbtypes.String(obj.key.BundledURL()),
			bundle.RelationKeyIsDeleted:    pbtypes.Bool(obj.isDeleted),
			bundle.RelationKeyIsArchived:   pbtypes.Bool(obj.isArchived),
			bundle.RelationKeyLayout:       pbtypes.Int64(int64(model.ObjectType_relation)),
		}})
		sourceObjectIds = append(sourceObjectIds, obj.key.BundledURL())
		if obj.spaceId == spaceId && (obj.isDeleted || obj.isArchived) {
			validObjectIds = append(validObjectIds, obj.key.URL())
		}
	}

	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().Id().Return(spaceId)

	s := service{objectStore: store}

	// when
	records, err := s.queryDeletedObjects(spc, sourceObjectIds)

	// then
	assert.NoError(t, err)
	assert.Len(t, records, 6)
	for _, det := range records {
		assert.Contains(t, validObjectIds, pbtypes.GetString(det.Details, bundle.RelationKeyId.String()))
	}
}

func TestInstaller_reinstallObject(t *testing.T) {
	t.Run("reinstall archived object", func(t *testing.T) {
		// given
		sourceDetails := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():      pbtypes.String(bundle.TypeKeyProject.BundledURL()),
			bundle.RelationKeySpaceId.String(): pbtypes.String(addr.AnytypeMarketplaceWorkspace),
			bundle.RelationKeyName.String():    pbtypes.String(bundle.TypeKeyProject.String()),
		}}

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

		oldDetails := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():           pbtypes.String(bundle.TypeKeyProject.URL()),
			bundle.RelationKeySpaceId.String():      pbtypes.String(spaceId),
			bundle.RelationKeySourceObject.String(): pbtypes.String(bundle.TypeKeyProject.BundledURL()),
			bundle.RelationKeyIsArchived.String():   pbtypes.Bool(true),
			bundle.RelationKeyIsDeleted.String():    pbtypes.Bool(false),
			bundle.RelationKeyName.String():         pbtypes.String("Project name was edited"),
		}}

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

		archiver := mock_detailservice.NewMockService(t)
		archiver.EXPECT().SetIsArchived(mock.Anything, mock.Anything).RunAndReturn(func(id string, isArchived bool) error {
			assert.Equal(t, id, bundle.TypeKeyProject.URL())
			assert.False(t, isArchived)
			return nil
		})

		s := service{archiver: archiver}

		// when
		id, _, newDetails, err := s.reinstallObject(nil, market, spc, oldDetails)

		// then
		assert.NoError(t, err)
		assert.Equal(t, bundle.TypeKeyProject.URL(), id)
		assert.Equal(t, bundle.TypeKeyProject.String(), pbtypes.GetString(newDetails, bundle.RelationKeyName.String()))
	})
}
