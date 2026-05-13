package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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

	t.Run("deleting homepage object resets homepage to widgets", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		detailsSvc := mock_detailservice.NewMockService(t)
		spc := mock_clientspace.NewMockSpace(t)
		spc.EXPECT().Id().Return(spaceId)

		s := &Service{
			objectStore:    store,
			detailsService: detailsSvc,
		}

		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String(workspaceId),
				bundle.RelationKeyHomepage: domain.String(objectId),
			},
		})

		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Workspace: workspaceId,
		})

		detailsSvc.EXPECT().SetSpaceInfo(mock.Anything, mock.Anything).RunAndReturn(func(spcId string, details *domain.Details) error {
			assert.Equal(t, spaceId, spcId)
			require.NotNil(t, details)
			require.NotEmpty(t, details)
			assert.Equal(t, domain.HomepageWidgets, details.GetString(bundle.RelationKeyHomepage))
			return nil
		})

		// when
		s.unsetHomepageIfNeeded(domain.FullID{SpaceID: spaceId, ObjectID: objectId}, spc)
	})

	t.Run("deleting object that is NOT the homepage leaves homepage setting unchanged", func(t *testing.T) {
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
				bundle.RelationKeyId:       domain.String(workspaceId),
				bundle.RelationKeyHomepage: domain.String(otherDashboardId),
			},
		})

		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Workspace: workspaceId,
		})

		// when
		s.unsetHomepageIfNeeded(domain.FullID{SpaceID: spaceId, ObjectID: objectId}, spc)

		// then
		detailsSvc.AssertNotCalled(t, "SetSpaceInfo", mock.Anything, mock.Anything)
	})

	t.Run("empty homepage does not trigger unset", func(t *testing.T) {
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
				bundle.RelationKeyId:       domain.String(workspaceId),
				bundle.RelationKeyHomepage: domain.String(""),
			},
		})

		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Workspace: workspaceId,
		})

		// when
		s.unsetHomepageIfNeeded(domain.FullID{SpaceID: spaceId, ObjectID: objectId}, spc)

		// then
		detailsSvc.AssertNotCalled(t, "SetSpaceInfo", mock.Anything, mock.Anything)
	})
}
