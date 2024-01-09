package objectcreator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/objecttype"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("object-service")

type eventKey int

const eventCreate eventKey = 0

type Service interface {
	CreateObject(ctx context.Context, spaceID string, req CreateObjectRequest) (id string, details *types.Struct, err error)
	CreateObjectInSpace(ctx context.Context, space clientspace.Space, req CreateObjectRequest) (id string, details *types.Struct, err error)
	CreateObjectUsingObjectUniqueTypeKey(ctx context.Context, spaceID string, objectUniqueTypeKey string, req CreateObjectRequest) (id string, details *types.Struct, err error)
	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromStateInSpace(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)

	InstallBundledObjects(ctx context.Context, space clientspace.Space, sourceObjectIds []string) (ids []string, objects []*types.Struct, err error)
	app.Component
}

type service struct {
	objectStore       objectstore.ObjectStore
	collectionService CollectionService
	bookmark          bookmark.Service
	app               *app.App
	spaceService      space.Service
	templateService   TemplateService
	fileService       files.Service
}

type CollectionService interface {
	CreateCollection(details *types.Struct, flags []*model.InternalFlag) (coresb.SmartBlockType, *types.Struct, *state.State, error)
}

type TemplateService interface {
	CreateTemplateStateWithDetails(templateId string, details *types.Struct) (st *state.State, err error)
	TemplateCloneInSpace(space clientspace.Space, id string) (templateId string, err error)
}

func NewCreator() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	s.collectionService = app.MustComponent[CollectionService](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.templateService = app.MustComponent[TemplateService](a)
	s.fileService = app.MustComponent[files.Service](a)
	s.app = a
	return nil
}

const CName = "objectCreator"

func (s *service) Name() (name string) {
	return CName
}

// CreateSmartBlockFromState create new object from the provided `createState` and `details`.
// If you pass `details` into the function, it will automatically add missing relationLinks and override the details from the `createState`
// It will return error if some of the relation keys in `details` not installed in the workspace.
func (s *service) CreateSmartBlockFromState(
	ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State,
) (id string, newDetails *types.Struct, err error) {
	spc, err := s.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", nil, err
	}
	return s.CreateSmartBlockFromStateInSpace(ctx, spc, objectTypeKeys, createState)
}

func (s *service) CreateSmartBlockFromStateInSpace(
	ctx context.Context, spc clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State,
) (id string, newDetails *types.Struct, err error) {
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
	sbType := objectTypeKeysToSmartBlockType(objectTypeKeys)

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
			RelationKeys:   generateRelationKeysFromState(createState),
			SpaceID:        spc.Id(),
		}
	}

	sb, err := createSmartBlock(ctx, spc, initFunc, createState, sbType)
	if err != nil {
		return "", nil, err
	}

	newDetails = sb.CombinedDetails()
	id = sb.Id()

	if sbType == coresb.SmartBlockTypeObjectType {
		objecttype.UpdateLastUsedDate(spc, s.objectStore, []domain.TypeKey{
			domain.TypeKey(strings.TrimPrefix(pbtypes.GetString(newDetails, bundle.RelationKeyUniqueKey.String()), addr.ObjectTypeKeyToIdPrefix)),
		})
	} else if pbtypes.GetInt64(newDetails, bundle.RelationKeyOrigin.String()) == int64(model.ObjectOrigin_none) {
		objecttype.UpdateLastUsedDate(spc, s.objectStore, objectTypeKeys)
	}

	ev.SmartblockCreateMs = time.Since(startTime).Milliseconds() - ev.SetDetailsMs - ev.WorkspaceCreateMs - ev.GetWorkspaceBlockWaitMs
	ev.SmartblockType = int(sbType)
	ev.ObjectId = id
	metrics.SharedClient.RecordEvent(*ev)
	return id, newDetails, nil
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

func (s *service) CreateObject(ctx context.Context, spaceID string, req CreateObjectRequest) (id string, details *types.Struct, err error) {
	space, err := s.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	return s.CreateObjectInSpace(ctx, space, req)
}

// CreateObjectInSpace is high-level method for creating new objects
func (s *service) CreateObjectInSpace(ctx context.Context, space clientspace.Space, req CreateObjectRequest) (id string, details *types.Struct, err error) {
	details = req.Details
	if details.GetFields() == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	details = internalflag.PutToDetails(details, req.InternalFlags)

	switch req.ObjectTypeKey {
	case bundle.TypeKeyBookmark:
		return s.ObjectCreateBookmark(ctx, space.Id(), &pb.RpcObjectCreateBookmarkRequest{
			Details: details,
		})
	case bundle.TypeKeySet:
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_set))
		return s.CreateSet(ctx, space, &pb.RpcObjectCreateSetRequest{
			Details:       details,
			InternalFlags: req.InternalFlags,
			Source:        pbtypes.GetStringList(details, bundle.RelationKeySetOf.String()),
		})
	case bundle.TypeKeyCollection:
		var st *state.State
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
		_, details, st, err = s.collectionService.CreateCollection(details, req.InternalFlags)
		if err != nil {
			return "", nil, err
		}
		return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyCollection}, st)
	case bundle.TypeKeyObjectType:
		return s.createObjectType(ctx, space, details)
	case bundle.TypeKeyRelation:
		return s.createRelation(ctx, space, details)
	case bundle.TypeKeyRelationOption:
		return s.createRelationOption(ctx, space, details)
	case bundle.TypeKeyFile:
		return "", nil, fmt.Errorf("files must be created via fileobject service")
	}

	return s.createSmartBlockFromTemplate(ctx, space, []domain.TypeKey{req.ObjectTypeKey}, details, req.TemplateId)
}

func (s *service) CreateObjectUsingObjectUniqueTypeKey(ctx context.Context, spaceID string, objectUniqueTypeKey string, req CreateObjectRequest) (id string, details *types.Struct, err error) {
	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(objectUniqueTypeKey)
	if err != nil {
		return "", nil, fmt.Errorf("get type key from raw unique key: %w", err)
	}
	req.ObjectTypeKey = objectTypeKey
	return s.CreateObject(ctx, spaceID, req)
}

func (s *service) createSmartBlockFromTemplate(
	ctx context.Context,
	space clientspace.Space,
	objectTypeKeys []domain.TypeKey,
	details *types.Struct,
	templateId string,
) (id string, newDetails *types.Struct, err error) {
	createState, err := s.templateService.CreateTemplateStateWithDetails(templateId, details)
	if err != nil {
		return
	}
	return s.CreateSmartBlockFromStateInSpace(ctx, space, objectTypeKeys, createState)
}

func objectTypeKeysToSmartBlockType(typeKeys []domain.TypeKey) coresb.SmartBlockType {
	// TODO Add validation for types that user can't create

	if slices.Contains(typeKeys, bundle.TypeKeyTemplate) {
		return coresb.SmartBlockTypeTemplate
	}
	typeKey := typeKeys[0]

	switch typeKey {
	case bundle.TypeKeyObjectType:
		return coresb.SmartBlockTypeObjectType
	case bundle.TypeKeyRelation:
		return coresb.SmartBlockTypeRelation
	case bundle.TypeKeyRelationOption:
		return coresb.SmartBlockTypeRelationOption
	case bundle.TypeKeyFile, bundle.TypeKeyImage, bundle.TypeKeyAudio, bundle.TypeKeyVideo:
		return coresb.SmartBlockTypeFileObject
	default:
		return coresb.SmartBlockTypePage
	}
}

func createSmartBlock(
	ctx context.Context, spc clientspace.Space, initFunc objectcache.InitFunc, createState *state.State, sbType coresb.SmartBlockType,
) (smartblock.SmartBlock, error) {
	var sb smartblock.SmartBlock
	if uKey := createState.UniqueKeyInternal(); uKey != "" {
		uk, err := domain.NewUniqueKey(sbType, uKey)
		if err != nil {
			return nil, err
		}
		if sbType == coresb.SmartBlockTypeFileObject {
			return spc.DeriveTreeObjectWithAccountSignature(ctx, objectcache.TreeDerivationParams{
				Key:      uk,
				InitFunc: initFunc,
			})
		} else {
			return spc.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
				Key:      uk,
				InitFunc: initFunc,
			})
		}

	} else {
		return spc.CreateTreeObject(ctx, objectcache.TreeCreationParams{
			Time:           time.Now(),
			SmartblockType: sbType,
			InitFunc:       initFunc,
		})
	}
}

func generateRelationKeysFromState(st *state.State) (relationKeys []string) {
	if st == nil {
		return
	}
	details := st.CombinedDetails().GetFields()
	relationKeys = make([]string, 0, len(details))
	for k := range details {
		relationKeys = append(relationKeys, k)
	}
	return
}
