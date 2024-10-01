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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var ErrBundledTypeIsReadonly = fmt.Errorf("can't modify bundled object type")

func (s *service) ObjectTypeAddRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error {
	if strings.HasPrefix(objectTypeId, bundle.TypePrefix) {
		return ErrBundledTypeIsReadonly
	}
	return cache.Do(s, objectTypeId, func(b smartblock.SmartBlock) error {
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
	return cache.Do(s, objectTypeId, func(b smartblock.SmartBlock) error {
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

func (s *service) ListRelationsWithValue(spaceId string, value *types.Value) (keys []string, counters []int64, err error) {
	countersByKeys := make(map[string]int64)
	detailHandlesValue := generateFilter(value)

	err = s.store.SpaceStore(spaceId).QueryIterate(database.Query{Filters: nil}, func(details *types.Struct) {
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
		return nil, nil, fmt.Errorf("failed to query objects: %w", err)
	}

	keys = maps.Keys(countersByKeys)
	slices.Sort(keys)

	for _, key := range keys {
		counters = append(counters, countersByKeys[key])
	}

	return keys, counters, nil
}

func generateFilter(value *types.Value) func(v *types.Value) bool {
	equalFilter := func(v *types.Value) bool {
		return v.Equal(value)
	}

	stringValue := value.GetStringValue()
	if stringValue == "" {
		return equalFilter
	}

	sbt, err := typeprovider.SmartblockTypeFromID(stringValue)
	if err != nil {
		log.Error("failed to determine smartblock type", zap.Error(err))
	}

	if sbt != coresb.SmartBlockTypeDate {
		return equalFilter
	}

	start, err := dateIDToDayStart(stringValue)
	if err != nil {
		log.Error("failed to convert date id to day start", zap.Error(err))
		return equalFilter
	}

	end := start.Add(24 * time.Hour)
	startTimestamp := start.Unix()
	endTimestamp := end.Unix()

	return func(v *types.Value) bool {
		numberValue := int64(v.GetNumberValue())
		if numberValue >= startTimestamp && numberValue < endTimestamp {
			return true
		}
		return equalFilter(v)
	}
}

func dateIDToDayStart(id string) (time.Time, error) {
	if !strings.HasPrefix(id, addr.DatePrefix) {
		return time.Time{}, fmt.Errorf("invalid id: date prefix not found")
	}
	return time.Parse("2006-01-02", strings.TrimPrefix(id, addr.DatePrefix))
}
