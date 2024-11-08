package detailservice

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var ErrBundledTypeIsReadonly = fmt.Errorf("can't modify bundled object type")

func (s *service) ObjectTypeAddRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error {
	if strings.HasPrefix(objectTypeId, bundle.TypePrefix) {
		return ErrBundledTypeIsReadonly
	}
	return cache.Do(s.objectGetter, objectTypeId, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		list := pbtypes.GetStringList(st.Details(), bundle.RelationKeyRecommendedRelations.String())
		for _, relKey := range relationKeys {
			relId, err := b.Space().GetRelationIdByKey(ctx, relKey)
			if err != nil {
				return err
			}
			if !slices.Contains(list, relId) {
				list = append(list, relId)
			}
		}
		st.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedRelations, pbtypes.StringList(list))
		return b.Apply(st)
	})
}

func (s *service) ObjectTypeRemoveRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error {
	if strings.HasPrefix(objectTypeId, bundle.TypePrefix) {
		return ErrBundledTypeIsReadonly
	}
	return cache.Do(s.objectGetter, objectTypeId, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		list := pbtypes.GetStringList(st.Details(), bundle.RelationKeyRecommendedRelations.String())
		for _, relKey := range relationKeys {
			relId, err := b.Space().GetRelationIdByKey(ctx, relKey)
			if err != nil {
				return fmt.Errorf("get relation id by key %s: %w", relKey, err)
			}
			list = slice.RemoveMut(list, relId)
		}
		st.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedRelations, pbtypes.StringList(list))
		return b.Apply(st)
	})
}

func (s *service) ListRelationsWithValue(spaceId string, value *types.Value) ([]*pb.RpcRelationListWithValueResponseResponseItem, error) {
	countersByKeys := make(map[string]int64)
	detailHandlesValue := generateFilter(value)

	err := s.store.SpaceIndex(spaceId).QueryIterate(database.Query{Filters: nil}, func(details *types.Struct) {
		for key, valueToCheck := range details.Fields {
			if detailHandlesValue(valueToCheck) {
				if counter, ok := countersByKeys[key]; ok {
					countersByKeys[key] = counter + 1
				} else {
					countersByKeys[key] = 1
				}
			}
		}
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query objects: %w", err)
	}

	keys := maps.Keys(countersByKeys)
	slices.Sort(keys)
	list := make([]*pb.RpcRelationListWithValueResponseResponseItem, len(keys))

	for i, key := range keys {
		list[i] = &pb.RpcRelationListWithValueResponseResponseItem{
			RelationKey: key,
			Counter:     countersByKeys[key],
		}
	}

	return list, nil
}

func generateFilter(value *types.Value) func(v *types.Value) bool {
	equalOrHasFilter := func(v *types.Value) bool {
		if list := v.GetListValue(); list != nil {
			for _, element := range list.Values {
				if element.Equal(value) {
					return true
				}
			}
		}
		return v.Equal(value)
	}

	stringValue := value.GetStringValue()
	if stringValue == "" {
		return equalOrHasFilter
	}

	sbt, err := typeprovider.SmartblockTypeFromID(stringValue)
	if err != nil {
		log.Error("failed to determine smartblock type", zap.Error(err))
	}

	if sbt != coresb.SmartBlockTypeDate {
		return equalOrHasFilter
	}

	start, err := dateutil.ParseDateId(stringValue)
	if err != nil {
		log.Error("failed to convert date id to day start", zap.Error(err))
		return equalOrHasFilter
	}

	end := start.Add(24 * time.Hour)
	startTimestamp := start.Unix()
	endTimestamp := end.Unix()

	return func(v *types.Value) bool {
		numberValue := int64(v.GetNumberValue())
		if numberValue >= startTimestamp && numberValue < endTimestamp {
			return true
		}
		return equalOrHasFilter(v)
	}
}
