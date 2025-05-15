package inviteservice

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/mock_domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl/mock_fileacl"
	"github.com/anyproto/anytype-heart/core/invitestore/mock_invitestore"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type mockInviteObject struct {
	smartblock.SmartBlock
	*mock_domain.MockInviteObject
}

func TestInviteService_GetCurrent(t *testing.T) {
	t.Run("get current no migration", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.mockSpaceService.EXPECT().Get(ctx, "spaceId").Return(fx.mockSpace, nil)
		fx.mockSpace.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Workspace: "workspaceId",
		})
		fx.mockSpace.EXPECT().Do("workspaceId", mock.Anything).RunAndReturn(func(s string, f func(smartblock.SmartBlock) error) error {
			return f(mockInviteObject{SmartBlock: smarttest.New("root"), MockInviteObject: fx.mockInviteObject})
		})
		returnedInfo := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
		}
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(returnedInfo)
		info, err := fx.GetCurrent(ctx, "spaceId")
		require.NoError(t, err)
		require.Equal(t, returnedInfo, info)
	})
}

var ctx = context.Background()

type fixture struct {
	*inviteService
	a                  *app.App
	ctrl               *gomock.Controller
	mockInviteStore    *mock_invitestore.MockService
	mockFileAcl        *mock_fileacl.MockService
	mockAccountService *mock_account.MockService
	mockSpaceService   *mock_space.MockService
	mockTechSpace      *mock_techspace.MockTechSpace
	mockSpaceView      *mock_techspace.MockSpaceView
	mockSpace          *mock_clientspace.MockSpace
	mockInviteObject   *mock_domain.MockInviteObject
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(nil)
	mockInviteStore := mock_invitestore.NewMockService(t)
	mockFileAcl := mock_fileacl.NewMockService(t)
	mockAccountService := mock_account.NewMockService(t)
	mockSpaceService := mock_space.NewMockService(t)
	mockTechSpace := mock_techspace.NewMockTechSpace(t)
	mockSpaceView := mock_techspace.NewMockSpaceView(t)
	mockSpace := mock_clientspace.NewMockSpace(t)
	mockInviteObject := mock_domain.NewMockInviteObject(t)
	fx := &fixture{
		inviteService:      New().(*inviteService),
		a:                  new(app.App),
		ctrl:               ctrl,
		mockInviteStore:    mockInviteStore,
		mockFileAcl:        mockFileAcl,
		mockAccountService: mockAccountService,
		mockSpaceService:   mockSpaceService,
		mockTechSpace:      mockTechSpace,
		mockSpaceView:      mockSpaceView,
		mockSpace:          mockSpace,
		mockInviteObject:   mockInviteObject,
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockInviteStore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockFileAcl)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockAccountService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockSpaceService)).
		Register(fx)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}
