package editor

import (
	"fmt"
	"strings"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	smartblock2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	database2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
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
	threadService   threads.Service
	threadQueue     threads.ThreadQueue
	templateCloner  templateCloner
	sourceService   source.Service
	anytype         core.Service
	objectStore     objectstore.ObjectStore
}

func NewWorkspace(
	objectStore objectstore.ObjectStore,
	anytype core.Service,
	relationService relation2.Service,
	sourceService source.Service,
	modifier DetailsModifier,
	fileBlockService file.BlockService,
) *Workspaces {
	return &Workspaces{
		SubObjectCollection: NewSubObjectCollection(
			collectionKeyRelationOptions,
			objectStore,
			anytype,
			relationService,
			sourceService,
			fileBlockService,
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
	p.templateCloner = p.app.MustComponent("blockService").(templateCloner)
	p.threadService = p.anytype.ThreadsService()
	p.threadQueue = p.anytype.ThreadsService().ThreadQueue()

	if ctx.Source.Type() != model.SmartBlockType_Workspace && ctx.Source.Type() != model.SmartBlockType_AccountOld {
		return fmt.Errorf("source type should be a workspace or an old account")
	}

	dataviewAllHighlightedObjects := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    []string{addr.RelationKeyToIdPrefix + bundle.RelationKeyName.String()},
			Relations: []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName)},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "_view1_1",
					Type: model.BlockContentDataviewView_Gallery,
					Name: "Highlighted",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{
						{
							Key:       bundle.RelationKeyName.String(),
							IsVisible: true,
						},
						{
							Key:       bundle.RelationKeyCreator.String(),
							IsVisible: true,
						},
					},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyWorkspaceId.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyId.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyIsHighlighted.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Bool(true),
					}},
				},
			},
		},
	}

	dataviewAllWorkspaceObjects := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    []string{addr.RelationKeyToIdPrefix + bundle.RelationKeyName.String()},
			Relations: []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName), bundle.MustGetRelation(bundle.RelationKeyCreator)},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "_view2_1",
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{
						{
							Key:       bundle.RelationKeyName.String(),
							IsVisible: true,
						},
						{
							Key:       bundle.RelationKeyCreator.String(),
							IsVisible: true,
						},
					},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyWorkspaceId.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyId.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.String(p.Id()),
					}},
				},
			},
		},
	}

	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	p.AddHook(p.updateSubObject, smartblock.HookAfterApply)

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

	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyWorkspaceId.String(): pbtypes.String(p.Id())}}
	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithFeaturedRelations,
		template.WithCondition(p.anytype.PredefinedBlocks().IsAccount(p.Id()),
			template.WithDetail(bundle.RelationKeyIsHidden, pbtypes.Bool(true))),
		template.WithCondition(p.anytype.PredefinedBlocks().IsAccount(p.Id()),
			template.WithForcedDetail(bundle.RelationKeyName, pbtypes.String("Personal space"))),
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()})),
		template.WithDataviewID("highlighted", dataviewAllHighlightedObjects, false),
		template.WithDataviewID(template.DataviewBlockId, dataviewAllWorkspaceObjects, false),
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

func (p *Workspaces) CreateObject(id thread.ID, sbType smartblock2.SmartBlockType) (core.SmartBlock, error) {
	st := p.NewState()
	if !id.Defined() {
		var err error
		id, err = threads.ThreadCreateID(thread.AccessControlled, sbType)
		if err != nil {
			return nil, err
		}
	}
	threadInfo, err := p.threadQueue.CreateThreadSync(id, p.Id())
	if err != nil {
		return nil, err
	}
	st.SetInStore([]string{source.WorkspaceCollection, threadInfo.ID.String()}, p.pbThreadInfoValueFromStruct(threadInfo))

	return core.NewSmartBlock(threadInfo, p.anytype), p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) DeleteObject(objectId string) error {
	st := p.NewState()
	err := p.threadQueue.DeleteThreadSync(objectId, p.Id())
	if err != nil {
		return err
	}
	st.RemoveFromStore([]string{source.WorkspaceCollection, objectId})
	return p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) GetAllObjects() []string {
	st := p.NewState()
	workspaceCollection := st.GetCollection(source.WorkspaceCollection)
	if workspaceCollection == nil || workspaceCollection.Fields == nil {
		return nil
	}
	objects := make([]string, 0, len(workspaceCollection.Fields))
	for objId, workspaceId := range workspaceCollection.Fields {
		if v, ok := workspaceId.Kind.(*types.Value_StringValue); ok && v.StringValue == p.Id() {
			objects = append(objects, objId)
		}
	}
	return objects
}

func (p *Workspaces) AddCreatorInfoIfNeeded() error {
	st := p.NewState()
	deviceID := p.anytype.Device()

	creatorCollection := st.GetCollection(source.CreatorCollection)
	if creatorCollection != nil && creatorCollection.Fields != nil && creatorCollection.Fields[deviceID] != nil {
		return nil
	}
	info, err := p.threadService.GetCreatorInfo(p.Id())
	if err != nil {
		return err
	}
	st.SetInStore([]string{source.CreatorCollection, deviceID}, p.pbCreatorInfoValue(info))

	return p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) MigrateMany(infos []threads.ThreadInfo) (int, error) {
	st := p.NewState()
	migrated := 0
	for _, info := range infos {
		if st.ContainsInStore([]string{source.AccountMigration, info.ID}) {
			continue
		}
		st.SetInStore([]string{source.AccountMigration, info.ID}, pbtypes.Bool(true))
		st.SetInStore([]string{source.WorkspaceCollection, info.ID},
			p.pbThreadInfoValue(info.ID, info.Key, info.Addrs),
		)
		migrated++
	}

	err := p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
	if err != nil {
		return 0, err
	}

	return migrated, nil
}

func (p *Workspaces) AddObject(objectId string, key string, addrs []string) error {
	st := p.NewState()
	err := p.threadQueue.AddThreadSync(threads.ThreadInfo{
		ID:    objectId,
		Key:   key,
		Addrs: addrs,
	}, p.Id())
	if err != nil {
		return err
	}
	st.SetInStore([]string{source.WorkspaceCollection, objectId}, p.pbThreadInfoValue(objectId, key, addrs))

	return p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

func (p *Workspaces) GetObjectKeyAddrs(objectId string) (string, []string, error) {
	threadId, err := thread.Decode(objectId)
	if err != nil {
		return "", nil, fmt.Errorf("failed to decode object %s: %w", objectId, err)
	}

	// we could have gotten the data from state, but to be sure 100% let's take it from service :-)
	threadInfo, err := p.threadService.GetThreadInfo(threadId)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get info on the thread %s: %w", objectId, err)
	}
	var publicAddrs []ma.Multiaddr
	for _, adr := range threadInfo.Addrs {
		// ignore cafe addr if it is there because we will add this anyway
		if manet.IsPublicAddr(adr) && adr.String() != p.threadService.CafePeer().String() {
			publicAddrs = append(publicAddrs, adr)
		}
	}
	if len(publicAddrs) > 2 {
		publicAddrs = publicAddrs[len(publicAddrs)-2:]
	}
	publicAddrs = append(publicAddrs, p.threadService.CafePeer())

	return threadInfo.Key.String(), util.MultiAddressesToStrings(publicAddrs), nil
}

func (p *Workspaces) SetIsHighlighted(objectId string, value bool) error {
	// TODO: this should be removed probably in the future?
	if p.anytype.PredefinedBlocks().IsAccount(p.Id()) {
		return fmt.Errorf("highlighting not supported for the account space")
	}

	st := p.NewState()
	st.SetInStore([]string{source.HighlightedCollection, objectId}, pbtypes.Bool(value))
	return p.Apply(st, smartblock.NoEvent, smartblock.NoHistory)
}

// TODO: try to save results from processing of previous state and get changes from apply for performance
func (p *Workspaces) updateObjects(info smartblock.ApplyInfo) error {
	objects, parameters := p.workspaceObjectsAndParametersFromState(info.State)
	startTime := time.Now()
	p.threadQueue.ProcessThreadsAsync(objects, p.Id())
	metrics.SharedClient.RecordEvent(metrics.ProcessThreadsEvent{WaitTimeMs: time.Now().Sub(startTime).Milliseconds()})
	if !p.anytype.PredefinedBlocks().IsAccount(p.Id()) {
		storedParameters := p.workspaceParametersFromRecords(p.storedRecordsForWorkspace())
		// we ignore the workspace object itself
		delete(storedParameters, p.Id())
		p.updateDetailsIfParametersChanged(storedParameters, parameters)
	}
	return nil
}

func (p *Workspaces) storedRecordsForWorkspace() []database2.Record {
	records, _, err := p.objectStore.Query(nil, database2.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyWorkspaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(p.Id()),
			},
		},
	})
	if err != nil {
		log.Errorf("workspace: can't get store workspace ids: %v", err)
		return nil
	}
	return records
}

func (p *Workspaces) workspaceParametersFromRecords(records []database2.Record) map[string]*WorkspaceParameters {
	var storeObjectInWorkspace = make(map[string]*WorkspaceParameters, len(records))
	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		storeObjectInWorkspace[id] = &WorkspaceParameters{
			IsHighlighted: pbtypes.GetBool(rec.Details, bundle.RelationKeyIsHighlighted.String()),
			WorkspaceId:   pbtypes.GetString(rec.Details, bundle.RelationKeyWorkspaceId.String()),
		}
	}
	return storeObjectInWorkspace
}

func (p *Workspaces) workspaceObjectsAndParametersFromState(st *state.State) ([]threads.ThreadInfo, map[string]*WorkspaceParameters) {
	workspaceCollection := st.GetCollection(source.WorkspaceCollection)
	if workspaceCollection == nil || workspaceCollection.Fields == nil {
		return nil, nil
	}
	parameters := make(map[string]*WorkspaceParameters, len(workspaceCollection.Fields))
	objects := make([]threads.ThreadInfo, 0, len(workspaceCollection.Fields))
	for objId, value := range workspaceCollection.Fields {
		if value == nil {
			continue
		}
		parameters[objId] = &WorkspaceParameters{
			IsHighlighted: false,
			WorkspaceId:   p.Id(),
		}
		objects = append(objects, p.threadInfoFromWorkspacePB(value))
	}

	creatorCollection := st.GetCollection(source.CreatorCollection)
	if creatorCollection != nil {
		for _, value := range creatorCollection.Fields {
			info, err := p.threadInfoFromCreatorPB(value)
			if err != nil {
				continue
			}
			objects = append(objects, info)
		}
	}
	highlightedCollection := st.GetCollection(source.HighlightedCollection)
	if highlightedCollection != nil {
		for objId, isHighlighted := range highlightedCollection.Fields {
			if pbtypes.IsExpectedBoolValue(isHighlighted, true) {
				if _, exists := parameters[objId]; exists {
					parameters[objId].IsHighlighted = true
				}
			}
		}
	}

	return objects, parameters
}

func (p *Workspaces) updateDetailsIfParametersChanged(
	oldParameters map[string]*WorkspaceParameters,
	newParameters map[string]*WorkspaceParameters) {
	for id, params := range newParameters {
		if oldParams, exists := oldParameters[id]; exists && oldParams.Equal(params) {
			continue
		}

		// TODO: we need to move it to another service, but now it is what it is
		go func(id string, params WorkspaceParameters) {
			if err := p.DetailsModifier.ModifyLocalDetails(id, func(current *types.Struct) (*types.Struct, error) {
				if current == nil || current.Fields == nil {
					current = &types.Struct{
						Fields: map[string]*types.Value{},
					}
				}
				current.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(params.WorkspaceId)
				current.Fields[bundle.RelationKeyIsHighlighted.String()] = pbtypes.Bool(params.IsHighlighted)

				return current, nil
			}); err != nil {
				log.Errorf("workspace: can't set detail to object: %v", err)
			}
		}(id, *params)
	}
}

func (p *Workspaces) pbCreatorInfoValue(info threads.CreatorInfo) *types.Value {
	return &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: &types.Struct{
				Fields: map[string]*types.Value{
					collectionKeyAccount:   pbtypes.String(info.AccountPubKey),
					collectionKeySignature: pbtypes.String(string(info.WorkspaceSig)),
					collectionKeyAddrs:     pbtypes.StringList(info.Addrs),
				},
			},
		},
	}
}

func (p *Workspaces) pbThreadInfoValue(id string, key string, addrs []string) *types.Value {
	return &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: &types.Struct{
				Fields: map[string]*types.Value{
					collectionKeyId:    pbtypes.String(id),
					collectionKeyKey:   pbtypes.String(key),
					collectionKeyAddrs: pbtypes.StringList(addrs),
				},
			},
		},
	}
}

func (p *Workspaces) pbThreadInfoValueFromStruct(ti thread.Info) *types.Value {
	return p.pbThreadInfoValue(ti.ID.String(), ti.Key.String(), util.MultiAddressesToStrings(ti.Addrs))
}

func (p *Workspaces) threadInfoFromWorkspacePB(val *types.Value) threads.ThreadInfo {
	fields := val.Kind.(*types.Value_StructValue).StructValue
	return threads.ThreadInfo{
		ID:    pbtypes.GetString(fields, collectionKeyId),
		Key:   pbtypes.GetString(fields, collectionKeyKey),
		Addrs: pbtypes.GetStringListValue(fields.Fields[collectionKeyAddrs]),
	}
}

func (p *Workspaces) threadInfoFromCreatorPB(val *types.Value) (threads.ThreadInfo, error) {
	fields := val.Kind.(*types.Value_StructValue).StructValue
	account := pbtypes.GetString(fields, collectionKeyAccount)
	profileId, err := threads.ProfileThreadIDFromAccountAddress(account)
	if err != nil {
		return threads.ThreadInfo{}, err
	}
	sk, pk, err := threads.ProfileThreadKeysFromAccountAddress(account)
	if err != nil {
		return threads.ThreadInfo{}, err
	}
	return threads.ThreadInfo{
		ID:    profileId.String(),
		Key:   thread.NewKey(sk, pk).String(),
		Addrs: pbtypes.GetStringListValue(fields.Fields[collectionKeyAddrs]),
	}, nil
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

	bundledTemplates, _, err := w.objectStore.Query(nil, database2.Query{
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

	alreadyInstalledTemplates, _, err := w.objectStore.Query(nil, database2.Query{
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

func (w *Workspaces) RemoveSubObjects(objectIds []string) (err error) {
	st := w.NewState()
	for _, id := range objectIds {
		err = w.removeObject(st, id)
		if err != nil {
			log.Errorf("failed to remove sub object: %s", err.Error())
			continue
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
