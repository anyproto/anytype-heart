package detailservice

import (
	"context"
	"errors"
	"slices"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filegc"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "details.service"

var log = logger.NewNamed(CName)

type Service interface {
	app.Component

	SetDetails(ctx session.Context, objectId string, details []domain.Detail) error
	SetDetailsList(ctx session.Context, objectIds []string, details []domain.Detail) error
	ModifyDetails(ctx session.Context, objectId string, modifier func(current *domain.Details) (*domain.Details, error)) error
	ModifyDetailsList(req *pb.RpcObjectListModifyDetailValuesRequest) error

	ObjectTypeAddRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error
	ObjectTypeRemoveRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error
	ObjectTypeSetRelations(objectTypeId string, relationObjectIds []string) error
	ObjectTypeSetFeaturedRelations(objectTypeId string, relationObjectIds []string) error
	ObjectTypeListConflictingRelations(spaceId, typeKey string) (relationObjectIds []string, err error)

	ListRelationsWithValue(spaceId string, value domain.Value) ([]*pb.RpcRelationListWithValueResponseResponseItem, error)

	SetSpaceInfo(spaceId string, details *domain.Details) error
	SetWorkspaceDashboardId(ctx session.Context, workspaceId string, id string) (setId string, err error)

	SetIsFavorite(objectId string, isFavorite bool) error
	SetIsArchived(objectId string, isArchived bool) error
	SetListIsFavorite(objectIds []string, isFavorite bool) error
	SetListIsArchived(objectIds []string, isArchived bool) error
}

func New() Service {
	return &service{}
}

type service struct {
	objectGetter cache.ObjectGetter
	resolver     idresolver.Resolver
	spaceService space.Service
	store        objectstore.ObjectStore
	fileGC       filegc.FileGC
}

func (s *service) Init(a *app.App) error {
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	s.resolver = app.MustComponent[idresolver.Resolver](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.store = app.MustComponent[objectstore.ObjectStore](a)
	s.fileGC = app.MustComponent[filegc.FileGC](a)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) SetDetails(ctx session.Context, objectId string, details []domain.Detail) (err error) {
	return cache.Do(s.objectGetter, objectId, func(b basic.DetailsSettable) error {
		return b.SetDetails(ctx, details, true)
	})
}

func (s *service) SetDetailsList(ctx session.Context, objectIds []string, details []domain.Detail) (resultError error) {
	var anySucceed bool
	for _, objectId := range objectIds {
		err := s.SetDetails(ctx, objectId, details)
		if err != nil {
			resultError = errors.Join(resultError, err)
		} else {
			anySucceed = true
		}
	}
	if resultError != nil {
		log.Warn("SetDetailsList", zap.Error(resultError))
	}
	if anySucceed {
		return nil
	}
	return resultError
}

// ModifyDetails performs details get and update under the sb lock to make sure no modifications are done in the middle
func (s *service) ModifyDetails(ctx session.Context, objectId string, modifier func(current *domain.Details) (*domain.Details, error)) (err error) {
	return cache.Do(s.objectGetter, objectId, func(du basic.DetailsUpdatable) error {
		return du.UpdateDetails(ctx, modifier)
	})
}

func (s *service) ModifyDetailsList(req *pb.RpcObjectListModifyDetailValuesRequest) (resultError error) {
	var anySucceed bool
	for _, objectId := range req.ObjectIds {
		err := s.ModifyDetails(nil, objectId, func(current *domain.Details) (*domain.Details, error) {
			for _, op := range req.Operations {
				if op.Set != nil {
					// Set operation has higher priority than Add and Remove, because it modifies full value
					current.Set(domain.RelationKey(op.RelationKey), domain.ValueFromProto(op.Set))
					continue
				}
				addValueToListDetail(current, domain.RelationKey(op.RelationKey), domain.ValueFromProto(op.Add))
				removeValueFromListDetail(current, domain.RelationKey(op.RelationKey), domain.ValueFromProto(op.Remove))
			}
			return current, nil
		})
		if err != nil {
			resultError = errors.Join(resultError, err)
		} else {
			anySucceed = true
		}
	}
	if resultError != nil {
		log.Warn("ModifyDetailsList", zap.Error(resultError))
	}
	if anySucceed {
		return nil
	}
	return resultError
}

// addValueToListDetail adds values to int lists and string lists
func addValueToListDetail(s *domain.Details, key domain.RelationKey, v domain.Value) {
	if s.Len() == 0 || v.IsNull() {
		return
	}
	toAdd := v.WrapToList()
	oldValues := s.Get(key).WrapToList()
	newValues := slice.MergeUniqBy(oldValues, toAdd, func(this domain.Value, that domain.Value) bool {
		return this.Equal(that)
	})
	s.Set(key, domain.ValueList(newValues))
}

// removeValueFromListDetail removes values from int lists and string lists
func removeValueFromListDetail(s *domain.Details, key domain.RelationKey, v domain.Value) {
	if s.Len() == 0 || v.IsNull() {
		return
	}
	value, ok := s.TryGet(key)
	if !ok {
		return
	}
	if value.Equal(v) {
		s.Delete(key)
		return
	}
	oldValues := value.WrapToList()
	if len(oldValues) == 0 {
		return
	}
	toDelete := v.WrapToList()
	newValues := lo.Filter(oldValues, func(oldValue domain.Value, _ int) bool {
		return !slices.ContainsFunc(toDelete, func(valueToDelete domain.Value) bool {
			return oldValue.Equal(valueToDelete)
		})
	})

	if len(newValues) == 0 {
		if value.IsStringList() {
			s.Set(key, domain.StringList(nil))
		} else {
			s.Set(key, domain.Float64List(nil))
		}
	} else {
		s.Set(key, domain.ValueList(newValues))

	}

}
