package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"

	"github.com/anyproto/anytype-heart/core/block/editor/lastused"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type (
	collectionService interface {
		CreateCollection(details *types.Struct, flags []*model.InternalFlag) (coresb.SmartBlockType, *types.Struct, *state.State, error)
	}

	templateService interface {
		CreateTemplateStateWithDetails(templateId string, details *types.Struct) (st *state.State, err error)
		TemplateCloneInSpace(space clientspace.Space, id string) (templateId string, err error)
	}

	bookmarkService interface {
		CreateObjectAndFetch(ctx context.Context, spaceId string, details *types.Struct) (objectID string, newDetails *types.Struct, err error)
	}

	installer interface {
		InstallBundledObjects(ctx context.Context, space clientspace.Space, sourceObjectIds []string, isNewSpace bool) (ids []string, objects []*types.Struct, err error)
	}
)

const CName = "objectCreator"

var log = logging.Logger(CName)

type Service interface {
	CreateObject(ctx context.Context, spaceID string, req CreateObjectRequest) (id string, details *types.Struct, err error)
	CreateObjectUsingObjectUniqueTypeKey(ctx context.Context, spaceID string, objectUniqueTypeKey string, req CreateObjectRequest) (id string, details *types.Struct, err error)
	CreateObjectInSpace(ctx context.Context, space clientspace.Space, req CreateObjectRequest) (id string, details *types.Struct, err error)

	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromStateInSpace(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *types.Struct, err error)
	AddChatDerivedObject(ctx context.Context, space clientspace.Space, chatObjectId string) (chatId string, err error)

	CreateTemplatesForObjectType(spc clientspace.Space, typeKey domain.TypeKey) error

	app.Component
}

type service struct {
	objectStore       objectstore.ObjectStore
	collectionService collectionService
	bookmarkService   bookmarkService
	spaceService      space.Service
	templateService   templateService
	lastUsedUpdater   lastused.ObjectUsageUpdater
	installer         installer
}

func NewCreator() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.bookmarkService = app.MustComponent[bookmarkService](a)
	s.collectionService = app.MustComponent[collectionService](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.templateService = app.MustComponent[templateService](a)
	s.lastUsedUpdater = app.MustComponent[lastused.ObjectUsageUpdater](a)
	s.installer = app.MustComponent[installer](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

// TODO Add validate method
type CreateObjectRequest struct {
	Details       *types.Struct
	InternalFlags []*model.InternalFlag
	TemplateId    string
	ObjectTypeKey domain.TypeKey
}

// CreateObject is high-level method for creating new objects
func (s *service) CreateObject(ctx context.Context, spaceID string, req CreateObjectRequest) (id string, details *types.Struct, err error) {
	space, err := s.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	return s.CreateObjectInSpace(ctx, space, req)
}

func (s *service) CreateObjectUsingObjectUniqueTypeKey(
	ctx context.Context, spaceID string, objectUniqueTypeKey string, req CreateObjectRequest,
) (id string, details *types.Struct, err error) {
	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(objectUniqueTypeKey)
	if err != nil {
		return "", nil, fmt.Errorf("get type key from raw unique key: %w", err)
	}
	req.ObjectTypeKey = objectTypeKey
	return s.CreateObject(ctx, spaceID, req)
}

// CreateObjectInSpace is supposed to be called for user-initiated object creation requests
// will return Restricted error in case called with types like File or Participant
func (s *service) CreateObjectInSpace(
	ctx context.Context, space clientspace.Space, req CreateObjectRequest,
) (id string, details *types.Struct, err error) {
	details = req.Details
	if details.GetFields() == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}
	details = internalflag.PutToDetails(details, req.InternalFlags)

	if bundle.HasObjectTypeByKey(req.ObjectTypeKey) {
		if t := bundle.MustGetType(req.ObjectTypeKey); t.RestrictObjectCreation {
			return "", nil, errors.Wrap(restriction.ErrRestricted, "creation of this object type is restricted")
		}
	}
	switch req.ObjectTypeKey {
	case bundle.TypeKeyBookmark:
		return s.bookmarkService.CreateObjectAndFetch(ctx, space.Id(), details)
	case bundle.TypeKeySet:
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_set))
		return s.createSet(ctx, space, &pb.RpcObjectCreateSetRequest{
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
	case bundle.TypeKeyChatDerived:
		return s.createChatDerived(ctx, space, details)
	case bundle.TypeKeyFile:
		return "", nil, fmt.Errorf("files must be created via fileobject service")
	case bundle.TypeKeyTemplate:
		if pbtypes.GetString(details, bundle.RelationKeyTargetObjectType.String()) == "" {
			return "", nil, fmt.Errorf("cannot create template without target object")
		}
	case bundle.TypeKeyDate:
		return buildDateObject(space, details)
	}

	return s.createObjectFromTemplate(ctx, space, []domain.TypeKey{req.ObjectTypeKey}, details, req.TemplateId)
}

func (s *service) createObjectFromTemplate(
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

// buildDateObject does not create real date object. It just builds date object details
func buildDateObject(space clientspace.Space, details *types.Struct) (string, *types.Struct, error) {
	ts := pbtypes.GetInt64(details, bundle.RelationKeyTimestamp.String())
	dateObject := dateutil.NewDateObject(time.Unix(ts, 0), false)

	typeId, err := space.GetTypeIdByKey(context.Background(), bundle.TypeKeyDate)
	if err != nil {
		return "", nil, fmt.Errorf("failed to find Date type to build Date object: %w", err)
	}

	dateSource := source.NewDate(source.DateSourceParams{
		Id: domain.FullID{
			ObjectID: dateObject.Id(),
			SpaceID:  space.Id(),
		},
		DateObjectTypeId: typeId,
	})

	detailsGetter, ok := dateSource.(source.SourceIdEndodedDetails)
	if !ok {
		return "", nil, fmt.Errorf("date object does not implement DetailsFromId")
	}

	details, err = detailsGetter.DetailsFromId()
	return dateObject.Id(), details, err
}
