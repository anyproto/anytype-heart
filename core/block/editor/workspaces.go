package editor

import (
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Workspaces struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	dataview.Dataview
	stext.Text

	DetailsModifier DetailsModifier
	sourceService   source.Service
	anytype         core.Service
	objectStore     objectstore.ObjectStore
	config          *config.Config
	objectDeriver   objectDeriver
}

func NewWorkspace(
	sb smartblock.SmartBlock,
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	systemObjectService system_object.Service,
	sourceService source.Service,
	modifier DetailsModifier,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	config *config.Config,
	eventSender event.Sender,
	objectDeriver objectDeriver,
) *Workspaces {
	return &Workspaces{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, systemObjectService, layoutConverter),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			objectStore,
			eventSender,
		),
		Dataview: dataview.NewDataview(
			sb,
			anytype,
			objectStore,
			systemObjectService,
			sbtProvider,
		),
		DetailsModifier: modifier,
		anytype:         anytype,
		objectStore:     objectStore,
		sourceService:   sourceService,
		config:          config,
		objectDeriver:   objectDeriver,
	}
}

func (w *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	err = w.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	w.initTemplate(ctx)

	subObjectMigration := subObjectsMigration{
		workspace:     w,
		objectDeriver: w.objectDeriver,
	}
	subObjectMigration.migrateSubObjects(ctx.State)

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
		template.WithDetail(bundle.RelationKeySpaceAccessibility, pbtypes.Int64(0)),
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_space))),
		template.WithForcedObjectTypes([]domain.TypeKey{bundle.TypeKeySpace}),
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()})),
		template.WithForcedDetail(bundle.RelationKeyCreator, pbtypes.String(w.anytype.PredefinedObjects(w.SpaceID()).Profile)),
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

func (w *Workspaces) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}
