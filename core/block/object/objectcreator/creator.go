package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
	"github.com/anyproto/anytype-heart/util/uri"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

var log = logging.Logger("object-service")

type eventKey int

const eventCreate eventKey = 0

type Service interface {
	CreateSmartBlockFromTemplate(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSet(ctx context.Context, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error)
	app.Component
}

type Creator struct {
	blockService      BlockService
	blockPicker       block.Picker
	objectStore       objectstore.ObjectStore
	collectionService CollectionService
	relationService   relation.Service
	bookmark          bookmark.Service
	objectFactory     *editor.ObjectFactory
	app               *app.App
	sbtProvider       typeprovider.SmartBlockTypeProvider
	creator           Service //nolint:unused

	// TODO: remove it?
	coreService core.Service
}

type CollectionService interface {
	CreateCollection(details *types.Struct, flags []*model.InternalFlag) (coresb.SmartBlockType, *types.Struct, *state.State, error)
}

func NewCreator() *Creator {
	return &Creator{}
}

func (c *Creator) Init(a *app.App) (err error) {
	c.blockService = a.MustComponent(block.CName).(BlockService)
	c.blockPicker = a.MustComponent(block.CName).(block.Picker)
	c.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.objectFactory = app.MustComponent[*editor.ObjectFactory](a)
	c.collectionService = app.MustComponent[CollectionService](a)
	c.relationService = app.MustComponent[relation.Service](a)
	c.coreService = app.MustComponent[core.Service](a)
	c.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	c.app = a
	return nil
}

const CName = "objectCreator"

func (c *Creator) Name() (name string) {
	return CName
}

// TODO Temporarily
type BlockService interface {
	StateFromTemplate(templateID string, name string) (st *state.State, err error)
	CreateTreeObject(ctx context.Context, spaceID string, tp coresb.SmartBlockType, initFunc block.InitFunc) (sb smartblock.SmartBlock, err error)
	CreateTreeObjectWithUniqueKey(ctx context.Context, spaceID string, key uniquekey.UniqueKey, initFunc block.InitFunc) (sb smartblock.SmartBlock, err error)
	TemplateClone(spaceID string, id string) (templateID string, err error)
}

func (c *Creator) CreateSmartBlockFromTemplate(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateID != "" {
		if createState, err = c.blockService.StateFromTemplate(templateID, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return c.CreateSmartBlockFromState(ctx, spaceID, sbType, details, createState)
}

// CreateSmartBlockFromState create new object from the provided `createState` and `details`. If you pass `details` into the function, it will automatically add missing relationLinks and override the details from the `createState`
// It will return error if some of the relation keys in `details` not installed in the workspace.
func (c *Creator) CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error) {
	if createState == nil {
		createState = state.NewDoc("", nil).(*state.State)
	}
	startTime := time.Now()
	// priority:
	// 1. details
	// 2. createState
	// 3. createState details
	// 4. default object type by smartblock type
	objectTypeIds := pbtypes.GetStringList(details, bundle.RelationKeyType.String())
	objectTypeKeys := make([]bundle.TypeKey, 0, len(objectTypeIds))
	if objectTypeIds == nil {
		objectTypeKeys = createState.ObjectTypeKeys()
		if objectTypeKeys == nil {
			objectTypeIds = pbtypes.GetStringList(createState.Details(), bundle.RelationKeyType.String())
		} else {
			for _, objectTypeKey := range objectTypeKeys {
				typeId, err := c.relationService.GetTypeIdByKey(ctx, spaceID, bundle.TypeKey(objectTypeKey))
				if err != nil {
					return "", nil, err
				}
				objectTypeIds = append(objectTypeIds, typeId)
			}
		}
	}
	if len(objectTypeIds) == 0 {
		if ot, exists := bundle.DefaultObjectTypePerSmartblockType[sbType]; exists {
			objectTypeKeys = []bundle.TypeKey{ot}
		} else {
			objectTypeKeys = []bundle.TypeKey{bundle.TypeKeyPage}
		}
	}

	if len(objectTypeIds) > 0 && len(objectTypeKeys) == 0 {
		ots, err := c.objectStore.GetObjectTypes(objectTypeIds)
		if err != nil {
			return "", nil, err
		}
		if len(ots) != len(objectTypeIds) {
			return "", nil, fmt.Errorf("can't find all object types")
		}
		for _, ot := range ots {
			objectTypeKeys = append(objectTypeKeys, bundle.TypeKey(ot.Key))
		}
	}

	var relationKeys []string
	var workspaceID string
	if details != nil && details.Fields != nil {
		for k, v := range details.Fields {
			// todo: check if relation exists locally
			relationKeys = append(relationKeys, k)
			createState.SetDetail(k, v)
		}

		detailsWorkspaceID := details.Fields[bundle.RelationKeyWorkspaceId.String()]
		if detailsWorkspaceID != nil && detailsWorkspaceID.GetStringValue() != "" {
			workspaceID = detailsWorkspaceID.GetStringValue()
		}
	}

	// if we don't have anything in details then check the object store
	if workspaceID == "" {
		workspaceID = c.coreService.PredefinedObjects(spaceID).Account
	}

	if workspaceID != "" {
		createState.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceID))
	}
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Int64(time.Now().Unix()))
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(c.coreService.ProfileID(spaceID)))

	// todo: find a proper way to inject the spaceID as soon as possible into the createState
	createState.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, pbtypes.String(spaceID))

	ev := &metrics.CreateObjectEvent{
		SetDetailsMs: time.Since(startTime).Milliseconds(),
	}

	ctx = context.WithValue(ctx, eventCreate, ev)
	initFunc := func(id string) *smartblock.InitContext {
		createState.SetRootId(id)
		return &smartblock.InitContext{
			Ctx:            ctx,
			ObjectTypeKeys: objectTypeKeys,
			State:          createState,
			RelationKeys:   relationKeys,
			SpaceID:        spaceID,
		}
	}

	var sb smartblock.SmartBlock

	if uKey := createState.UniqueKeyInternal(); uKey != "" {
		uk, err := uniquekey.New(sbType.ToProto(), uKey)
		if err != nil {
			return "", nil, err
		}
		sb, err = c.blockService.CreateTreeObjectWithUniqueKey(ctx, spaceID, uk, initFunc)
		if err != nil {
			return "", nil, err
		}
	} else {
		sb, err = c.blockService.CreateTreeObject(ctx, spaceID, sbType, initFunc)
		if err != nil {
			return
		}
	}

	id = sb.Id()
	ev.SmartblockCreateMs = time.Since(startTime).Milliseconds() - ev.SetDetailsMs - ev.WorkspaceCreateMs - ev.GetWorkspaceBlockWaitMs
	ev.SmartblockType = int(sbType)
	ev.ObjectId = id
	metrics.SharedClient.RecordEvent(*ev)
	return id, sb.CombinedDetails(), nil
}

func (c *Creator) InjectWorkspaceID(details *types.Struct, spaceID string, objectID string) {
	workspaceID, err := c.coreService.GetWorkspaceIdForObject(spaceID, objectID)
	if err != nil {
		workspaceID = ""
	}
	if workspaceID == "" || details == nil {
		return
	}
	if details.Fields == nil {
		details.Fields = make(map[string]*types.Value)
	}
	details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceID)
}

func (c *Creator) CreateSet(ctx context.Context, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error) {
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	// TODO remove it, when schema will be refactored
	source := req.Source
	var dvContent model.BlockContentOfDataview
	var dvSchema schema.Schema
	var blockContent *model.BlockContentOfDataview

	newState := state.NewDoc("", nil).NewState()

	if len(source) > 0 {
		// todo: decide the behaviour in case of empty source
		if dvContent, dvSchema, err = dataview.DataviewBlockBySource(req.SpaceId, c.sbtProvider, c.objectStore, source); err != nil {
			return
		}

		if dvSchema != nil {
			blockContent = &dvContent
		}

		if len(req.Source) > 0 {
			newState.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(req.Source))
		}
	}
	tmpls := []template.StateTransformer{
		template.WithRequiredRelations(),
	}

	if blockContent != nil {
		for i, view := range blockContent.Dataview.Views {
			if view.Relations == nil {
				blockContent.Dataview.Views[i].Relations = editor.GetDefaultViewRelations(blockContent.Dataview.Relations)
			}
		}
		tmpls = append(tmpls,
			template.WithDataview(*blockContent, false),
		)
	}

	template.InitTemplate(newState, tmpls...)

	// TODO: here can be a deadlock if this is somehow created from workspace (as set)
	return c.CreateSmartBlockFromState(ctx, req.SpaceId, coresb.SmartBlockTypePage, req.Details, newState)
}

// ObjectCreateBookmark creates a new Bookmark object for provided URL or returns id of existing one
func (c *Creator) ObjectCreateBookmark(ctx context.Context, req *pb.RpcObjectCreateBookmarkRequest) (objectID string, newDetails *types.Struct, err error) {
	source := pbtypes.GetString(req.Details, bundle.RelationKeySource.String())
	var res bookmark.ContentFuture
	if source != "" {
		u, err := uri.NormalizeURI(source)
		if err != nil {
			return "", nil, fmt.Errorf("process uri: %w", err)
		}
		res = c.bookmark.FetchBookmarkContent(req.SpaceId, u)
	} else {
		res = func() *model.BlockContentBookmark {
			return nil
		}
	}
	return c.bookmark.CreateBookmarkObject(ctx, req.SpaceId, req.Details, res)
}

func (w *Creator) createRelation(ctx context.Context, spaceID string, details *types.Struct) (id string, object *types.Struct, err error) {
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
		if bundle.HasRelation(key) {
			object.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(addr.BundledRelationURLPrefix + key)
		}
	}
	uk, err := uniquekey.New(coresb.SmartBlockTypeRelation.ToProto(), key)
	if err != nil {
		return "", nil, err
	}
	object.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uk.Marshal())
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	if pbtypes.GetInt64(details, bundle.RelationKeyRelationFormat.String()) == int64(model.RelationFormat_status) {
		object.Fields[bundle.RelationKeyRelationMaxCount.String()] = pbtypes.Int64(1)
	}
	// objectTypes := pbtypes.GetStringList(object, bundle.RelationKeyRelationFormatObjectTypes.String())
	// todo: check the objectTypes
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relation))

	return w.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypeRelation, object, nil)
}

func (w *Creator) createRelationOption(ctx context.Context, spaceID string, details *types.Struct) (id string, object *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create option: no data")
	}

	if pbtypes.GetString(details, "relationOptionText") != "" {
		return "", nil, fmt.Errorf("use name instead of relationOptionText")
	} else if pbtypes.GetString(details, "name") == "" {
		return "", nil, fmt.Errorf("name is empty")
	} else if pbtypes.GetString(details, bundle.RelationKeyRelationKey.String()) == "" {
		return "", nil, fmt.Errorf("invalid relation key: unknown enum")
	}

	object = pbtypes.CopyStruct(details)
	key := pbtypes.GetString(details, bundle.RelationKeyId.String())
	if key == "" {
		key = bson.NewObjectId().Hex()
	}

	// options has a short id for now to avoid migration of values inside relations
	id = key
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relationOption))

	return w.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypeRelation, object, nil)
}

type internalKeyGetter interface {
	InternalKey() string
}

func (w *Creator) createObjectType(ctx context.Context, spaceID string, details *types.Struct) (id string, newDetails *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create object type: no data")
	}

	sbType := coresb.SmartBlockTypeObjectType
	uk, err := getUniqueKeyOrGenerate(sbType, details)
	if err != nil {
		return "", nil, err
	}
	details.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uk.Marshal())
	key := uk.(internalKeyGetter).InternalKey()
	var recommendedRelationKeys []string
	for _, relId := range pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()) {
		relKey, err2 := pbtypes.BundledRelationIdToKey(relId)
		if err2 != nil {
			log.Errorf("create object type: invalid recommended relation id: %s", relId)
			continue
		}
		// todo: support custom relations here
		rel, _ := bundle.GetRelation(bundle.RelationKey(relKey))
		if rel != nil {
			_, _, err2 := w.createRelation(ctx, spaceID, (&relationutils.Relation{rel}).ToStruct())
			// todo: check if the relation already exists
			if err2 != nil && err2 != fmt.Errorf("TODO: relation already exists") {
				err = fmt.Errorf("failed to create relation for objectType: %s", err2.Error())
				return
			}
		}
		recommendedRelationKeys = append(recommendedRelationKeys, relKey)
	}
	object := pbtypes.CopyStruct(details)
	rawLayout := pbtypes.GetInt64(details, bundle.RelationKeyRecommendedLayout.String())
	layout, err := bundle.GetLayout(model.ObjectTypeLayout(int32(rawLayout)))
	if err != nil {
		return "", nil, fmt.Errorf("invalid layout %d: %w", rawLayout, err)
	}

	for _, rel := range layout.RequiredRelations {
		if slice.FindPos(recommendedRelationKeys, rel.Key) != -1 {
			continue
		}
		recommendedRelationKeys = append(recommendedRelationKeys, rel.Key)
	}
	var recommendedRelationIds = make([]string, len(recommendedRelationKeys))
	for _, relKey := range recommendedRelationKeys {
		uk, err := uniquekey.New(model.SmartBlockType_STRelation, relKey)
		if err != nil {
			return "", nil, fmt.Errorf("failed to create unique key: %w", err)
		}
		id, err := w.coreService.DeriveObjectId(ctx, spaceID, uk)
		if err != nil {
			return "", nil, fmt.Errorf("failed to derive object id: %w", err)
		}
		recommendedRelationIds = append(recommendedRelationIds, id)
	}
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
	object.Fields[bundle.RelationKeyRecommendedLayout.String()] = pbtypes.Float64(float64(rawLayout))
	object.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(recommendedRelationIds)

	if details.GetFields() == nil {
		details.Fields = map[string]*types.Value{}
	}
	bundledTemplates, _, err := w.objectStore.Query(database.Query{
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

	alreadyInstalledTemplates, _, err := w.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(w.coreService.PredefinedObjects(spaceID).SystemTypes[bundle.TypeKeyTemplate]),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       object.Fields[bundle.RelationKeyType.String()],
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

			_, err := w.blockService.TemplateClone(spaceID, id)
			if err != nil {
				log.Errorf("failed to clone template %s: %s", id, err.Error())
			}
		}
	}()

	// we need to create it here directly, because we need to set the object type
	createState := state.NewDoc("", nil).(*state.State)
	createState.SetObjectTypeKey(bundle.TypeKeyObjectType)
	createState.SetDetails(object)
	return w.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypeObjectType, nil, createState)
}

func getUniqueKeyOrGenerate(sbType coresb.SmartBlockType, details *types.Struct) (uniquekey.UniqueKey, error) {
	uniqueKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
	if uniqueKey == "" {
		return uniquekey.New(sbType.ToProto(), bson.NewObjectId().Hex())
	}
	return uniquekey.UnmarshalFromString(uniqueKey)
}

func (c *Creator) CreateObject(ctx context.Context, spaceID string, req block.DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error) {
	details = req.GetDetails()
	if details.GetFields() == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}

	objectTypeId := pbtypes.GetString(details, bundle.RelationKeyType.String())
	var objectTypeKey bundle.TypeKey
	if forcedType != "" {
		objectTypeId, err = c.relationService.GetSystemTypeId(spaceID, forcedType)
		if err != nil {
			return "", nil, err
		}
		objectTypeKey = forcedType
	} else if objectTypeId == "" {
		// here we can't rely on DefaultObjectTypePerSmartblockType, because we don't know the smartblock type yet
		// it's actually opposite. We assume that client provides the object type, so we can determine the smartblock type
		// todo: client has the default objectType setting, so probably we should assume that it is up for a client and just return error if it's not provided
		objectTypeId, err = c.relationService.GetSystemTypeId(spaceID, bundle.TypeKeyPage)
		if err != nil {
			return "", nil, err
		}
		objectTypeKey = bundle.TypeKeyPage
	} else {
		ot, err := c.objectStore.GetObjectType(objectTypeId)
		if err != nil {
			return "", nil, err
		}

		objectTypeKey = bundle.TypeKey(ot.Key)
	}
	details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(objectTypeId)

	var internalFlags []*model.InternalFlag
	if v, ok := req.(block.InternalFlagsGetter); ok {
		internalFlags = v.GetInternalFlags()
		details = internalflag.PutToDetails(details, internalFlags)
	}

	var templateID string
	if v, ok := req.(block.TemplateIDGetter); ok {
		templateID = v.GetTemplateId()
	}

	details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(objectTypeId)
	var sbType = coresb.SmartBlockTypePage

	switch objectTypeKey {
	case bundle.TypeKeyBookmark:
		return c.ObjectCreateBookmark(ctx, &pb.RpcObjectCreateBookmarkRequest{
			Details: details,
			SpaceId: spaceID,
		})
	case bundle.TypeKeySet:
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_set))
		return c.CreateSet(ctx, &pb.RpcObjectCreateSetRequest{
			Details:       details,
			InternalFlags: internalFlags,
			Source:        pbtypes.GetStringList(details, bundle.RelationKeySetOf.String()),
			SpaceId:       spaceID,
		})
	case bundle.TypeKeyCollection:
		var st *state.State
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
		sbType, details, st, err = c.collectionService.CreateCollection(details, internalFlags)
		if err != nil {
			return "", nil, err
		}
		return c.CreateSmartBlockFromState(ctx, spaceID, sbType, details, st)
	case bundle.TypeKeyObjectType:
		return c.createObjectType(ctx, spaceID, details)
	case bundle.TypeKeyRelation:
		return c.createRelation(ctx, spaceID, details)
	case bundle.TypeKeyRelationOption:
		return c.createRelationOption(ctx, spaceID, details)
	case bundle.TypeKeyTemplate:
		sbType = coresb.SmartBlockTypeTemplate
	}

	return c.CreateSmartBlockFromTemplate(ctx, spaceID, sbType, details, templateID)
}
