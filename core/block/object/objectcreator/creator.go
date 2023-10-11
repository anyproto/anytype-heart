package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("object-service")

type eventKey int

const eventCreate eventKey = 0

type Service interface {
	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSet(ctx context.Context, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error)
	app.Component
}

type Creator struct {
	blockService      BlockService
	blockPicker       block.ObjectGetter
	objectStore       objectstore.ObjectStore
	collectionService CollectionService
	bookmark          bookmark.Service
	app               *app.App
	spaceService      space.Service
}

type CollectionService interface {
	CreateCollection(details *types.Struct, flags []*model.InternalFlag) (coresb.SmartBlockType, *types.Struct, *state.State, error)
}

func NewCreator() *Creator {
	return &Creator{}
}

func (c *Creator) Init(a *app.App) (err error) {
	c.blockService = a.MustComponent(block.CName).(BlockService)
	c.blockPicker = a.MustComponent(block.CName).(block.ObjectGetter)
	c.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.collectionService = app.MustComponent[CollectionService](a)
	c.spaceService = app.MustComponent[space.Service](a)
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
	TemplateCloneInSpace(space space.Space, id string) (templateID string, err error)
}

func (c *Creator) createSmartBlockFromTemplate(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateID != "" {
		if createState, err = c.blockService.StateFromTemplate(templateID, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return c.CreateSmartBlockFromState(ctx, spaceID, objectTypeKeys, createState)
}

func objectTypeKeysToSmartblockType(typeKeys []domain.TypeKey) (coresb.SmartBlockType, error) {
	// TODO Add validation for types that user can't create

	if slices.Contains(typeKeys, bundle.TypeKeyTemplate) {
		return coresb.SmartBlockTypeTemplate, nil
	}
	typeKey := typeKeys[0]

	switch typeKey {
	case bundle.TypeKeyObjectType:
		return coresb.SmartBlockTypeObjectType, nil
	case bundle.TypeKeyRelation:
		return coresb.SmartBlockTypeRelation, nil
	case bundle.TypeKeyRelationOption:
		return coresb.SmartBlockTypeRelationOption, nil
	default:
		return coresb.SmartBlockTypePage, nil
	}
}

// CreateSmartBlockFromState create new object from the provided `createState` and `details`. If you pass `details` into the function, it will automatically add missing relationLinks and override the details from the `createState`
// It will return error if some of the relation keys in `details` not installed in the workspace.
func (c *Creator) CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error) {
	spc, err := c.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", nil, err
	}
	return c.CreateSmartBlockFromStateInSpace(ctx, spc, objectTypeKeys, createState)
}

func (c *Creator) CreateSmartBlockFromStateInSpace(ctx context.Context, spc space.Space, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error) {
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
		objectTypeKeys = []domain.TypeKey{bundle.TypeKeyPage}
	}
	sbType, err := objectTypeKeysToSmartblockType(objectTypeKeys)
	if err != nil {
		return "", nil, fmt.Errorf("objectTypeKey to smartblockType: %w", err)
	}

	var relationKeys []string
	for k, v := range createState.Details().GetFields() {
		relationKeys = append(relationKeys, k)
		createState.SetDetail(k, v)
	}
	for k, v := range createState.LocalDetails().GetFields() {
		relationKeys = append(relationKeys, k)
		createState.SetDetail(k, v)
	}
	createState.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, pbtypes.String(spc.Id()))

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
			SpaceID:        spc.Id(),
		}
	}

	var sb smartblock.SmartBlock
	if uKey := createState.UniqueKeyInternal(); uKey != "" {
		uk, err := domain.NewUniqueKey(sbType, uKey)
		if err != nil {
			return "", nil, err
		}
		sb, err = spc.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
			Key:      uk,
			InitFunc: initFunc,
		})
		if err != nil {
			return "", nil, err
		}
	} else {
		sb, err = spc.CreateTreeObject(ctx, objectcache.TreeCreationParams{
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

	dvContent, err := dataview.BlockBySource(c.objectStore, req.Source)
	if err != nil {
		return
	}

	newState := state.NewDoc("", nil).NewState()
	if len(req.Source) > 0 {
		newState.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(req.Source))
	}

	tmpls := []template.StateTransformer{
		template.WithRequiredRelations(),
	}

	for i, view := range dvContent.Dataview.Views {
		if view.Relations == nil {
			dvContent.Dataview.Views[i].Relations = editor.GetDefaultViewRelations(dvContent.Dataview.Relations)
		}
	}
	tmpls = append(tmpls,
		template.WithDataview(dvContent, false),
	)

	template.InitTemplate(newState, tmpls...)

	return c.CreateSmartBlockFromState(ctx, req.SpaceId, []domain.TypeKey{bundle.TypeKeySet}, newState)
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

func (c *Creator) createRelation(ctx context.Context, spaceID string, details *types.Struct) (id string, object *types.Struct, err error) {
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
	return c.CreateSmartBlockFromState(ctx, spaceID, []domain.TypeKey{bundle.TypeKeyRelation}, createState)
}

func (c *Creator) createRelationOption(ctx context.Context, spaceID string, details *types.Struct) (id string, object *types.Struct, err error) {
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
	return c.CreateSmartBlockFromState(ctx, spaceID, []domain.TypeKey{bundle.TypeKeyRelationOption}, createState)
}

func (c *Creator) createObjectType(ctx context.Context, spaceID string, details *types.Struct) (id string, newDetails *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create object type: no data")
	}

	uniqueKey, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeObjectType, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}
	details.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uniqueKey.Marshal())

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
	spc, err := c.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	for _, relKey := range recommendedRelationKeys {
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, relKey)
		if err != nil {
			return "", nil, fmt.Errorf("failed to create unique Key: %w", err)
		}
		id, err := spc.DeriveObjectID(ctx, uk)
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

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	id, newDetails, err = c.CreateSmartBlockFromState(ctx, spaceID, []domain.TypeKey{bundle.TypeKeyObjectType}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	installingObjectTypeKey := domain.TypeKey(uniqueKey.InternalKey())
	err = c.installTemplatesForObjectType(spc, installingObjectTypeKey)
	if err != nil {
		log.With("spaceID", spaceID, "objectTypeKey", installingObjectTypeKey).Errorf("error while installing templates: %s", err)
	}
	return id, newDetails, nil
}

func (c *Creator) installTemplatesForObjectType(spc space.Space, typeKey domain.TypeKey) error {
	bundledTemplates, _, err := c.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(bundle.TypeKeyTemplate.BundledURL()),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(typeKey.BundledURL()),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query bundled templates: %w", err)
	}

	installedTemplatesIDs, err := c.listInstalledTemplatesForType(spc, typeKey)
	if err != nil {
		return fmt.Errorf("list installed templates: %w", err)
	}

	for _, record := range bundledTemplates {
		id := pbtypes.GetString(record.Details, bundle.RelationKeyId.String())
		if _, exists := installedTemplatesIDs[id]; exists {
			continue
		}

		_, err := c.blockService.TemplateCloneInSpace(spc, id)
		if err != nil {
			return fmt.Errorf("clone template: %w", err)
		}
	}
	return nil
}

func (c *Creator) listInstalledTemplatesForType(spc space.Space, typeKey domain.TypeKey) (map[string]struct{}, error) {
	templateTypeID, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeyTemplate)
	if err != nil {
		return nil, fmt.Errorf("get template type id by key: %w", err)
	}
	targetObjectTypeID, err := spc.GetTypeIdByKey(context.Background(), typeKey)
	if err != nil {
		return nil, fmt.Errorf("get type id by key: %w", err)
	}
	alreadyInstalledTemplates, _, err := c.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(templateTypeID),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(targetObjectTypeID),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spc.Id()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	existingTemplatesMap := map[string]struct{}{}
	for _, rec := range alreadyInstalledTemplates {
		sourceObject := pbtypes.GetString(rec.Details, bundle.RelationKeySourceObject.String())
		if sourceObject != "" {
			existingTemplatesMap[sourceObject] = struct{}{}
		}
	}
	return existingTemplatesMap, nil
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
		_, details, st, err = c.collectionService.CreateCollection(details, internalFlags)
		if err != nil {
			return "", nil, err
		}
		return c.CreateSmartBlockFromState(ctx, spaceID, []domain.TypeKey{bundle.TypeKeyCollection}, st)
	case bundle.TypeKeyObjectType:
		return c.createObjectType(ctx, spaceID, details)
	case bundle.TypeKeyRelation:
		return c.createRelation(ctx, spaceID, details)
	case bundle.TypeKeyRelationOption:
		return c.createRelationOption(ctx, spaceID, details)
	}

	if templateID == block.BlankTemplateID {
		templateID = ""
	}

	return c.createSmartBlockFromTemplate(ctx, spaceID, []domain.TypeKey{objectTypeKey}, details, templateID)
}

// TODO Temporarily home. Refactor to use CreateObject after object creator refactoring
func (c *Creator) DeriveTreeObject(ctx context.Context, spaceID string, params objectcache.TreeDerivationParams) (sb smartblock.SmartBlock, err error) {
	spc, err := c.spaceService.Get(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	return spc.DeriveTreeObject(ctx, params)
}
