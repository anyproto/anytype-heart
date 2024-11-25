package detailservice

import (
	"context"
	"errors"
	"slices"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "details.service"

var log = logger.NewNamed(CName)

type Service interface {
	app.Component

	SetDetails(ctx session.Context, objectId string, details []*model.Detail) error
	SetDetailsAndUpdateLastUsed(ctx session.Context, objectId string, details []*model.Detail) error
	SetDetailsList(ctx session.Context, objectIds []string, details []*model.Detail) error
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) error
	ModifyDetailsList(req *pb.RpcObjectListModifyDetailValuesRequest) error

	ObjectTypeAddRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error
	ObjectTypeRemoveRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error
	ObjectTypeSetRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error
	ObjectTypeSetFeaturedRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error

	ListRelationsWithValue(spaceId string, value *types.Value) ([]*pb.RpcRelationListWithValueResponseResponseItem, error)

	SetSpaceInfo(spaceId string, details *types.Struct) error
	SetWorkspaceDashboardId(ctx session.Context, workspaceId string, id string) (setId string, err error)

	SetIsFavorite(objectId string, isFavorite, createWidget bool) error
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
	restriction  restriction.Service
}

func (s *service) Init(a *app.App) error {
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	s.resolver = app.MustComponent[idresolver.Resolver](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.store = app.MustComponent[objectstore.ObjectStore](a)
	s.restriction = app.MustComponent[restriction.Service](a)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) SetDetails(ctx session.Context, objectId string, details []*model.Detail) (err error) {
	return cache.Do(s.objectGetter, objectId, func(b basic.DetailsSettable) error {
		return b.SetDetails(ctx, details, true)
	})
}

func (s *service) SetDetailsAndUpdateLastUsed(ctx session.Context, objectId string, details []*model.Detail) (err error) {
	return cache.Do(s.objectGetter, objectId, func(b basic.DetailsSettable) error {
		return b.SetDetailsAndUpdateLastUsed(ctx, details, true)
	})
}

func (s *service) SetDetailsList(ctx session.Context, objectIds []string, details []*model.Detail) (err error) {
	var (
		resultError error
		anySucceed  bool
	)
	for i, objectId := range objectIds {
		setDetailsFunc := s.SetDetails
		if i == 0 {
			setDetailsFunc = s.SetDetailsAndUpdateLastUsed
		}
		err := setDetailsFunc(ctx, objectId, details)
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
func (s *service) ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) {
	return cache.Do(s.objectGetter, objectId, func(du basic.DetailsUpdatable) error {
		return du.UpdateDetails(modifier)
	})
}

func (s *service) ModifyDetailsAndUpdateLastUsed(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) {
	return cache.Do(s.objectGetter, objectId, func(du basic.DetailsUpdatable) error {
		return du.UpdateDetailsAndLastUsed(modifier)
	})
}

func (s *service) ModifyDetailsList(req *pb.RpcObjectListModifyDetailValuesRequest) (resultError error) {
	var anySucceed bool
	for i, objectId := range req.ObjectIds {
		modifyDetailsFunc := s.ModifyDetails
		if i == 0 {
			modifyDetailsFunc = s.ModifyDetailsAndUpdateLastUsed
		}
		err := modifyDetailsFunc(objectId, func(current *types.Struct) (*types.Struct, error) {
			for _, op := range req.Operations {
				if !pbtypes.IsEmptyValue(op.Set) {
					// Set operation has higher priority than Add and Remove, because it modifies full value
					current.Fields[op.RelationKey] = op.Set
					continue
				}
				addValueToListDetail(current, op.RelationKey, op.Add)
				removeValueFromListDetail(current, op.RelationKey, op.Remove)
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
func addValueToListDetail(s *types.Struct, key string, v *types.Value) {
	if pbtypes.IsStructEmpty(s) || v == nil {
		return
	}
	toAdd := pbtypes.GetList(v)
	oldValues := pbtypes.GetValueList(s, key)
	newValues := slice.MergeUniqBy(oldValues, toAdd, func(this *types.Value, that *types.Value) bool {
		return this.Equal(that)
	})
	s.Fields[key] = &types.Value{
		Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: newValues}},
	}
}

// removeValueFromListDetail removes values from int lists and string lists
func removeValueFromListDetail(s *types.Struct, key string, v *types.Value) {
	if pbtypes.IsStructEmpty(s) || v == nil {
		return
	}
	value := pbtypes.Get(s, key)
	if value == nil {
		return
	}
	if value.Equal(v) {
		delete(s.Fields, key)
		return
	}
	oldValues := pbtypes.GetList(value)
	if len(oldValues) == 0 {
		return
	}
	toDelete := pbtypes.GetList(v)
	newValues := lo.Filter(oldValues, func(oldValue *types.Value, _ int) bool {
		return !slices.ContainsFunc(toDelete, func(valueToDelete *types.Value) bool {
			return oldValue.Equal(valueToDelete)
		})
	})
	s.Fields[key] = &types.Value{
		Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: newValues}},
	}
}
