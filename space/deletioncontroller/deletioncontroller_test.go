package deletioncontroller

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient/mock_aclclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/space/deletioncontroller/mock_deletioncontroller"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

func TestDeletionController_Loop(t *testing.T) {
	t.Run("space to delete for owner/non-owner, space not exists", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		fx.AddSpaceToDelete("spaceId1")
		fx.AddSpaceToDelete("spaceId2")
		payloads := []*coordinatorproto.SpaceStatusPayload{
			{
				Status:      coordinatorproto.SpaceStatus_SpaceStatusCreated,
				Permissions: coordinatorproto.SpacePermissions_SpacePermissionsOwner,
				Limits: &coordinatorproto.SpaceLimits{
					ReadMembers:  10,
					WriteMembers: 15,
				},
				IsShared: true,
			},
			{
				Status:      coordinatorproto.SpaceStatus_SpaceStatusCreated,
				Permissions: coordinatorproto.SpacePermissions_SpacePermissionsUnknown,
				IsShared:    false,
			},
			{
				Status: coordinatorproto.SpaceStatus_SpaceStatusNotExists,
			},
		}
		limits := &coordinatorproto.AccountLimits{SharedSpacesLimit: 10}
		fx.mockSpaceManager.EXPECT().AllSpaceIds().Return([]string{"spaceId1", "spaceId2", "spaceId3"})
		fx.mockClient.EXPECT().StatusCheckMany(ctx, []string{"spaceId1", "spaceId2", "spaceId3"}).Return(payloads, limits, nil)
		firstStatus := spaceinfo.NewSpaceLocalInfo("spaceId1")
		firstStatus.
			SetRemoteStatus(spaceinfo.RemoteStatusOk).
			SetShareableStatus(spaceinfo.ShareableStatusShareable).
			SetReadLimit(10).
			SetWriteLimit(15)
		secondStatus := spaceinfo.NewSpaceLocalInfo("spaceId2")
		secondStatus.
			SetRemoteStatus(spaceinfo.RemoteStatusOk).
			SetShareableStatus(spaceinfo.ShareableStatusNotShareable)
		fx.mockSpaceManager.EXPECT().UpdateSharedLimits(ctx, 10).Return(nil)
		fx.mockSpaceManager.EXPECT().UpdateRemoteStatus(ctx, spaceinfo.SpaceRemoteStatusInfo{
			IsOwned:   true,
			LocalInfo: firstStatus,
		}).Return(nil)
		fx.mockSpaceManager.EXPECT().UpdateRemoteStatus(ctx, spaceinfo.SpaceRemoteStatusInfo{
			IsOwned:   false,
			LocalInfo: secondStatus,
		}).Return(nil)
		fx.mockSpaceCore.EXPECT().Delete(ctx, "spaceId1").Return(nil)
		err := fx.loopIterate(ctx)
		require.NoError(t, err)
		require.NotContains(t, fx.toDelete, "spaceId1")
	})
	t.Run("nil limits", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockSpaceManager.EXPECT().AllSpaceIds().Return([]string{})
		fx.mockClient.EXPECT().StatusCheckMany(ctx, []string{}).Return(nil, nil, nil)
		err := fx.loopIterate(ctx)
		require.NoError(t, err)
	})
}

type fixture struct {
	*deletionController
	a                 *app.App
	ctrl              *gomock.Controller
	mockClient        *mock_coordinatorclient.MockCoordinatorClient
	mockSpaceCore     *mock_spacecore.MockSpaceCoreService
	mockJoiningClient *mock_aclclient.MockAclJoiningClient
	mockSpaceManager  *mock_deletioncontroller.MockSpaceManager
}

func (fx *fixture) Run(ctx context.Context) error {
	fx.deletionController.updater = newUpdateLoop(fx.loopIterate, loopInterval, loopTimeout)
	return nil
}

func (fx *fixture) Close(ctx context.Context) error {
	return nil
}

func (fx *fixture) finish(t *testing.T) {
	fx.ctrl.Finish()
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		deletionController: New().(*deletionController),
		a:                  new(app.App),
		ctrl:               ctrl,
		mockClient:         mock_coordinatorclient.NewMockCoordinatorClient(ctrl),
		mockSpaceCore:      mock_spacecore.NewMockSpaceCoreService(t),
		mockJoiningClient:  mock_aclclient.NewMockAclJoiningClient(ctrl),
		mockSpaceManager:   mock_deletioncontroller.NewMockSpaceManager(t),
	}

	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockSpaceCore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockJoiningClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockSpaceManager)).
		Register(fx)
	require.NoError(t, fx.a.Start(ctx))

	return fx
}
