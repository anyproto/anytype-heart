package invitemigrator

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain/mock_domain"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus/mock_spacestatus"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type mockInviteObject struct {
	smartblock.SmartBlock
	*mock_domain.MockInviteObject
}

func TestInviteMigrator(t *testing.T) {
	t.Run("migrate existing invites", func(t *testing.T) {
		fx := newFixture(t)
		fx.mockStatus.EXPECT().GetSpaceView().Return(fx.mockSpaceView)
		fx.mockSpaceView.EXPECT().Lock()
		fx.mockSpaceView.EXPECT().Unlock()
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return("fileCid", "fileKey")
		fx.mockSpaceView.EXPECT().RemoveExistingInviteInfo().Return("fileCid", nil)
		fx.mockSpaceObject.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Workspace: "workspaceId",
		})
		fx.mockSpaceObject.EXPECT().Do("workspaceId", mock.Anything).RunAndReturn(func(s string, f func(smartblock.SmartBlock) error) error {
			return f(mockInviteObject{SmartBlock: smarttest.New("root"), MockInviteObject: fx.mockInviteObject})
		})
		fx.mockInviteObject.EXPECT().SetInviteFileInfo("fileCid", "fileKey").Return(nil)
		err := fx.MigrateExistingInvites(fx.mockSpaceObject)
		require.NoError(t, err)
	})
	t.Run("migrate existing invites empty spaceview", func(t *testing.T) {
		fx := newFixture(t)
		fx.mockStatus.EXPECT().GetSpaceView().Return(fx.mockSpaceView)
		fx.mockSpaceView.EXPECT().Lock()
		fx.mockSpaceView.EXPECT().Unlock()
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return("", "")
		err := fx.MigrateExistingInvites(fx.mockSpaceObject)
		require.NoError(t, err)
	})
}

var ctx = context.Background()

type fixture struct {
	*inviteMigrator
	a                *app.App
	mockStatus       *mock_spacestatus.MockSpaceStatus
	mockSpaceView    *mock_techspace.MockSpaceView
	mockInviteObject *mock_domain.MockInviteObject
	mockSpaceObject  *mock_clientspace.MockSpace
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		inviteMigrator:   New().(*inviteMigrator),
		a:                new(app.App),
		mockStatus:       mock_spacestatus.NewMockSpaceStatus(t),
		mockSpaceView:    mock_techspace.NewMockSpaceView(t),
		mockInviteObject: mock_domain.NewMockInviteObject(t),
		mockSpaceObject:  mock_clientspace.NewMockSpace(t),
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockStatus)).
		Register(fx)

	require.NoError(t, fx.a.Start(ctx))
	return fx
}
