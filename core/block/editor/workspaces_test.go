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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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
	t.Run("an invite held by the owner leaves nothing but the marker", func(t *testing.T) {
		// given a workspace, which every member of the space reads
		fx := newWorkspacesFixture(t)
		defer fx.finish()

		// when an invite the owner holds is written to it
		err := fx.SetInviteFileInfo(domain.InviteInfo{
			InviteFileCid: "fileId",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
			HeldByOwner:   true,
		})
		require.NoError(t, err)

		// then the members learn that an invite exists and that it is the owner's to share, and the
		// link itself is nowhere in the object they sync
		require.Equal(t, domain.InviteInfo{HeldByOwner: true}, fx.GetExistingInviteInfo())
		details := fx.CombinedDetails()
		require.Empty(t, details.GetString(bundle.RelationKeySpaceInviteFileCid))
		require.Empty(t, details.GetString(bundle.RelationKeySpaceInviteFileKey))
	})
	t.Run("sharing an invite within the space clears the marker", func(t *testing.T) {
		// given a workspace that carries the marker of an invite the owner held
		fx := newWorkspacesFixture(t)
		defer fx.finish()
		err := fx.SetInviteFileInfo(domain.InviteInfo{InviteFileCid: "oldId", InviteFileKey: "oldKey", HeldByOwner: true})
		require.NoError(t, err)
		want := domain.InviteInfo{
			InviteFileCid: "fileId",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
		}

		// when an invite shared within the space replaces it
		err = fx.SetInviteFileInfo(want)
		require.NoError(t, err)

		// then the members read the invite itself, and nothing tells them to ask the owner for it
		require.Equal(t, want, fx.GetExistingInviteInfo())
		require.False(t, fx.CombinedDetails().GetBool(bundle.RelationKeySpaceInviteHeldByOwner))
	})
	t.Run("removing an invite held by the owner clears the marker", func(t *testing.T) {
		fx := newWorkspacesFixture(t)
		defer fx.finish()
		err := fx.SetInviteFileInfo(domain.InviteInfo{InviteFileCid: "fileId", InviteFileKey: "fileKey", HeldByOwner: true})
		require.NoError(t, err)

		removed, err := fx.RemoveExistingInviteInfo()
		require.NoError(t, err)
		require.Equal(t, domain.InviteInfo{HeldByOwner: true}, removed)
		require.Empty(t, fx.GetExistingInviteInfo())
	})
	t.Run("a cid alongside a stale marker reads as a shared invite", func(t *testing.T) {
		// an old client can write a cid without clearing the held-by-owner marker (it knows about
		// neither). The cid means a shared invite, so the marker must not make it look owner-held.
		fx := newWorkspacesFixture(t)
		defer fx.finish()
		st := fx.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteHeldByOwner, domain.Bool(true))
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileCid, domain.String("fileId"))
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileKey, domain.String("fileKey"))
		require.NoError(t, fx.Apply(st))

		info := fx.GetExistingInviteInfo()
		require.Equal(t, "fileId", info.InviteFileCid)
		require.False(t, info.HeldByOwner)
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
