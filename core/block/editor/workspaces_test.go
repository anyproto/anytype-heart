package editor

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
)

func TestWorkspaces_FileInfo(t *testing.T) {
	t.Run("file info add remove", func(t *testing.T) {
		fx := newWorkspacesFixture(t)
		defer fx.finish()
		info := domain.InviteInfo{
			InviteFileCid: "fileId",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
		}
		err := fx.SetInviteFileInfo(info)
		require.NoError(t, err)
		returnedInfo := fx.GetExistingInviteInfo()
		require.Equal(t, info, returnedInfo)
		returnedInfo, err = fx.RemoveExistingInviteInfo()
		require.NoError(t, err)
		require.Equal(t, info, returnedInfo)
		returnedInfo, err = fx.RemoveExistingInviteInfo()
		require.NoError(t, err)
		require.Empty(t, returnedInfo)
	})
	t.Run("file info empty", func(t *testing.T) {
		fx := newWorkspacesFixture(t)
		defer fx.finish()
		fileId, err := fx.RemoveExistingInviteInfo()
		require.NoError(t, err)
		require.Empty(t, fileId)
	})
}

type migratorStub struct {
}

func (m migratorStub) migrateSubObjects(st *state.State) {
}

func NewWorkspacesTest(ctrl *gomock.Controller) (*Workspaces, error) {
	sb := smarttest.New("root")
	a := &Workspaces{
		SmartBlock:   sb,
		spaceService: &spaceServiceStub{},
		migrator:     migratorStub{},
		config:       &config.Config{},
	}
	initCtx := &smartblock.InitContext{
		IsNewObject: true,
	}
	if err := a.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(a, initCtx)
	if err := a.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return a, nil
}

type workspacesFixture struct {
	*Workspaces
	ctrl *gomock.Controller
}

func newWorkspacesFixture(t *testing.T) *workspacesFixture {
	ctrl := gomock.NewController(t)
	a, err := NewWorkspacesTest(ctrl)
	require.NoError(t, err)
	return &workspacesFixture{
		Workspaces: a,
		ctrl:       ctrl,
	}
}

func (f *workspacesFixture) finish() {
	f.ctrl.Finish()
}
