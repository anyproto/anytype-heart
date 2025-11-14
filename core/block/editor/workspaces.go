package editor

import (
	"github.com/anyproto/any-sync/commonspace/object/acl/list"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var workspaceRequiredRelations = []domain.RelationKey{
	// SpaceInviteFileCid and SpaceInviteFileKey are added only when creating invite
}

type Workspaces struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	dataview.Dataview
	stext.Text

	spaceService spaceService
	config       *config.Config
	migrator     subObjectsMigrator
}

func (f *ObjectFactory) newWorkspace(sb smartblock.SmartBlock, store spaceindex.Store) *Workspaces {
	w := &Workspaces{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, store, f.layoutConverter, f.fileObjectService),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			store,
			f.eventSender,
		),
		Dataview:     dataview.NewDataview(sb, store),
		spaceService: f.spaceService,
		config:       f.config,
	}
	w.migrator = &subObjectsMigration{
		workspace: w,
	}
	return w
}

func (w *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, workspaceRequiredRelations...)
	err = w.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	w.initTemplate(ctx)
	w.migrator.migrateSubObjects(ctx.State)
	w.onWorkspaceChanged(ctx.State)
	w.AddHook(w.onApply, smartblock.HookAfterApply)
	return nil
}

func (w *Workspaces) initTemplate(ctx *smartblock.InitContext) {
	if w.config.AnalyticsId != "" {
		ctx.State.SetSetting(state.SettingsAnalyticsId, pbtypes.String(w.config.AnalyticsId))
	} else if ctx.State.GetSetting(state.SettingsAnalyticsId) == nil {
		// add analytics id for existing users, so it will be active from the next start
		log.Warnf("analyticsID is missing, generating new one")
		ctx.State.SetSetting(state.SettingsAnalyticsId, pbtypes.String(metrics.GenerateAnalyticsId()))
	}

	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
		template.WithLayout(model.ObjectType_space),
		template.WithForcedObjectTypes([]domain.TypeKey{bundle.TypeKeySpace}),
	)
}

func (w *Workspaces) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	// TODO Maybe move init logic here?
	return migration.Migration{
		Version: 2,
		Proc: func(s *state.State) {
			// no-op
		},
	}
}

func (w *Workspaces) SetInviteFileInfo(info domain.InviteInfo) (err error) {
	st := w.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInvitePermissions, domain.Int64(domain.ConvertAclPermissions(info.Permissions)))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteType, domain.Int64(info.InviteType))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileCid, domain.String(info.InviteFileCid))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileKey, domain.String(info.InviteFileKey))
	return w.Apply(st)
}

func (w *Workspaces) GetExistingInviteInfo() (inviteInfo domain.InviteInfo) {
	details := w.CombinedDetails()
	inviteInfo.InviteType = domain.InviteType(details.GetInt64(bundle.RelationKeySpaceInviteType))
	// nolint: gosec
	inviteInfo.Permissions = domain.ConvertParticipantPermissions(model.ParticipantPermissions(details.GetInt64(bundle.RelationKeySpaceInvitePermissions)))
	inviteInfo.InviteFileCid = details.GetString(bundle.RelationKeySpaceInviteFileCid)
	inviteInfo.InviteFileKey = details.GetString(bundle.RelationKeySpaceInviteFileKey)
	if inviteInfo.InviteType == domain.InviteTypeDefault {
		inviteInfo.Permissions = list.AclPermissionsNone
	}
	return
}

func (w *Workspaces) RemoveExistingInviteInfo() (info domain.InviteInfo, err error) {
	info = w.GetExistingInviteInfo()
	newState := w.NewState()
	newState.RemoveDetail(
		bundle.RelationKeySpaceInviteFileCid,
		bundle.RelationKeySpaceInviteFileKey,
		bundle.RelationKeySpaceInvitePermissions,
		bundle.RelationKeySpaceInviteType)
	return info, w.Apply(newState)
}

func (w *Workspaces) SetGuestInviteFileInfo(fileCid string, fileKey string) (err error) {
	st := w.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteGuestFileCid, domain.String(fileCid))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteGuestFileKey, domain.String(fileKey))
	return w.Apply(st)
}

func (w *Workspaces) GetExistingGuestInviteInfo() (fileCid string, fileKey string) {
	details := w.CombinedDetails()
	fileCid = details.GetString(bundle.RelationKeySpaceInviteGuestFileCid)
	fileKey = details.GetString(bundle.RelationKeySpaceInviteGuestFileKey)
	return
}

func (w *Workspaces) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{{
		Version: 2,
		Proc: func(s *state.State) {
			spaceUxType, ok := s.Details().TryInt64(bundle.RelationKeySpaceUxType)
			if !ok {
				spaceUxType = int64(model.SpaceUxType_Data)
				s.SetDetail(bundle.RelationKeySpaceUxType, domain.Int64(spaceUxType))
			} else if spaceUxType == 0 {
				// convert old spaceUxType 0 to Chat
				spaceUxType = int64(model.SpaceUxType_Chat)
				s.SetDetail(bundle.RelationKeySpaceUxType, domain.Int64(spaceUxType))
			}
		},
	}})
}

func (w *Workspaces) onApply(info smartblock.ApplyInfo) error {
	allDetails := info.State.CombinedDetails()
	spaceUxType := model.SpaceUxType(allDetails.GetInt64(bundle.RelationKeySpaceUxType))
	isOneToOne := spaceUxType == model.SpaceUxType_OneToOne
	if isOneToOne {
		return nil
	}
	w.onWorkspaceChanged(info.State)
	return nil
}

func (w *Workspaces) onWorkspaceChanged(state *state.State) {
	details := state.CombinedDetails().Copy()
	w.spaceService.OnWorkspaceChanged(w.SpaceID(), details)
}
