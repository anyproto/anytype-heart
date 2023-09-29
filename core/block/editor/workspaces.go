package editor

import (
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"context"
	"github.com/gogo/protobuf/types"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"fmt"
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
)

type Workspaces struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	dataview.Dataview
	stext.Text

	DetailsModifier DetailsModifier
	templateCloner  templateCloner
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
	templateCloner templateCloner,
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
		templateCloner:  templateCloner,
		config:          config,
		objectDeriver:   objectDeriver,
	}
}

type objectDeriver interface {
	DeriveTreeObjectWithUniqueKey(ctx context.Context, spaceID string, key domain.UniqueKey, initFunc smartblock.InitFunc) (sb smartblock.SmartBlock, err error)
}

func (w *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	err = w.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	w.initTemplate(ctx)

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

type templateCloner interface {
	TemplateClone(spaceID string, id string) (templateID string, err error)
}

type WorkspaceParameters struct {
	IsHighlighted bool
	WorkspaceId   string
}

func (wp *WorkspaceParameters) Equal(other *WorkspaceParameters) bool {
	return wp.IsHighlighted == other.IsHighlighted
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
	return migration.MakeMigrations([]migration.Migration{
		{
			Version: 1,
			Proc:    w.migrateSubObjects,
		},
	})
}

func (w *Workspaces) migrateSubObjects(_ *state.State) {
	w.iterateAllSubObjects(
		func(info smartblock.DocInfo) bool {
			uniqueKeyRaw := pbtypes.GetString(info.Details, bundle.RelationKeyUniqueKey.String())
			id, err := w.migrateSubObject(context.Background(), uniqueKeyRaw, info.Details, info.Type)
			if err != nil {
				log.Errorf("failed to index subobject %s: %s", info.Id, err)
				log.With("objectID", id).Errorf("failed to migrate subobject: %v", err)
				return true
			}
			log.With("objectId", id, "uniqueKey", uniqueKeyRaw).Warnf("migrated sub-object")
			return true
		},
	)
}

func (w *Workspaces) migrateSubObject(
	ctx context.Context,
	uniqueKeyRaw string,
	details *types.Struct,
	typeKey domain.TypeKey,
) (id string, err error) {
	uniqueKey, err := domain.UnmarshalUniqueKey(uniqueKeyRaw)
	if err != nil {
		return "", fmt.Errorf("unmarshal unique key: %w", err)
	}
	sb, err := w.objectDeriver.DeriveTreeObjectWithUniqueKey(ctx, w.SpaceID(), uniqueKey, func(id string) *smartblock.InitContext {
		st := state.NewDocWithUniqueKey(id, nil, uniqueKey).NewState()
		st.SetDetails(details)
		st.SetObjectTypeKey(typeKey)
		return &smartblock.InitContext{
			IsNewObject: true,
			State:       st,
			SpaceID:     w.SpaceID(),
		}
	})
	if err != nil {
		return "", err
	}

	return sb.Id(), nil
}

const (
	collectionKeyRelationOptions = "opt"
	collectionKeyRelations       = "rel"
	collectionKeyObjectTypes     = "ot"
)

var objectTypeToCollection = map[domain.TypeKey]string{
	bundle.TypeKeyObjectType:     collectionKeyObjectTypes,
	bundle.TypeKeyRelation:       collectionKeyRelations,
	bundle.TypeKeyRelationOption: collectionKeyRelationOptions,
}

func collectionKeyToTypeKey(collKey string) (domain.TypeKey, bool) {
	for ot, v := range objectTypeToCollection {
		if v == collKey {
			return ot, true
		}
	}
	return "", false
}

func (w *Workspaces) iterateAllSubObjects(proc func(smartblock.DocInfo) bool) {
	st := w.NewState()
	for _, coll := range objectTypeToCollection {
		data := st.GetSubObjectCollection(coll)
		if data == nil {
			continue
		}
		tk, ok := collectionKeyToTypeKey(coll)
		if !ok {
			log.With("collection", coll).Errorf("subobject migration: collection is invalid")
			continue
		}

		for subId, d := range data.GetFields() {
			if st, ok := d.Kind.(*types.Value_StructValue); !ok {
				log.Errorf("got invalid value for %s.%s:%t", coll, subId, d.Kind)
				continue
			} else {
				uk, err := w.getUniqueKey(coll, subId)
				if err != nil {
					log.With("collection", coll).Errorf("subobject migration: failed to get uniqueKey: %s", err.Error())
					continue
				}

				d := st.StructValue
				d.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uk.Marshal())

				if !proc(smartblock.DocInfo{
					SpaceID:    w.SpaceID(),
					Links:      nil,
					FileHashes: nil,
					Heads:      nil,
					Type:       tk,
					Details:    d,
				}) {
					return
				}
			}
		}
	}
	return
}

func (w *Workspaces) getUniqueKey(collection, key string) (domain.UniqueKey, error) {
	typeKey, ok := collectionKeyToTypeKey(collection)
	if !ok {
		return nil, fmt.Errorf("unknown collection %s", collection)
	}

	var sbt smartblock2.SmartBlockType
	switch typeKey {
	case bundle.TypeKeyRelation:
		sbt = smartblock2.SmartBlockTypeRelation
	case bundle.TypeKeyObjectType:
		sbt = smartblock2.SmartBlockTypeObjectType
	case bundle.TypeKeyRelationOption:
		sbt = smartblock2.SmartBlockTypeRelationOption
	default:
		return nil, fmt.Errorf("unknown collection %s", collection)
	}

	return domain.NewUniqueKey(sbt, key)
}
