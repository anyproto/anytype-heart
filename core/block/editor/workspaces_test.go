package editor

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
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

func newWorkspaceForMigration(t *testing.T, spaceUxType model.SpaceUxType, dashboardId string, chatId string) *Workspaces {
	sb := smarttest.New("workspaceId")
	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).Return(chatId, nil).Maybe()
	sb.SetSpace(spc)

	w := &Workspaces{
		SmartBlock:   sb,
		spaceService: &spaceServiceStub{},
		migrator:     migratorStub{},
		config:       &config.Config{},
	}

	initCtx := &smartblock.InitContext{
		IsNewObject: true,
	}
	require.NoError(t, w.Init(initCtx))
	migration.RunMigrations(w, initCtx)
	require.NoError(t, w.Apply(initCtx.State))

	// Set pre-migration state: version 2 details (simulating existing data before migration 3)
	st := w.NewState()
	st.SetDetail(bundle.RelationKeySpaceUxType, domain.Int64(int64(spaceUxType)))
	st.SetDetail(bundle.RelationKeySpaceDashboardId, domain.String(dashboardId))
	st.SetMigrationVersion(2)
	require.NoError(t, w.Apply(st, smartblock.NoHooks))

	// Run migrations again (version 3 should now execute)
	runCtx := &smartblock.InitContext{
		State: w.NewState(),
	}
	migration.RunMigrations(w, runCtx)
	require.NoError(t, w.Apply(runCtx.State, smartblock.NoHooks))

	return w
}

func TestWorkspaces_StateMigrationV3(t *testing.T) {
	t.Run("Chat space gets homepage=chatId and UxType=Data", func(t *testing.T) {
		w := newWorkspaceForMigration(t, model.SpaceUxType_Chat, "oldDashboard", "derivedChatId")

		details := w.Details()
		assert.Equal(t, "derivedChatId", details.GetString(bundle.RelationKeySpaceDashboardId))
		assert.Equal(t, int64(model.SpaceUxType_Data), details.GetInt64(bundle.RelationKeySpaceUxType))
	})

	t.Run("OneToOne space gets homepage=chatId, UxType stays OneToOne", func(t *testing.T) {
		w := newWorkspaceForMigration(t, model.SpaceUxType_OneToOne, "oldDashboard", "derivedChatId")

		details := w.Details()
		assert.Equal(t, "derivedChatId", details.GetString(bundle.RelationKeySpaceDashboardId))
		assert.Equal(t, int64(model.SpaceUxType_OneToOne), details.GetInt64(bundle.RelationKeySpaceUxType))
	})

	t.Run("Data space with lastOpened gets homepage=widgets", func(t *testing.T) {
		w := newWorkspaceForMigration(t, model.SpaceUxType_Data, "lastOpened", "")

		details := w.Details()
		assert.Equal(t, HomepageWidgets, details.GetString(bundle.RelationKeySpaceDashboardId))
		assert.Equal(t, int64(model.SpaceUxType_Data), details.GetInt64(bundle.RelationKeySpaceUxType))
	})

	t.Run("Data space with custom homepage is unchanged", func(t *testing.T) {
		w := newWorkspaceForMigration(t, model.SpaceUxType_Data, "customObjectId", "")

		details := w.Details()
		assert.Equal(t, "customObjectId", details.GetString(bundle.RelationKeySpaceDashboardId))
		assert.Equal(t, int64(model.SpaceUxType_Data), details.GetInt64(bundle.RelationKeySpaceUxType))
	})
}
