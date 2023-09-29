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
)

const (
	collectionKeySignature = "signature"
	collectionKeyAccount   = "account"
	collectionKeyAddrs     = "addrs"
	collectionKeyId        = "id"
	collectionKeyKey       = "key"
)

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

type Workspaces struct {
	*SubObjectCollection

	DetailsModifier DetailsModifier
	templateCloner  templateCloner
	sourceService   source.Service
	anytype         core.Service
	objectStore     objectstore.ObjectStore
	config          *config.Config
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
) *Workspaces {
	return &Workspaces{
		SubObjectCollection: NewSubObjectCollection(
			sb,
			collectionKeyRelationOptions,
			objectStore,
			anytype,
			systemObjectService,
			sourceService,
			sbtProvider,
			layoutConverter,
			eventSender,
		),
		DetailsModifier: modifier,
		anytype:         anytype,
		objectStore:     objectStore,
		sourceService:   sourceService,
		templateCloner:  templateCloner,
		config:          config,
	}
}

// nolint:funlen
func (p *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	// init template before sub-object initialization because sub-objects could fire onSubObjectChange callback
	// and index incomplete workspace template

	err = p.SubObjectCollection.Init(ctx)
	if err != nil {
		return err
	}
	p.initTemplate(ctx)

	data := ctx.State.Store()
	if data != nil && data.Fields != nil {
		// todo: replace with migration
		for collName, coll := range data.Fields {
			if !collectionKeyIsSupported(collName) {
				continue
			}
			if coll != nil && coll.GetStructValue() != nil {

			}
		}
	}
	return nil
}

func (p *Workspaces) initTemplate(ctx *smartblock.InitContext) {
	if p.config.AnalyticsId != "" {
		ctx.State.SetSetting(state.SettingsAnalyticsId, pbtypes.String(p.config.AnalyticsId))
	} else if ctx.State.GetSetting(state.SettingsAnalyticsId) == nil {
		// add analytics id for existing users so it will be active from the next start
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
		template.WithForcedDetail(bundle.RelationKeyCreator, pbtypes.String(p.anytype.PredefinedObjects(p.SpaceID()).Profile)),
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

func collectionKeyIsSupported(collKey string) bool {
	for _, v := range objectTypeToCollection {
		if v == collKey {
			return true
		}
	}
	return false
}

func collectionKeyToObjectType(collKey string) (domain.TypeKey, bool) {
	for ot, v := range objectTypeToCollection {
		if v == collKey {
			return ot, true
		}
	}
	return "", false
}
