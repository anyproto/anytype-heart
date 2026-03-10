package block

import (
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

func TestService_unsetDashboardIdIfNeeded(t *testing.T) {
	const (
		spaceId     = "space1"
		workspaceId = "workspace1"
		objectId    = "deletedObj"
	)

	t.Run("deleting object that is current dashboard resets spaceDashboardId", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		detailsSvc := mock_detailservice.NewMockService(t)
		spc := mock_clientspace.NewMockSpace(t)

		s := &Service{
			objectStore:    store,
			detailsService: detailsSvc,
		}

		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String(workspaceId),
				bundle.RelationKeySpaceDashboardId: domain.String(objectId),
			},
		})

		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Workspace: workspaceId,
		})

		detailsSvc.EXPECT().SetWorkspaceDashboardId(nil, workspaceId, "").Return("", nil)

		// when
		s.unsetDashboardIdIfNeeded(domain.FullID{SpaceID: spaceId, ObjectID: objectId}, spc)

		// then
		detailsSvc.AssertCalled(t, "SetWorkspaceDashboardId", nil, workspaceId, "")
	})

	t.Run("deleting object that is NOT the dashboard leaves spaceDashboardId unchanged", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		detailsSvc := mock_detailservice.NewMockService(t)
		spc := mock_clientspace.NewMockSpace(t)

		s := &Service{
			objectStore:    store,
			detailsService: detailsSvc,
		}

		otherDashboardId := "someOtherObject"
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String(workspaceId),
				bundle.RelationKeySpaceDashboardId: domain.String(otherDashboardId),
			},
		})

		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Workspace: workspaceId,
		})

		// when
		s.unsetDashboardIdIfNeeded(domain.FullID{SpaceID: spaceId, ObjectID: objectId}, spc)

		// then
		detailsSvc.AssertNotCalled(t, "SetWorkspaceDashboardId", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("empty dashboard id does not trigger unset", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		detailsSvc := mock_detailservice.NewMockService(t)
		spc := mock_clientspace.NewMockSpace(t)

		s := &Service{
			objectStore:    store,
			detailsService: detailsSvc,
		}

		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String(workspaceId),
				bundle.RelationKeySpaceDashboardId: domain.String(""),
			},
		})

		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Workspace: workspaceId,
		})

		// when
		s.unsetDashboardIdIfNeeded(domain.FullID{SpaceID: spaceId, ObjectID: objectId}, spc)

		// then
		detailsSvc.AssertNotCalled(t, "SetWorkspaceDashboardId", mock.Anything, mock.Anything, mock.Anything)
	})
}
