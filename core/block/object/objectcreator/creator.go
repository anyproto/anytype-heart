package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/pkg/errors"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/internalflag"
)

type (
	templateService interface {
		CreateTemplateStateWithDetails(templateId string, details *domain.Details, withTemplateValidation bool) (st *state.State, err error)
		TemplateCloneInSpace(space clientspace.Space, id string) (templateId string, err error)
		SetDefaultTemplateInType(ctx context.Context, typeId, templateId string) error
	}

	bookmarkService interface {
		CreateObjectAndFetch(ctx context.Context, spaceId string, details *domain.Details) (objectID string, newDetails *domain.Details, err error)
	}

	objectArchiver interface {
		SetIsArchived(objectId string, isArchived bool) error
	}
)

const CName = "objectCreator"

var log = logging.Logger(CName)

type Service interface {
	CreateObject(ctx context.Context, spaceID string, req CreateObjectRequest) (id string, details *domain.Details, err error)
	CreateObjectUsingObjectUniqueTypeKey(ctx context.Context, spaceID string, objectUniqueTypeKey string, req CreateObjectRequest) (id string, details *domain.Details, err error)

	CreateSmartBlockFromState(ctx context.Context, spaceID string, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *domain.Details, err error)
	CreateSmartBlockFromStateInSpace(ctx context.Context, space clientspace.Space, objectTypeKeys []domain.TypeKey, createState *state.State) (id string, newDetails *domain.Details, err error)
	AddChatDerivedObject(ctx context.Context, space clientspace.Space, chatObjectId string) (chatId string, err error)

	InstallBundledObjects(ctx context.Context, space clientspace.Space, sourceObjectIds []string, isNewSpace bool) (ids []string, objects []*domain.Details, err error)
	app.Component
}

type service struct {
	objectStore     objectstore.ObjectStore
	bookmarkService bookmarkService
	spaceService    space.Service
	templateService templateService
	archiver        objectArchiver
}

func NewCreator() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.bookmarkService = app.MustComponent[bookmarkService](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.templateService = app.MustComponent[templateService](a)
	s.archiver = app.MustComponent[objectArchiver](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

// TODO Add validate method
type CreateObjectRequest struct {
	Details       *domain.Details
	InternalFlags []*model.InternalFlag
	TemplateId    string
	ObjectTypeKey domain.TypeKey
}

// CreateObject is high-level method for creating new objects
func (s *service) CreateObject(ctx context.Context, spaceID string, req CreateObjectRequest) (id string, details *domain.Details, err error) {
	space, err := s.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	return s.createObjectInSpace(ctx, space, req)
}

func (s *service) CreateObjectUsingObjectUniqueTypeKey(
	ctx context.Context, spaceID string, objectUniqueTypeKey string, req CreateObjectRequest,
) (id string, details *domain.Details, err error) {
	objectTypeKey, err := domain.GetTypeKeyFromRawUniqueKey(objectUniqueTypeKey)
	if err != nil {
		return "", nil, fmt.Errorf("get type key from raw unique key: %w", err)
	}
	req.ObjectTypeKey = objectTypeKey
	return s.CreateObject(ctx, spaceID, req)
}

// createObjectInSpace is supposed to be called for user-initiated object creation requests
// will return Restricted error in case called with types like File or Participant
func (s *service) createObjectInSpace(
	ctx context.Context, space clientspace.Space, req CreateObjectRequest,
) (id string, details *domain.Details, err error) {
	details = req.Details
	if details == nil {
		details = domain.NewDetails()
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
		return s.createTemplate(ctx, space, details, req)
	case bundle.TypeKeyDate:
		return buildDateObject(space, details)
	default:
		return s.createCommonObject(ctx, space, details, req)
	}
}

func (s *service) createTemplate(
	ctx context.Context, space clientspace.Space, details *domain.Details, req CreateObjectRequest,
) (id string, resultDetails *domain.Details, err error) {
	target := details.GetString(bundle.RelationKeyTargetObjectType)
	if target == "" {
		return "", nil, fmt.Errorf("cannot create template without target object type")
	}
	layout, err := s.getTypeRecommendedLayout(domain.FullID{SpaceID: space.Id(), ObjectID: target})
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch target object type from store: %w", err)
	}
	details.SetInt64(bundle.RelationKeyResolvedLayout, layout)

	typeId, err := space.DeriveObjectID(ctx, domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, req.ObjectTypeKey.String()))
	if err != nil {
		return "", nil, fmt.Errorf("failed to derive object type id: %w", err)
	}
	// we should enrich details with spaceId, type and layout to use type object to form state of new object
	details.Set(bundle.RelationKeySpaceId, domain.String(space.Id()))
	details.Set(bundle.RelationKeyType, domain.String(typeId))

	createState, err := s.templateService.CreateTemplateStateWithDetails("", details, false)
	if err != nil {
		return
	}
	id, resultDetails, err = s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{req.ObjectTypeKey}, createState)
	if e := s.templateService.SetDefaultTemplateInType(ctx, target, id); e != nil {
		log.Errorf("failed to set defaultTemplateId to type: %v", e)
	}
	return
}

func (s *service) createCommonObject(
	ctx context.Context, space clientspace.Space, details *domain.Details, req CreateObjectRequest,
) (id string, resultDetails *domain.Details, err error) {
	typeId, err := space.DeriveObjectID(ctx, domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, req.ObjectTypeKey.String()))
	if err != nil {
		return "", nil, fmt.Errorf("failed to derive object type id: %w", err)
	}

	layout, err := s.getTypeRecommendedLayout(domain.FullID{SpaceID: space.Id(), ObjectID: typeId})
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch target object type from store: %w", err)
	}

	// we should enrich details with spaceId, type and layout to use type object to form state of new object
	details.Set(bundle.RelationKeySpaceId, domain.String(space.Id()))
	details.Set(bundle.RelationKeyType, domain.String(typeId))
	details.Set(bundle.RelationKeyResolvedLayout, domain.Int64(layout))

	createState, err := s.templateService.CreateTemplateStateWithDetails(req.TemplateId, details, true)
	if err != nil {
		return
	}
	return s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{req.ObjectTypeKey}, createState)
}

func (s *service) getTypeRecommendedLayout(typeId domain.FullID) (int64, error) {
	ot, err := s.objectStore.SpaceIndex(typeId.SpaceID).GetObjectType(typeId.ObjectID)
	if err != nil {
		return 0, err
	}
	return int64(ot.Layout), nil
}

// buildDateObject does not create real date object. It just builds date object details
func buildDateObject(space clientspace.Space, details *domain.Details) (string, *domain.Details, error) {
	ts := details.GetInt64(bundle.RelationKeyTimestamp)
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

func setOriginalCreatedTimestamp(state *state.State, details *domain.Details) {
	if createDate := details.GetInt64(bundle.RelationKeyCreatedDate); createDate != 0 {
		state.SetOriginalCreatedTimestamp(createDate)
	}
}
