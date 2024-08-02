package editor

import (
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

func (f *ObjectFactory) newWorkspace(sb smartblock.SmartBlock) *Workspaces {
	w := &Workspaces{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, f.objectStore, f.layoutConverter, f.fileObjectService),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			f.objectStore,
			f.eventSender,
		),
		Dataview:     dataview.NewDataview(sb, f.objectStore),
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
		template.WithTitle,
		template.WithFeaturedRelations,
		template.WithDetail(bundle.RelationKeyIsHidden, pbtypes.Bool(true)),
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_space))),
		template.WithForcedObjectTypes([]domain.TypeKey{bundle.TypeKeySpace}),
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()})),
	)
}

func (w *Workspaces) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	// TODO Maybe move init logic here?
	return migration.Migration{
		Version: 0,
		Proc: func(s *state.State) {
			// no-op
		},
	}
}

func (w *Workspaces) SetInviteFileInfo(fileCid string, fileKey string) (err error) {
	st := w.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileCid, pbtypes.String(fileCid))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileKey, pbtypes.String(fileKey))
	return w.Apply(st)
}

func (w *Workspaces) GetExistingInviteInfo() (fileCid string, fileKey string) {
	details := w.CombinedDetails()
	fileCid = pbtypes.GetString(details, bundle.RelationKeySpaceInviteFileCid.String())
	fileKey = pbtypes.GetString(details, bundle.RelationKeySpaceInviteFileKey.String())
	return
}

func (w *Workspaces) RemoveExistingInviteInfo() (fileCid string, err error) {
	details := w.Details()
	fileCid = pbtypes.GetString(details, bundle.RelationKeySpaceInviteFileCid.String())
	newState := w.NewState()
	newState.RemoveDetail(bundle.RelationKeySpaceInviteFileCid.String(), bundle.RelationKeySpaceInviteFileKey.String())
	return fileCid, w.Apply(newState)
}

func (w *Workspaces) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func (w *Workspaces) onApply(info smartblock.ApplyInfo) error {
	w.onWorkspaceChanged(info.State)
	return nil
}

func (w *Workspaces) onWorkspaceChanged(state *state.State) {
	details := pbtypes.CopyStruct(state.CombinedDetails(), true)
	w.spaceService.OnWorkspaceChanged(w.SpaceID(), details)
}
