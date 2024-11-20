package detailservice

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

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
		list := st.Details().GetStringList(bundle.RelationKeyRecommendedRelations)
		for _, relKey := range relationKeys {
			relId, err := b.Space().GetRelationIdByKey(ctx, relKey)
			if err != nil {
				return err
			}
			if !slices.Contains(list, relId) {
				list = append(list, relId)
			}
		}
		st.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedRelations, domain.StringList(list))
		return b.Apply(st)
	})
}

func (s *service) ObjectTypeRemoveRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error {
	if strings.HasPrefix(objectTypeId, bundle.TypePrefix) {
		return ErrBundledTypeIsReadonly
	}
	return cache.Do(s.objectGetter, objectTypeId, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		list := st.Details().GetStringList(bundle.RelationKeyRecommendedRelations)
		for _, relKey := range relationKeys {
			relId, err := b.Space().GetRelationIdByKey(ctx, relKey)
			if err != nil {
				return fmt.Errorf("get relation id by key %s: %w", relKey, err)
			}
			list = slice.RemoveMut(list, relId)
		}
		st.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedRelations, domain.StringList(list))
		return b.Apply(st)
	})
}

func (s *service) ListRelationsWithValue(spaceId string, value domain.Value) ([]*pb.RpcRelationListWithValueResponseResponseItem, error) {
	countersByKeys := make(map[domain.RelationKey]int64)
	detailHandlesValue := generateFilter(value)

	err := s.store.SpaceIndex(spaceId).QueryIterate(
		database.Query{Filters: nil},
		func(details *domain.Details) {
			for key, valueToCheck := range details.Iterate() {
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

func generateFilter(value domain.Value) func(v domain.Value) bool {
	equalOrHasFilter := func(v domain.Value) bool {
		if list, ok := v.TryListValues(); ok {
			for _, element := range list {
				if element.Equal(value) {
					return true
				}
			}
		}
		return v.Equal(value)
	}

	stringValue := value.String()
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

	ts, err := dateutil.ParseDateId(stringValue)
	if err != nil {
		log.Error("failed to parse Date object id", zap.Error(err))
		return equalOrHasFilter
	}

	shortId := dateutil.TimeToDateId(ts)

	start := ts.Truncate(24 * time.Hour)
	end := start.Add(24 * time.Hour)
	startTimestamp := start.Unix()
	endTimestamp := end.Unix()

	// filter for date objects is able to find relations with values between the borders of queried day
	// - for relations with number format it checks timestamp value is between timestamps of this day midnights
	// - for relations carrying string list it checks if some of the strings has day prefix, e.g.
	// if _date_2023-12-12-08-30-50 is queried, then all relations with prefix _date_2023-12-12 will be returned
	return func(v domain.Value) bool {
		numberValue := v.Int64()
		if numberValue >= startTimestamp && numberValue < endTimestamp {
			return true
		}

		if list := v.GetListValue(); list != nil {
			for _, element := range list.Values {
				if element.Equal(value) {
					return true
				}
				if strings.HasPrefix(element.GetStringValue(), shortId) {
					return true
				}
			}
		}
		return v.Equal(value)
	}
}
