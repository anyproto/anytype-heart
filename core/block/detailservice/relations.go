package detailservice

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/slice"
	timeutil "github.com/anyproto/anytype-heart/util/time"
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

func (s *service) ObjectTypeSetRelations(objectTypeId string, relationObjectIds []string) error {
	return s.objectTypeSetRelations(objectTypeId, relationObjectIds, false)
}

func (s *service) ObjectTypeSetFeaturedRelations(objectTypeId string, relationObjectIds []string) error {
	return s.objectTypeSetRelations(objectTypeId, relationObjectIds, true)
}

func (s *service) objectTypeSetRelations(
	objectTypeId string, relationList []string, isFeatured bool,
) error {
	if strings.HasPrefix(objectTypeId, bundle.TypePrefix) {
		return ErrBundledTypeIsReadonly
	}
	relationToSet := bundle.RelationKeyRecommendedRelations
	if isFeatured {
		relationToSet = bundle.RelationKeyRecommendedFeaturedRelations
	}
	return cache.Do(s.objectGetter, objectTypeId, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		st.SetDetailAndBundledRelation(relationToSet, domain.StringList(relationList))
		return b.Apply(st)
	})
}

func (s *service) ObjectTypeSetLayout(objectTypeId string, layout int64) error {
	if strings.HasPrefix(objectTypeId, bundle.TypePrefix) {
		return ErrBundledTypeIsReadonly
	}

	// 1. set layout to object type
	err := cache.Do(s.objectGetter, objectTypeId, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedLayout, domain.Int64(layout))
		return b.Apply(st)
	})
	if err != nil {
		return fmt.Errorf("failed to set recommended layout: %w", err)
	}

	spaceId, err := s.resolver.ResolveSpaceID(objectTypeId)
	if err != nil {
		return fmt.Errorf("failed to resolve space: %w", err)
	}

	// object types are not cross-space
	index := s.store.SpaceIndex(spaceId)
	records, err := index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(objectTypeId),
		},
	}})
	if err != nil {
		return fmt.Errorf("failed to get objects of single type: %w", err)
	}

	spc, err := s.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return fmt.Errorf("failed to get space: %w", err)
	}

	var resultErr error
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}
		if record.Details.Has(bundle.RelationKeyLayout) {
			// we should delete layout from object, that's why we apply changes even if object is not in cache
			err = cache.Do(s.objectGetter, id, func(b smartblock.SmartBlock) error {
				st := b.NewState()
				st.RemoveDetail(bundle.RelationKeyLayout)
				st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(layout))
				return b.Apply(st)
			})
			if err != nil {
				resultErr = errors.Join(resultErr, err)
			}
			continue
		}

		err = spc.DoLockedIfNotExists(id, func() error {
			return index.ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
				if details == nil {
					return nil, false, nil
				}
				if details.GetInt64(bundle.RelationKeyResolvedLayout) == layout {
					return nil, false, nil
				}
				details.Set(bundle.RelationKeyResolvedLayout, domain.Int64(layout))
				return details, true, nil
			})
		})

		if err == nil {
			continue
		}

		if !errors.Is(err, ocache.ErrExists) {
			resultErr = errors.Join(resultErr, err)
			continue
		}

		err = spc.Do(id, func(b smartblock.SmartBlock) error {
			if cr, ok := b.(source.ChangeReceiver); ok {
				return cr.StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error) {
					st := d.NewState()
					st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(layout))
					return st, nil, nil
				})
			}
			// do no Apply. StateAppend sends the event and runs reindex
			return nil
		})
		if err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}

	return resultErr
}

func (s *service) ListRelationsWithValue(spaceId string, value domain.Value) ([]*pb.RpcRelationListWithValueResponseResponseItem, error) {
	var (
		countersByKeys     = make(map[domain.RelationKey]int64)
		detailHandlesValue = generateFilter(value)
	)

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
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == bundle.RelationKeyMentions {
			return true
		}
		if keys[j] == bundle.RelationKeyMentions {
			return false
		}
		return keys[i] < keys[j]
	})

	list := make([]*pb.RpcRelationListWithValueResponseResponseItem, len(keys))
	for i, key := range keys {
		list[i] = &pb.RpcRelationListWithValueResponseResponseItem{
			RelationKey: string(key),
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

	// date object section

	dateObject, err := dateutil.BuildDateObjectFromId(stringValue)
	if err != nil {
		log.Error("failed to parse Date object id", zap.Error(err))
		return equalOrHasFilter
	}

	start := timeutil.CutToDay(dateObject.Time())
	end := start.Add(24 * time.Hour)
	startTimestamp := start.Unix()
	endTimestamp := end.Unix()

	startDateObject := dateutil.NewDateObject(start, false)
	shortId := startDateObject.Id()

	// filter for date objects is able to find relations with values between the borders of queried day
	// - for relations with number format it checks timestamp value is between timestamps of this day midnights
	// - for relations carrying string list it checks if some of the strings has day prefix, e.g.
	// if _date_2023-12-12-08-30-50Z-0200 is queried, then all relations with prefix _date_2023-12-12 will be returned
	return func(v domain.Value) bool {
		numberValue := v.Int64()
		if numberValue >= startTimestamp && numberValue < endTimestamp {
			return true
		}

		for _, element := range v.WrapToList() {
			if element.Equal(value) {
				return true
			}
			if strings.HasPrefix(element.String(), shortId) {
				return true
			}
		}

		return v.Equal(value)
	}
}
