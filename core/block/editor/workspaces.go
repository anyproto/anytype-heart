package editor

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
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

var objectTypeToCollection = map[bundle.TypeKey]string{
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
	relationService relation.Service,
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
			relationService,
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
	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyWorkspaceId.String(): pbtypes.String(p.Id())}}
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
		template.WithForcedDetail(bundle.RelationKeyWorkspaceId, pbtypes.String(p.Id())),
		template.WithDetail(bundle.RelationKeyIsHidden, pbtypes.Bool(true)),
		template.WithDetail(bundle.RelationKeySpaceAccessibility, pbtypes.Int64(0)),
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_space))),
		template.WithForcedObjectTypes([]bundle.TypeKey{bundle.TypeKeySpace}),
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()})),
		template.WithForcedDetail(bundle.RelationKeyCreator, pbtypes.String(p.anytype.PredefinedObjects(p.SpaceID()).Profile)),
		template.WithBlockField(template.DataviewBlockId, dataview.DefaultDetailsFieldName, pbtypes.Struct(defaultValue)),
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

// objectTypeRelationsForGC returns the list of relation IDs that are safe to remove alongside with the provided object type
// - they were installed from the marketplace(not custom by the user)
// - they are not used as recommended in other installed/custom object types
// - they are not used directly in some object
func (w *Workspaces) objectTypeRelationsForGC(objectTypeID string) (ids []string, err error) {
	obj, err := w.objectStore.GetDetails(objectTypeID)
	if err != nil {
		return nil, err
	}

	source := pbtypes.GetString(obj.Details, bundle.RelationKeySourceObject.String())
	if source == "" {
		// type was not installed from marketplace
		return nil, nil
	}
	spaceId := w.SpaceID()
	predefinedIds := w.anytype.PredefinedObjects(spaceId)
	var skipIDs = map[string]struct{}{}
	for _, rel := range bundle.SystemRelations {
		skipIDs[predefinedIds.SystemRelations[rel]] = struct{}{}
	}

	relIds := pbtypes.GetStringList(obj.Details, bundle.RelationKeyRecommendedRelations.String())

	// find relations that are custom(was not installed from somewhere)
	records, _, err := w.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(relIds),
			},
			{
				RelationKey: bundle.RelationKeySourceObject.String(),
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	for _, rec := range records {
		skipIDs[pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())] = struct{}{}
	}

	// check if this relation is used in some other installed object types
	records, _, err = w.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(predefinedIds.SystemTypes[bundle.TypeKeyObjectType]),
			},
			{
				RelationKey: bundle.RelationKeyRecommendedRelations.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(relIds),
			},
			{
				RelationKey: bundle.RelationKeyWorkspaceId.String(), // todo: replace with spaceId
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(w.Id()),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	for _, rec := range records {
		recId := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		if recId == objectTypeID {
			continue
		}
		rels := pbtypes.GetStringList(rec.Details, bundle.RelationKeyRecommendedRelations.String())
		for _, rel := range rels {
			if slice.FindPos(relIds, rel) > -1 {
				skipIDs[rel] = struct{}{}
			}
		}
	}

	for _, relId := range relIds {
		if _, exists := skipIDs[relId]; exists {
			continue
		}
		relKey, err := pbtypes.BundledRelationIdToKey(relId)
		if err != nil {
			log.Errorf("failed to get relation key from id %s: %s", relId, err.Error())
			continue
		}
		records, _, err := w.objectStore.Query(database.Query{
			Limit: 1,
			Filters: []*model.BlockContentDataviewFilter{
				{
					// exclude installed templates that we don't remove yet and they may depend on the relation
					RelationKey: bundle.RelationKeyTargetObjectType.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.String(objectTypeID),
				},
				{
					RelationKey: bundle.RelationKeyWorkspaceId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(w.Id()),
				},
				{
					RelationKey: relKey,
					Condition:   model.BlockContentDataviewFilter_NotEmpty,
				},
			},
		})
		if len(records) > 0 {
			skipIDs[relId] = struct{}{}
		}
	}
	return slice.Filter(relIds, func(s string) bool {
		_, exists := skipIDs[s]
		return !exists
	}), nil
}

func collectionKeyIsSupported(collKey string) bool {
	for _, v := range objectTypeToCollection {
		if v == collKey {
			return true
		}
	}
	return false
}

func collectionKeyToObjectType(collKey string) (bundle.TypeKey, bool) {
	for ot, v := range objectTypeToCollection {
		if v == collKey {
			return ot, true
		}
	}
	return "", false
}
