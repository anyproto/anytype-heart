package editor

import (
	"fmt"
	"strings"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/treemanager"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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

	app             *app.App
	DetailsModifier DetailsModifier
	templateCloner  templateCloner
	sourceService   source.Service
	anytype         core.Service
	objectStore     objectstore.ObjectStore
}

func NewWorkspace(
	sb smartblock.SmartBlock,
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	relationService relation.Service,
	sourceService source.Service,
	modifier DetailsModifier,
	fileBlockService file.BlockService,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
) *Workspaces {
	return &Workspaces{
		SubObjectCollection: NewSubObjectCollection(
			sb,
			collectionKeyRelationOptions,
			objectStore,
			anytype,
			relationService,
			sourceService,
			fileBlockService,
			tempDirProvider,
			sbtProvider,
			layoutConverter,
		),
		DetailsModifier: modifier,
		anytype:         anytype,
		objectStore:     objectStore,
	}
}

// nolint:funlen
func (p *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	err = p.SubObjectCollection.Init(ctx)
	if err != nil {
		return err
	}

	p.app = ctx.App
	// TODO pass as explicit deps
	p.sourceService = p.app.MustComponent(source.CName).(source.Service)
	p.templateCloner = p.app.MustComponent(treemanager.CName).(templateCloner)
	cfg := p.app.MustComponent(config.CName).(*config.Config)
	if cfg.AnalyticsId != "" {
		ctx.State.SetSetting(state.SettingsAnalyticsId, pbtypes.String(cfg.AnalyticsId))
	} else if ctx.State.GetSetting(state.SettingsAnalyticsId) == nil {
		// add analytics id for existing users so it will be active from the next start
		log.Warnf("analyticsID is missing, generating new one")
		ctx.State.SetSetting(state.SettingsAnalyticsId, pbtypes.String(metrics.GenerateAnalyticsId()))
	}
	p.AddHook(p.updateSubObject, smartblock.HookAfterApply)

	// init template before sub-object initialization because sub-objects could fire onSubObjectChange callback
	// and index incomplete workspace template
	err = p.initTemplate(ctx)
	if err != nil {
		return fmt.Errorf("init template: %w", err)
	}

	data := ctx.State.Store()
	if data != nil && data.Fields != nil {
		for collName, coll := range data.Fields {
			if !collectionKeyIsSupported(collName) {
				continue
			}
			if coll != nil && coll.GetStructValue() != nil {
				for sub := range coll.GetStructValue().GetFields() {
					if err = p.initSubObject(ctx.State, collName, sub, false); err != nil {
						log.Errorf("failed to init sub object %s-%s: %v", collName, sub, err)
					}
				}
			}
		}
	}

	for path := range ctx.State.StoreKeysRemoved() {
		pathS := strings.Split(path, "/")
		if !collectionKeyIsSupported(pathS[0]) {
			continue
		}
		if err = p.initSubObject(ctx.State, pathS[0], strings.Join(pathS[1:], addr.SubObjectCollectionIdSeparator), true); err != nil {
			log.Errorf("failed to init deleted sub object %s: %v", path, err)
		}
	}

	return nil
}

func (p *Workspaces) initTemplate(ctx *smartblock.InitContext) error {
	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyWorkspaceId.String(): pbtypes.String(p.Id())}}
	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithFeaturedRelations,
		template.WithForcedDetail(bundle.RelationKeyWorkspaceId, pbtypes.String(p.Id())),
		template.WithDetail(bundle.RelationKeyIsHidden, pbtypes.Bool(true)),
		template.WithDetail(bundle.RelationKeySpaceAccessibility, pbtypes.Int64(0)),
		template.WithForcedDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_space))),
		template.WithForcedObjectTypes([]string{bundle.TypeKeySpace.URL()}),
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()})),
		template.WithForcedDetail(bundle.RelationKeyCreator, pbtypes.String(p.anytype.PredefinedBlocks().Profile)),
		template.WithBlockField(template.DataviewBlockId, dataview.DefaultDetailsFieldName, pbtypes.Struct(defaultValue)),
	)
}

type templateCloner interface {
	TemplateClone(id string) (templateID string, err error)
}

type WorkspaceParameters struct {
	IsHighlighted bool
	WorkspaceId   string
}

func (wp *WorkspaceParameters) Equal(other *WorkspaceParameters) bool {
	return wp.IsHighlighted == other.IsHighlighted
}

func (w *Workspaces) createRelation(st *state.State, details *types.Struct) (id string, object *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create relation: no data")
	}

	if v, ok := details.GetFields()[bundle.RelationKeyRelationFormat.String()]; !ok {
		return "", nil, fmt.Errorf("missing relation format")
	} else if i, ok := v.Kind.(*types.Value_NumberValue); !ok {
		return "", nil, fmt.Errorf("invalid relation format: not a number")
	} else if model.RelationFormat(int(i.NumberValue)).String() == "" {
		return "", nil, fmt.Errorf("invalid relation format: unknown enum")
	}

	if pbtypes.GetString(details, bundle.RelationKeyName.String()) == "" {
		return "", nil, fmt.Errorf("missing relation name")
	}

	object = pbtypes.CopyStruct(details)
	key := pbtypes.GetString(details, bundle.RelationKeyRelationKey.String())
	if key == "" {
		key = bson.NewObjectId().Hex()
	} else {
		// no need to check for the generated bson's
		if st.HasInStore([]string{collectionKeyRelations, key}) {
			return id, object, ErrSubObjectAlreadyExists
		}
		if bundle.HasRelation(key) {
			object.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(addr.BundledRelationURLPrefix + key)
		}
	}
	id = addr.RelationKeyToIdPrefix + key
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	if pbtypes.GetInt64(details, bundle.RelationKeyRelationFormat.String()) == int64(model.RelationFormat_status) {
		object.Fields[bundle.RelationKeyRelationMaxCount.String()] = pbtypes.Int64(1)
	}
	objectTypes := pbtypes.GetStringList(object, bundle.RelationKeyRelationFormatObjectTypes.String())
	if len(objectTypes) > 0 {
		var objectTypesToMigrate []string
		objectTypes, objectTypesToMigrate = relationutils.MigrateObjectTypeIds(objectTypes)
		if len(objectTypesToMigrate) > 0 {
			st.SetObjectTypesToMigrate(append(st.ObjectTypesToMigrate(), objectTypesToMigrate...))
		}
	}
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relation))
	object.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyRelation.URL())
	st.SetInStore([]string{collectionKeyRelations, key}, pbtypes.Struct(cleanSubObjectDetails(object)))
	// nolint:errcheck
	_ = w.objectStore.DeleteDetails(id) // we may have details exist from the previously removed relation. Do it before the init so we will not have existing local details populated
	if err = w.initSubObject(st, collectionKeyRelations, key, true); err != nil {
		return
	}
	return
}

func (w *Workspaces) createRelationOption(st *state.State, details *types.Struct) (id string, object *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create option: no data")
	}

	if pbtypes.GetString(details, "relationOptionText") != "" {
		return "", nil, fmt.Errorf("use name instead of relationOptionText")
	} else if pbtypes.GetString(details, "name") == "" {
		return "", nil, fmt.Errorf("name is empty")
	} else if pbtypes.GetString(details, bundle.RelationKeyType.String()) != bundle.TypeKeyRelationOption.URL() {
		return "", nil, fmt.Errorf("invalid type: not an option")
	} else if pbtypes.GetString(details, bundle.RelationKeyRelationKey.String()) == "" {
		return "", nil, fmt.Errorf("invalid relation key: unknown enum")
	}

	object = pbtypes.CopyStruct(details)
	key := pbtypes.GetString(details, bundle.RelationKeyId.String())
	if key == "" {
		key = bson.NewObjectId().Hex()
	} else {
		// no need to check for the generated bson's
		if st.HasInStore([]string{collectionKeyRelationOptions, key}) {
			return key, object, ErrSubObjectAlreadyExists
		}
	}
	// options has a short id for now to avoid migration of values inside relations
	id = key
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relationOption))
	object.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyRelationOption.URL())

	st.SetInStore([]string{collectionKeyRelationOptions, key}, pbtypes.Struct(cleanSubObjectDetails(object)))
	// nolint:errcheck
	_ = w.objectStore.DeleteDetails(id) // we may have details exist from the previously removed relation option. Do it before the init so we will not have existing local details populated
	if err = w.initSubObject(st, collectionKeyRelationOptions, key, true); err != nil {
		return
	}
	return
}

func (w *Workspaces) createObjectType(st *state.State, details *types.Struct) (id string, object *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create object type: no data")
	}

	var recommendedRelationIds []string
	for _, relId := range pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()) {
		relKey, err2 := pbtypes.RelationIdToKey(relId)
		if err2 != nil {
			log.Errorf("create object type: invalid recommended relation id: %s", relId)
			continue
		}
		rel, _ := bundle.GetRelation(bundle.RelationKey(relKey))
		if rel != nil {
			_, _, err2 := w.createRelation(st, (&relationutils.Relation{rel}).ToStruct())
			if err2 != nil && err2 != ErrSubObjectAlreadyExists {
				err = fmt.Errorf("failed to create relation for objectType: %s", err2.Error())
				return
			}
		}
		recommendedRelationIds = append(recommendedRelationIds, addr.RelationKeyToIdPrefix+relKey)
	}
	object = pbtypes.CopyStruct(details)
	key := pbtypes.GetString(details, bundle.RelationKeyId.String())
	if key == "" {
		key = bson.NewObjectId().Hex()
	} else {
		key = strings.TrimPrefix(key, addr.BundledObjectTypeURLPrefix)
		if bundle.HasObjectType(key) {
			object.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(addr.BundledObjectTypeURLPrefix + key)
		}
		key = strings.TrimPrefix(key, addr.ObjectTypeKeyToIdPrefix)
	}

	rawLayout := pbtypes.GetInt64(details, bundle.RelationKeyRecommendedLayout.String())
	layout, err := bundle.GetLayout(model.ObjectTypeLayout(int32(rawLayout)))
	if err != nil {
		return "", nil, fmt.Errorf("invalid layout %d: %w", rawLayout, err)
	}

	for _, rel := range layout.RequiredRelations {
		relId := addr.RelationKeyToIdPrefix + rel.Key
		if slice.FindPos(recommendedRelationIds, relId) != -1 {
			continue
		}
		recommendedRelationIds = append(recommendedRelationIds, relId)
	}
	id = addr.ObjectTypeKeyToIdPrefix + key
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyObjectType.URL())
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
	object.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(recommendedRelationIds)
	sbType := pbtypes.GetIntList(details, bundle.RelationKeySmartblockTypes.String())
	if len(sbType) == 0 {
		sbType = []int{int(model.SmartBlockType_Page)}
	}
	object.Fields[bundle.RelationKeySmartblockTypes.String()] = pbtypes.IntList(sbType...)

	// no need to check for the generated bson's
	if st.HasInStore([]string{collectionKeyObjectTypes, key}) {
		// todo: optimize this
		return id, object, ErrSubObjectAlreadyExists
	}

	st.SetInStore([]string{collectionKeyObjectTypes, key}, pbtypes.Struct(cleanSubObjectDetails(object)))
	// nolint:errcheck
	_ = w.objectStore.DeleteDetails(id) // we may have details exist from the previously removed object type. Do it before the init so we will not have existing local details populated
	if err = w.initSubObject(st, collectionKeyObjectTypes, key, true); err != nil {
		return
	}

	bundledTemplates, _, err := w.objectStore.Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(bundle.TypeKeyTemplate.BundledURL()),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(addr.BundledObjectTypeURLPrefix + key),
			},
		},
	})

	alreadyInstalledTemplates, _, err := w.objectStore.Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(bundle.TypeKeyTemplate.URL()),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(addr.ObjectTypeKeyToIdPrefix + key),
			},
		},
	})
	if err != nil {
		return
	}

	var existingTemplatesMap = map[string]struct{}{}
	for _, rec := range alreadyInstalledTemplates {
		sourceObject := pbtypes.GetString(rec.Details, bundle.RelationKeySourceObject.String())
		if sourceObject != "" {
			existingTemplatesMap[sourceObject] = struct{}{}
		}
	}

	go func() {
		// todo: remove this dirty hack to avoid lock
		for _, record := range bundledTemplates {
			id := pbtypes.GetString(record.Details, bundle.RelationKeyId.String())
			if _, exists := existingTemplatesMap[id]; exists {
				continue
			}

			_, err := w.templateCloner.TemplateClone(id)
			if err != nil {
				log.Errorf("failed to clone template %s: %s", id, err.Error())
			}
		}
	}()
	return
}

func (w *Workspaces) createObject(st *state.State, details *types.Struct) (id string, object *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create object type: no data")
	}

	if pbtypes.GetString(details, bundle.RelationKeyType.String()) == "" {
		return "", nil, fmt.Errorf("type is empty")
	}

	details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(w.Id())
	if pbtypes.GetFloat64(details, bundle.RelationKeyCreatedDate.String()) == 0 {
		details.Fields[bundle.RelationKeyCreatedDate.String()] = pbtypes.Float64(float64(time.Now().Unix()))
	}
	switch pbtypes.GetString(details, bundle.RelationKeyType.String()) {
	case bundle.TypeKeyObjectType.URL():
		return w.createObjectType(st, details)
	case bundle.TypeKeyRelation.URL():
		return w.createRelation(st, details)
	case bundle.TypeKeyRelationOption.URL():
		return w.createRelationOption(st, details)
	default:
		return "", nil, fmt.Errorf("invalid type: %s", pbtypes.GetString(details, bundle.RelationKeyType.String()))
	}
}

func (w *Workspaces) CreateSubObject(details *types.Struct) (id string, object *types.Struct, err error) {
	st := w.NewState()
	id, object, err = w.createObject(st, details)
	if err != nil {
		return "", nil, err
	}
	if err = w.Apply(st, smartblock.NoHooks); err != nil {
		return
	}
	return
}

func (w *Workspaces) CreateSubObjects(details []*types.Struct) (ids []string, objects []*types.Struct, err error) {
	st := w.NewState()
	var (
		id     string
		object *types.Struct
	)
	for _, det := range details {
		id, object, err = w.createObject(st, det)
		if err != nil {
			if err != ErrSubObjectAlreadyExists {
				log.Errorf("failed to create sub object: %s", err.Error())
			}
			continue
		}
		ids = append(ids, id)
		objects = append(objects, object)
	}

	if len(ids) == 0 {
		return
	}
	// reset error in case we have at least 1 object created
	err = nil
	if err = w.Apply(st, smartblock.NoHooks); err != nil {
		return
	}
	return
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

	var skipIDs = map[string]struct{}{}
	for _, rel := range bundle.SystemRelations {
		skipIDs[addr.RelationKeyToIdPrefix+rel.String()] = struct{}{}
	}

	relIds := pbtypes.GetStringList(obj.Details, bundle.RelationKeyRecommendedRelations.String())

	// find relations that are custom(was not installed from somewhere)
	records, _, err := w.objectStore.Query(nil, database.Query{
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
	records, _, err = w.objectStore.Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(bundle.TypeKeyObjectType.URL()),
			},
			{
				RelationKey: bundle.RelationKeyRecommendedRelations.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(relIds),
			},
			{
				RelationKey: bundle.RelationKeyWorkspaceId.String(),
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
		relKey, err := pbtypes.RelationIdToKey(relId)
		if err != nil {
			log.Errorf("failed to get relation key from id %s: %s", relId, err.Error())
			continue
		}
		records, _, err := w.objectStore.Query(nil, database.Query{
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

// RemoveSubObjects removes sub objects from the workspace collection
// if orphansGC is true, then relations that are not used by any object in the workspace will be removed as well
func (w *Workspaces) RemoveSubObjects(objectIds []string, orphansGC bool) (err error) {
	st := w.NewState()
	for _, id := range objectIds {
		// special case for object types
		var idsToRemove []string
		if strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) && orphansGC {
			idsToRemove, err = w.objectTypeRelationsForGC(id)
			if err != nil {
				log.Errorf("objectTypeRelationsForGC failed: %s", err.Error())
				continue
			}
			if len(idsToRemove) > 0 {
				log.Debugf("objectTypeRelationsForGC, relations to remove: %v", idsToRemove)
			}
		}

		err = w.removeObject(st, id)
		if err != nil {
			log.Errorf("failed to remove sub object: %s", err.Error())
			continue
		}
		if orphansGC && len(idsToRemove) > 0 {
			for _, relId := range idsToRemove {
				err = w.removeObject(st, relId)
				if err != nil {
					log.Errorf("failed to remove dependent sub object: %s", err.Error())
					continue
				}
			}
		}
	}

	// reset error in case we have at least 1 object created
	err = nil
	if err = w.Apply(st, smartblock.NoHooks); err != nil {
		return
	}
	return
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
