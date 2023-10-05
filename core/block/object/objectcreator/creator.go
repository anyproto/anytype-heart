package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object"
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
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("object-service")

type eventKey int

const eventCreate eventKey = 0

type Service interface {
	CreateSmartBlockFromTemplate(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, objectTypeKeys []domain.TypeKey, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, objectTypeKeys []domain.TypeKey, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSet(ctx context.Context, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error)
	app.Component
}

type Creator struct {
	blockService        BlockService
	objectCache         objectcache.Cache
	blockPicker         block.ObjectGetter
	objectStore         objectstore.ObjectStore
	collectionService   CollectionService
	systemObjectService system_object.Service
	bookmark            bookmark.Service
	app                 *app.App
	sbtProvider         typeprovider.SmartBlockTypeProvider
	creator             Service //nolint:unused

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
	c.objectCache = a.MustComponent(objectcache.CName).(objectcache.Cache)
	c.blockPicker = a.MustComponent(block.CName).(block.ObjectGetter)
	c.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.collectionService = app.MustComponent[CollectionService](a)
	c.systemObjectService = app.MustComponent[system_object.Service](a)
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
	StateFromTemplate(templateID, name string) (st *state.State, err error)
	TemplateClone(spaceID string, id string) (templateID string, err error)
}

func (c *Creator) CreateSmartBlockFromTemplate(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, objectTypeKeys []domain.TypeKey, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateID != "" {
		if createState, err = c.blockService.StateFromTemplate(templateID, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return c.CreateSmartBlockFromState(ctx, spaceID, sbType, objectTypeKeys, details, createState)
}

// CreateSmartBlockFromState create new object from the provided `createState` and `details`. If you pass `details` into the function, it will automatically add missing relationLinks and override the details from the `createState`
// It will return error if some of the relation keys in `details` not installed in the workspace.
func (c *Creator) CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, objectTypeKeys []domain.TypeKey, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error) {
	if createState == nil {
		createState = state.NewDoc("", nil).(*state.State)
	}
	startTime := time.Now()
	// priority:
	// 1. details
	// 2. createState
	// 3. createState details
	// 4. default object type by smartblock type
	if len(objectTypeKeys) == 0 {
		if ot, exists := bundle.DefaultObjectTypePerSmartblockType[sbType]; exists {
			objectTypeKeys = []domain.TypeKey{ot}
		} else {
			objectTypeKeys = []domain.TypeKey{bundle.TypeKeyPage}
		}
	}

	var relationKeys []string
	if details != nil && details.Fields != nil {
		for k, v := range details.Fields {
			// todo: check if relation exists locally
			relationKeys = append(relationKeys, k)
			createState.SetDetail(k, v)
		}
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
		uk, err := domain.NewUniqueKey(sbType, uKey)
		if err != nil {
			return "", nil, err
		}
		sb, err = c.objectCache.DeriveTreeObject(ctx, spaceID, objectcache.TreeDerivationParams{
			Key:      uk,
			InitFunc: initFunc,
		})
		if err != nil {
			return "", nil, err
		}
	} else {
		sb, err = c.objectCache.CreateTreeObject(ctx, spaceID, objectcache.TreeCreationParams{
			Time:           time.Now(),
			SmartblockType: sbType,
			InitFunc:       initFunc,
		})
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

func (c *Creator) CreateSet(ctx context.Context, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error) {
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	// TODO remove it, when schema will be refactored
	source := req.Source
	var dvContent model.BlockContentOfDataview
	var dvSchema database.Schema
	var blockContent *model.BlockContentOfDataview

	newState := state.NewDoc("", nil).NewState()

	if len(source) > 0 {
		// todo: decide the behavior in case of empty source
		if dvContent, dvSchema, err = dataview.BlockBySource(req.SpaceId, c.sbtProvider, c.systemObjectService, source); err != nil {
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

	return c.CreateSmartBlockFromState(ctx, req.SpaceId, coresb.SmartBlockTypePage, []domain.TypeKey{bundle.TypeKeySet}, req.Details, newState)
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
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, key)
	if err != nil {
		return "", nil, err
	}
	object.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uniqueKey.Marshal())
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
	if pbtypes.GetInt64(details, bundle.RelationKeyRelationFormat.String()) == int64(model.RelationFormat_status) {
		object.Fields[bundle.RelationKeyRelationMaxCount.String()] = pbtypes.Int64(1)
	}
	// objectTypes := pbtypes.GetStringList(object, bundle.RelationKeyRelationFormatObjectTypes.String())
	// todo: check the objectTypes
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relation))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	return w.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypeRelation, []domain.TypeKey{bundle.TypeKeyRelation}, nil, createState)
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
		return "", nil, fmt.Errorf("invalid relation Key: unknown enum")
	}

	uniqueKey, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeRelationOption, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}

	object = pbtypes.CopyStruct(details)
	object.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uniqueKey.Marshal())
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Int64(int64(model.ObjectType_relationOption))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	return w.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypeRelationOption, []domain.TypeKey{bundle.TypeKeyRelationOption}, nil, createState)
}

func (w *Creator) createObjectType(ctx context.Context, spaceID string, details *types.Struct) (id string, newDetails *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create object type: no data")
	}

	uniqueKey, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeObjectType, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}
	details.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uniqueKey.Marshal())
	key := uniqueKey.InternalKey()

	object := pbtypes.CopyStruct(details)
	rawRecommendedLayout := pbtypes.GetInt64(details, bundle.RelationKeyRecommendedLayout.String())
	recommendedLayout, err := bundle.GetLayout(model.ObjectTypeLayout(int32(rawRecommendedLayout)))
	if err != nil {
		return "", nil, fmt.Errorf("invalid recommended layout %d: %w", rawRecommendedLayout, err)
	}

	recommendedRelationKeys := make([]string, 0, len(recommendedLayout.RequiredRelations))
	for _, rel := range recommendedLayout.RequiredRelations {
		recommendedRelationKeys = append(recommendedRelationKeys, rel.Key)
	}
	recommendedRelationIDs := make([]string, 0, len(recommendedRelationKeys))
	for _, relKey := range recommendedRelationKeys {
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, relKey)
		if err != nil {
			return "", nil, fmt.Errorf("failed to create unique Key: %w", err)
		}
		id, err := w.objectCache.DeriveObjectID(ctx, spaceID, uk)
		if err != nil {
			return "", nil, fmt.Errorf("failed to derive object id: %w", err)
		}
		recommendedRelationIDs = append(recommendedRelationIDs, id)
	}
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
	object.Fields[bundle.RelationKeyRecommendedLayout.String()] = pbtypes.Int64(rawRecommendedLayout)
	object.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(recommendedRelationIDs)

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
	if err != nil {
		return "", nil, fmt.Errorf("query bundled templates: %w", err)
	}

	alreadyInstalledTemplates, _, err := w.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(w.coreService.GetSystemTypeID(spaceID, bundle.TypeKeyTemplate)),
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

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	return w.CreateSmartBlockFromState(ctx, spaceID, coresb.SmartBlockTypeObjectType, []domain.TypeKey{bundle.TypeKeyObjectType}, nil, createState)
}

func getUniqueKeyOrGenerate(sbType coresb.SmartBlockType, details *types.Struct) (domain.UniqueKey, error) {
	uniqueKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
	if uniqueKey == "" {
		return domain.NewUniqueKey(sbType, bson.NewObjectId().Hex())
	}
	return domain.UnmarshalUniqueKey(uniqueKey)
}

func (c *Creator) CreateObject(ctx context.Context, spaceID string, req block.DetailsGetter, objectTypeKey domain.TypeKey) (id string, details *types.Struct, err error) {
	details = req.GetDetails()
	if details.GetFields() == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}

	var internalFlags []*model.InternalFlag
	if v, ok := req.(block.InternalFlagsGetter); ok {
		internalFlags = v.GetInternalFlags()
		details = internalflag.PutToDetails(details, internalFlags)
	}

	var templateID string
	if v, ok := req.(block.TemplateIDGetter); ok {
		templateID = v.GetTemplateId()
	}

	sbType := coresb.SmartBlockTypePage

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
		return c.CreateSmartBlockFromState(ctx, spaceID, sbType, []domain.TypeKey{bundle.TypeKeyCollection}, details, st)
	case bundle.TypeKeyObjectType:
		return c.createObjectType(ctx, spaceID, details)
	case bundle.TypeKeyRelation:
		return c.createRelation(ctx, spaceID, details)
	case bundle.TypeKeyRelationOption:
		return c.createRelationOption(ctx, spaceID, details)
	case bundle.TypeKeyTemplate:
		sbType = coresb.SmartBlockTypeTemplate
	}

	if templateID == block.BlankTemplateID {
		templateID = ""
	}

	return c.CreateSmartBlockFromTemplate(ctx, spaceID, sbType, []domain.TypeKey{objectTypeKey}, details, templateID)
}
