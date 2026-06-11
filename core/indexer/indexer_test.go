package indexer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/participants"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

func TestRemoveAclIndexes(t *testing.T) {
	t.Run("clears participant records and the processed acl head marker", func(t *testing.T) {
		// given
		fx := newFixture(t)
		spaceIndex := fx.store.SpaceIndex("spaceId1")
		participantId := domain.NewParticipantId("spaceId1", "identity1")
		err := spaceIndex.UpdateObjectDetails(ctx, participantId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:           domain.String("John"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
		}))
		require.NoError(t, err)
		err = spaceIndex.SaveLastIndexedHeadsHash(ctx, participants.AclHeadMarkerId, "aclHeadId")
		require.NoError(t, err)

		// when
		err = fx.RemoveAclIndexes("spaceId1")

		// then
		require.NoError(t, err)
		marker, err := spaceIndex.GetLastIndexedHeadsHash(ctx, participants.AclHeadMarkerId)
		require.NoError(t, err)
		assert.Empty(t, marker)
		details, err := spaceIndex.GetDetails(participantId)
		require.NoError(t, err)
		assert.Zero(t, details.Len())
	})
}

func TestIndexStoreOwnedDetails(t *testing.T) {
	t.Run("participant details are not written back from smartblock state", func(t *testing.T) {
		// given
		fx := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Id().Return("spaceId1").Maybe()
		spaceIndex := fx.store.SpaceIndex("spaceId1")
		participantId := domain.NewParticipantId("spaceId1", "identity1")
		err := spaceIndex.UpdateObjectDetails(ctx, participantId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("fresh name"),
		}))
		require.NoError(t, err)

		// when: a stale smartblock state reaches the indexer (e.g. open-time apply)
		err = fx.Index(smartblock.DocInfo{
			Id:             participantId,
			Space:          space,
			SmartblockType: coresb.SmartBlockTypeParticipant,
			Heads:          []string{"head"},
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String(participantId),
				bundle.RelationKeyName: domain.String("stale name"),
			}),
		})

		// then: the store record is owned by dedicated writers and stays untouched
		require.NoError(t, err)
		details, err := spaceIndex.GetDetails(participantId)
		require.NoError(t, err)
		assert.Equal(t, "fresh name", details.GetString(bundle.RelationKeyName))
	})
}

func TestIndexer(t *testing.T) {
	space := mock_clientspace.NewMockSpace(t)
	space.EXPECT().Id().Return("spaceId1").Maybe()
	space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: "workspaceId1"}).Maybe()

	for _, testCase := range []struct {
		name    string
		options smartblock.IndexOption
	}{
		{
			name:    "SkipFullTextIfHeadsNotChanged",
			options: smartblock.SkipFullTextIfHeadsNotChanged,
		},
		{
			name:    "SkipIfHeadsNotChanged",
			options: smartblock.SkipIfHeadsNotChanged,
		},
	} {
		t.Run("index has not started - when hashes match and "+testCase.name, func(t *testing.T) {
			// given
			indexerFx := newFixture(t)
			smartTest := smarttest.New("objectId1")
			smartTest.SetSpaceId("spaceId1")
			smartTest.SetSpace(space)
			smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"to index",
						blockbuilder.ID("blockId1"),
					),
				)))

			smartTest.SetType(coresb.SmartBlockTypePage)
			indexerFx.store.SpaceIndex("spaceId1").SaveLastIndexedHeadsHash(ctx, "objectId1", "7f40bc2814f5297818461f889780a870ea033fe64c5a261117f2b662515a3dba")

			// when
			err := indexerFx.Index(smartTest.GetDocInfo(), testCase.options)

			// then
			assert.NoError(t, err)
			count, _ := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
			assert.Equal(t, 0, len(count))
		})

		t.Run("index has started - when hashes don't match and key "+testCase.name, func(t *testing.T) {
			// given
			indexerFx := newFixture(t)
			smartTest := smarttest.New("objectId1")
			smartTest.SetSpaceId("spaceId1")
			smartTest.SetSpace(space)
			smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"to index",
						blockbuilder.ID("blockId1"),
					),
				)))

			smartTest.SetType(coresb.SmartBlockTypePage)
			indexerFx.store.SpaceIndex("spaceId1").SaveLastIndexedHeadsHash(ctx, "objectId1", "randomHash")

			// when
			err := indexerFx.Index(smartTest.GetDocInfo(), testCase.options)

			// then
			assert.NoError(t, err)
			count, _ := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
			assert.Equal(t, 1, len(count))
		})
	}

	t.Run("ftQueueCtr is saved when object is added to fulltext queue", func(t *testing.T) {
		// given
		indexerFx := newFixture(t)
		smartTest := smarttest.New("objectId1")
		smartTest.SetSpaceId("spaceId1")
		smartTest.SetSpace(space)
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"to index",
					blockbuilder.ID("blockId1"),
				),
			)))

		smartTest.SetType(coresb.SmartBlockTypePage)
		info := smartTest.GetDocInfo()
		info.Details = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("test"),
		})

		// when
		err := indexerFx.Index(info)

		// then
		assert.NoError(t, err)
		entries, err := indexerFx.store.SpaceIndex("spaceId1").GetHeadsWithFtQueueCtrGreaterThan(ctx, 0)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "objectId1", entries[0].ObjectID)
		assert.Greater(t, entries[0].FTQueueCtr, uint64(0))
	})

	t.Run("ftQueueCtr is not saved when object is not added to fulltext queue", func(t *testing.T) {
		// given
		indexerFx := newFixture(t)
		smartTest := smarttest.New("objectId1")
		smartTest.SetSpaceId("spaceId1")
		smartTest.SetSpace(space)
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"to index",
					blockbuilder.ID("blockId1"),
				),
			)))

		// SmartBlockTypeWorkspace has fulltext=false, so it won't be added to FT queue
		smartTest.SetType(coresb.SmartBlockTypeWorkspace)
		info := smartTest.GetDocInfo()
		info.Details = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("test"),
		})

		// when
		err := indexerFx.Index(info)

		// then
		assert.NoError(t, err)
		entries, err := indexerFx.store.SpaceIndex("spaceId1").GetHeadsWithFtQueueCtrGreaterThan(ctx, 0)
		require.NoError(t, err)
		assert.Len(t, entries, 0)
	})

	t.Run("index has started - when hashes match and options are not provided", func(t *testing.T) {
		// given
		indexerFx := newFixture(t)
		smartTest := smarttest.New("objectId1")
		smartTest.SetSpaceId("spaceId1")
		smartTest.SetSpace(space)
		smartTest.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"to index",
					blockbuilder.ID("blockId1"),
				),
			)))

		smartTest.SetType(coresb.SmartBlockTypePage)
		indexerFx.store.SpaceIndex("spaceId1").SaveLastIndexedHeadsHash(ctx, "objectId1", "7f40bc2814f5297818461f889780a870ea033fe64c5a261117f2b662515a3dba")

		// when
		err := indexerFx.Index(smartTest.GetDocInfo())

		// then
		assert.NoError(t, err)
		count, _ := indexerFx.store.ListIdsFromFullTextQueue([]string{"spaceId1"}, 0)
		assert.Equal(t, 1, len(count))
	})
}
