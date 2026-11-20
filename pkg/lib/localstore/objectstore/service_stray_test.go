package objectstore

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestStartRemovesStoresOfOffloadedSpacesWithoutStorage(t *testing.T) {
	ctx := context.Background()
	const (
		withStorage = "withStorage" // storage on disk: kept, though its view says offloaded
		offloaded   = "offloaded"   // left, offload finished, no storage: removed
		removing    = "removing"    // being left, offload finished, no storage: removed
		failedLoad  = "failedLoad"  // a member's space whose load failed for good: kept
		leaving     = "leaving"     // left, its offload not finished yet: kept
		joining     = "joining"     // no storage yet, its space view says otherwise: kept
		noSpaceView = "noSpaceView" // no storage, nothing says it is gone: kept
		virtual     = "virtual"     // no storage by design: kept, though its view says offloaded
	)
	spaceView := func(target string, account, local model.SpaceStatus) TestObject {
		return TestObject{
			bundle.RelationKeyId:                 domain.String("view-" + target),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:      domain.String(target),
			bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(account)),
			bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(local)),
		}
	}
	// a local-only detail of an object in a kept store, which only that store holds
	lastOpened := TestObject{
		bundle.RelationKeyId:             domain.String("object"),
		bundle.RelationKeyLastOpenedDate: domain.Int64(1700000000),
	}
	// seed leaves, under a repository, a store (index and CRDT databases) for each space, the space
	// views and the virtual space registration, all closed, as a previous run would
	seed := func(t *testing.T) string {
		root := t.TempDir()
		fx := newStoreFixtureInDir(t, root)
		require.NoError(t, fx.WaitStoresLoaded(ctx))
		for _, id := range []string{withStorage, offloaded, removing, failedLoad, leaving, joining, noSpaceView, virtual, addr.AnytypeMarketplaceWorkspace} {
			_, err := fx.anystoreProvider.GetSpaceIndexDb(id)
			require.NoError(t, err)
			_, err = fx.anystoreProvider.GetCrdtDb(id).Wait()
			require.NoError(t, err)
		}
		require.NoError(t, fx.SaveVirtualSpace(virtual))
		fx.AddObjects(t, withStorage, []TestObject{lastOpened})
		fx.AddObjects(t, leaving, []TestObject{lastOpened})
		fx.AddObjects(t, TestTechSpaceId, []TestObject{
			spaceView(withStorage, model.SpaceStatus_SpaceDeleted, model.SpaceStatus_Missing),
			spaceView(offloaded, model.SpaceStatus_SpaceDeleted, model.SpaceStatus_Missing),
			spaceView(removing, model.SpaceStatus_SpaceRemoving, model.SpaceStatus_Missing),
			spaceView(failedLoad, model.SpaceStatus_SpaceActive, model.SpaceStatus_Missing),
			spaceView(leaving, model.SpaceStatus_SpaceDeleted, model.SpaceStatus_Loading),
			spaceView(joining, model.SpaceStatus_SpaceJoining, model.SpaceStatus_Loading),
			spaceView(virtual, model.SpaceStatus_SpaceDeleted, model.SpaceStatus_Missing),
			spaceView(addr.AnytypeMarketplaceWorkspace, model.SpaceStatus_SpaceDeleted, model.SpaceStatus_Missing),
		})
		require.NoError(t, fx.Close(ctx))
		require.NoError(t, fx.anystoreProvider.Close(ctx))
		return root
	}
	start := func(t *testing.T, root string, storage app.Component) (*StoreFixture, []string) {
		fx := newStoreFixtureInDir(t, root, storage)
		require.NoError(t, fx.WaitStoresLoaded(ctx))
		ids, err := fx.anystoreProvider.ListSpaceIdsFromFilesystem()
		require.NoError(t, err)
		return fx, ids
	}
	all := []string{withStorage, offloaded, removing, failedLoad, leaving, joining, noSpaceView, virtual, addr.AnytypeMarketplaceWorkspace, TestTechSpaceId}

	t.Run("only a left or removed space without storage loses its store", func(t *testing.T) {
		// given
		root := seed(t)

		// when
		fx, onDisk := start(t, root, &spaceIdsListerStub{ids: []string{withStorage, TestTechSpaceId}})

		// then
		assert.ElementsMatch(t, []string{withStorage, failedLoad, leaving, joining, noSpaceView, virtual, addr.AnytypeMarketplaceWorkspace, TestTechSpaceId}, onDisk)
		// the kept stores are the ones the previous run wrote, not ones created again
		for _, id := range []string{withStorage, leaving} {
			details, err := fx.SpaceIndex(id).GetDetails("object")
			require.NoError(t, err)
			assert.Equal(t, int64(1700000000), details.GetInt64(bundle.RelationKeyLastOpenedDate), id)
		}
	})

	t.Run("an empty storage listing removes nothing", func(t *testing.T) {
		// given
		root := seed(t)

		// when
		_, onDisk := start(t, root, &spaceIdsListerStub{})

		// then
		assert.ElementsMatch(t, all, onDisk)
	})
}
