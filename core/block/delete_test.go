package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

// TestService_deleteDerivedObjectKeepsTheIndexRow pins the one thing that
// separates a derived object's delete from an ordinary one: its index row
// is NOT stripped to a tombstone. The row stays the full corpse (name,
// keys, slug) the isUninstalled Apply indexed, which is what the API's
// removed-type and removed-property lookups read and what every other
// device holds. Route the derived path through BeforeDelete again and this
// fails.
func TestService_deleteDerivedObjectKeepsTheIndexRow(t *testing.T) {
	const (
		spaceId    = "space1"
		relationId = "rel-warranty"
	)
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, spaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String(relationId),
		bundle.RelationKeyUniqueKey:      domain.String("rel-6a7663db61fab21cd4b9e201"),
		bundle.RelationKeyRelationKey:    domain.String("6a7663db61fab21cd4b9e201"),
		bundle.RelationKeyApiObjectKey:   domain.String("warranty_until"),
		bundle.RelationKeyName:           domain.String("Warranty until"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	}})
	sb := smarttest.New(relationId)
	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().Id().Return(spaceId).Maybe()
	spc.EXPECT().Do(relationId, mock.Anything).RunAndReturn(func(_ string, apply func(smartblock.SmartBlock) error) error {
		return apply(sb)
	})
	spaceSvc := mock_space.NewMockService(t)
	spaceSvc.EXPECT().Get(mock.Anything, spaceId).Return(spc, nil)
	detailsSvc := mock_detailservice.NewMockService(t)
	detailsSvc.EXPECT().SetIsFavorite(relationId, false).Return(nil)
	s := &Service{objectStore: store, spaceService: spaceSvc, detailsService: detailsSvc}

	err := s.deleteDerivedObject(domain.FullID{SpaceID: spaceId, ObjectID: relationId}, coresb.SmartBlockTypeRelation, spc)

	require.NoError(t, err)
	assert.True(t, sb.IsDeleted(), "marked deleted in memory")
	assert.True(t, sb.Details().GetBool(bundle.RelationKeyIsUninstalled), "uninstalled in the tree")
	row, err := store.SpaceIndex(spaceId).GetDetails(relationId)
	require.NoError(t, err)
	assert.Equal(t, "Warranty until", row.GetString(bundle.RelationKeyName), "the row keeps its details")
	assert.Equal(t, "6a7663db61fab21cd4b9e201", row.GetString(bundle.RelationKeyRelationKey))
	assert.Equal(t, "warranty_until", row.GetString(bundle.RelationKeyApiObjectKey))
	assert.False(t, row.Has(bundle.RelationKeyDeletedSnapshot), "no tombstone was written")
}

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
