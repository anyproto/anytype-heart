package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

type CreatePayload struct {
	ObjectTypeKeys []domain.TypeKey
	State          *state.State
}

type CreationStrategy interface {
	PrepareCreatePayload(ctx context.Context, spaceId string) (*CreatePayload, error)
	AfterCreateOrIfExists() error
}

type Service interface {
	CreateObject(ctx context.Context, spaceID string, req CreateObjectRequest) (id string, details *types.Struct, err error)
	CreateObjectUsingObjectUniqueTypeKey(ctx context.Context, spaceID string, objectUniqueTypeKey string, req CreateObjectRequest) (id string, details *types.Struct, err error)
	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
	RegisterCreationStrategy(typeKey domain.TypeKey, strategy CreationStrategy)
	app.Component
}

type creator struct {
	blockService        BlockService
	objectCache         objectcache.Cache
	objectStore         objectstore.ObjectStore
	collectionService   CollectionService
	systemObjectService system_object.Service
	bookmark            bookmark.Service
	app                 *app.App
	sbtProvider         typeprovider.SmartBlockTypeProvider

	coreService core.Service // TODO: remove it?

	creationStrategies map[domain.TypeKey]CreationStrategy
}

type CollectionService interface {
	CreateCollection(details *types.Struct, flags []*model.InternalFlag) (coresb.SmartBlockType, *types.Struct, *state.State, error)
}

func NewCreator() Service {
	return &creator{}
}

func (c *creator) RegisterCreationStrategy(typeKey domain.TypeKey, strategy CreationStrategy) {
	c.creationStrategies[typeKey] = strategy
}

func (c *creator) Init(a *app.App) (err error) {
	c.blockService = app.MustComponent[BlockService](a)
	c.objectCache = a.MustComponent(objectcache.CName).(objectcache.Cache)
	c.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.collectionService = app.MustComponent[CollectionService](a)
	c.systemObjectService = app.MustComponent[system_object.Service](a)
	c.coreService = app.MustComponent[core.Service](a)
	c.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	c.app = a
	return nil
}

const CName = "objectCreator"

func (c *creator) Name() (name string) {
	return CName
}

// TODO Temporarily
type BlockService interface {
	StateFromTemplate(templateID, name string) (st *state.State, err error)
	TemplateClone(spaceID string, id string) (templateID string, err error)
}

func (c *creator) createSmartBlockFromTemplate(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error) {
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
func (c *creator) CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error) {
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

// ObjectCreateBookmark creates a new Bookmark object for provided URL or returns id of existing one
func (c *creator) ObjectCreateBookmark(ctx context.Context, req *pb.RpcObjectCreateBookmarkRequest) (objectID string, newDetails *types.Struct, err error) {
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

func getUniqueKeyOrGenerate(sbType coresb.SmartBlockType, details *types.Struct) (domain.UniqueKey, error) {
	uniqueKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
	if uniqueKey == "" {
		return domain.NewUniqueKey(sbType, bson.NewObjectId().Hex())
	}
	return domain.UnmarshalUniqueKey(uniqueKey)
}

// TODO Add validate method
type CreateObjectRequest struct {
	Details       *types.Struct
	InternalFlags []*model.InternalFlag
	TemplateId    string
	ObjectTypeKey domain.TypeKey
}

// CreateObject is high-level method for creating new objects
func (c *creator) CreateObject(ctx context.Context, spaceID string, req CreateObjectRequest) (id string, details *types.Struct, err error) {
	details = req.Details
	if details.GetFields() == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	details = internalflag.PutToDetails(details, req.InternalFlags)

	switch req.ObjectTypeKey {
	case bundle.TypeKeyBookmark:
		return c.ObjectCreateBookmark(ctx, &pb.RpcObjectCreateBookmarkRequest{
			Details: details,
			SpaceId: spaceID,
		})
	case bundle.TypeKeySet:
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_set))
		return c.CreateSet(ctx, &pb.RpcObjectCreateSetRequest{
			Details:       details,
			InternalFlags: req.InternalFlags,
			Source:        pbtypes.GetStringList(details, bundle.RelationKeySetOf.String()),
			SpaceId:       spaceID,
		})
	case bundle.TypeKeyCollection:
		var st *state.State
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
		_, details, st, err = c.collectionService.CreateCollection(details, req.InternalFlags)
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

	if req.TemplateId == template.BlankTemplateID {
		req.TemplateId = ""
	}

	return c.createSmartBlockFromTemplate(ctx, spaceID, []domain.TypeKey{req.ObjectTypeKey}, details, req.TemplateId)
}

func (c *creator) CreateObjectUsingObjectUniqueTypeKey(ctx context.Context, spaceID string, objectUniqueTypeKey string, req CreateObjectRequest) (id string, details *types.Struct, err error) {
	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(objectUniqueTypeKey)
	if err != nil {
		return "", nil, fmt.Errorf("get type key from raw unique key: %w", err)
	}
	req.ObjectTypeKey = objectTypeKey
	return c.CreateObject(ctx, spaceID, req)
}
