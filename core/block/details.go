package block

import (
	"errors"
	"slices"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *Service) SetDetails(ctx session.Context, objectId string, details []domain.Detail) (err error) {
	return cache.Do(s, objectId, func(b basic.DetailsSettable) error {
		return b.SetDetails(ctx, details, true)
	})
}

func (s *Service) SetDetailsList(ctx session.Context, objectIds []string, details []domain.Detail) (err error) {
	var (
		resultError error
		anySucceed  bool
	)
	for _, objectId := range objectIds {
		err := s.SetDetails(ctx, objectId, details)
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
func (s *Service) ModifyDetails(objectId string, modifier func(current *domain.Details) (*domain.Details, error)) (err error) {
	return cache.Do(s, objectId, func(du basic.DetailsUpdatable) error {
		return du.UpdateDetails(modifier)
	})
}

func (s *Service) ModifyDetailsList(req *pb.RpcObjectListModifyDetailValuesRequest) (resultError error) {
	var anySucceed bool
	for _, objectId := range req.ObjectIds {
		err := s.ModifyDetails(objectId, func(current *domain.Details) (*domain.Details, error) {
			for _, op := range req.Operations {
				if !pbtypes.IsEmptyValue(op.Set) {
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
		log.Warnf("ModifyDetailsList: %v", resultError)
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
	s.Set(key, domain.ValueList(newValues))
}
