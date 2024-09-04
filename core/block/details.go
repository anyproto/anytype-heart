package block

import (
	"errors"
	"slices"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *Service) SetDetails(ctx session.Context, objectId string, details []*model.Detail) (err error) {
	return cache.Do(s, objectId, func(b basic.DetailsSettable) error {
		return b.SetDetails(ctx, details, true)
	})
}

func (s *Service) SetDetailsAndUpdateLastUsed(ctx session.Context, objectId string, details []*model.Detail) (err error) {
	return cache.Do(s, objectId, func(b basic.DetailsSettable) error {
		return b.SetDetailsAndUpdateLastUsed(ctx, details, true)
	})
}

func (s *Service) SetDetailsList(ctx session.Context, objectIds []string, details []*model.Detail) (err error) {
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
		log.Warnf("SetDetailsList: %v", resultError)
	}
	if anySucceed {
		return nil
	}
	return resultError
}

// ModifyDetails performs details get and update under the sb lock to make sure no modifications are done in the middle
func (s *Service) ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) {
	return cache.Do(s, objectId, func(du basic.DetailsUpdatable) error {
		return du.UpdateDetails(modifier)
	})
}

func (s *Service) ModifyDetailsAndUpdateLastUsed(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error) {
	return cache.Do(s, objectId, func(du basic.DetailsUpdatable) error {
		return du.UpdateDetailsAndLastUsed(modifier)
	})
}

func (s *Service) ModifyDetailsList(req *pb.RpcObjectListModifyDetailValuesRequest) (resultError error) {
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
		log.Warnf("ModifyDetailsList: %v", resultError)
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
