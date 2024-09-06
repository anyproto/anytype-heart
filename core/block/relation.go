package block

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type filtersGetter func(spaceId string, rel *model.Relation) ([]*model.BlockContentDataviewFilter, error)

var ErrBundledTypeIsReadonly = fmt.Errorf("can't modify bundled object type")

func (s *Service) ObjectTypeRelationAdd(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error {
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

func (s *Service) ObjectTypeRemoveRelations(ctx context.Context, objectTypeId string, relationKeys []domain.RelationKey) error {
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

func (s *Service) ListRelationsWithValue(spaceId string, value *types.Value) (keys []string, counters []int64, err error) {
	var (
		allRelations   relationutils.Relations
		getFilters     func(spaceId string, rel *model.Relation) ([]*model.BlockContentDataviewFilter, error)
		countersByKeys = make(map[string]int64)
	)

	allRelations, err = s.objectStore.ListAllRelations(spaceId)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list all relations: %w", err)
	}

	getFilters, err = generateFiltersGetter(value)
	if err != nil {
		return nil, nil, err
	}

	for _, rel := range allRelations {
		filters, err := getFilters(spaceId, rel.Relation)
		if err != nil {
			continue
		}
		records, err := s.objectStore.Query(database.Query{
			Filters: filters,
		})
		if err != nil {
			log.Errorf("failed to query objects: %v", err)
			continue
		}
		if len(records) != 0 {
			countersByKeys[rel.Key] = int64(len(records))
		}
	}

	keys = maps.Keys(countersByKeys)
	slices.Sort(keys)

	for _, key := range keys {
		counters = append(counters, countersByKeys[key])
	}

	return keys, counters, nil
}

func generateFiltersGetter(value *types.Value) (filtersGetter, error) {
	stringFormats := []model.RelationFormat{
		model.RelationFormat_longtext,
		model.RelationFormat_shorttext,
		model.RelationFormat_status,
		model.RelationFormat_tag,
		model.RelationFormat_object,
	}

	switch t := value.Kind.(type) {
	case *types.Value_StringValue:
		sbt, err := typeprovider.SmartblockTypeFromID(t.StringValue)
		if err != nil {
			log.Errorf("failed to determine smartblock type: %v", err)
		}
		if sbt != coresb.SmartBlockTypeDate {
			return func(spaceId string, rel *model.Relation) ([]*model.BlockContentDataviewFilter, error) {
				if !slices.Contains(stringFormats, rel.Format) {
					return nil, fmt.Errorf("unsupported format %q", rel.Format)
				}
				return simpleFilters(spaceId, rel.Key, value), nil
			}, nil
		}

		start, err := addr.DateIDToDayStart(t.StringValue)
		if err != nil {
			return nil, fmt.Errorf("failed to convert date id to day start: %w", err)
		}
		end := start.Add(24 * time.Hour)

		return func(spaceId string, rel *model.Relation) ([]*model.BlockContentDataviewFilter, error) {
			switch rel.Format {
			case model.RelationFormat_date:
				return dateRangeFilters(spaceId, rel.Key, start, end), nil
			case model.RelationFormat_longtext,
				model.RelationFormat_shorttext,
				model.RelationFormat_object:
				return simpleFilters(spaceId, rel.Key, value), nil
			}
			return nil, fmt.Errorf("unsupported format %q", rel.Format)
		}, nil
	case *types.Value_NumberValue:
		return func(spaceId string, rel *model.Relation) ([]*model.BlockContentDataviewFilter, error) {
			if rel.Format != model.RelationFormat_number {
				return nil, fmt.Errorf("unsupported format %q", rel.Format)
			}
			return simpleFilters(spaceId, rel.Key, value), nil
		}, nil
	case *types.Value_BoolValue:
		return func(spaceId string, rel *model.Relation) ([]*model.BlockContentDataviewFilter, error) {
			if rel.Format != model.RelationFormat_checkbox {
				return nil, fmt.Errorf("unsupported format %q", rel.Format)
			}
			return simpleFilters(spaceId, rel.Key, value), nil
		}, nil
	case *types.Value_ListValue:
		if pbtypes.GetStringListValue(value) != nil {
			return func(spaceId string, rel *model.Relation) ([]*model.BlockContentDataviewFilter, error) {
				if !slices.Contains(stringFormats, rel.Format) {
					return nil, fmt.Errorf("unsupported format %q", rel.Format)
				}
				return listFilters(spaceId, rel.Key, value), nil
			}, nil
		}

		if pbtypes.GetIntListValue(value) != nil {
			return func(spaceId string, rel *model.Relation) ([]*model.BlockContentDataviewFilter, error) {
				if rel.Format != model.RelationFormat_number {
					return nil, fmt.Errorf("unsupported format %q", rel.Format)
				}
				return listFilters(spaceId, rel.Key, value), nil
			}, nil
		}

		return nil, fmt.Errorf("unsupported list value type: %T", t.ListValue.Values[0])
	default:
		return nil, fmt.Errorf("unsupported value type: %T", value)
	}
}

func simpleFilters(spaceId, key string, value *types.Value) []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeySpaceId.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(spaceId),
		},
		{
			RelationKey: key,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       value,
		},
	}
}

func listFilters(spaceId, key string, value *types.Value) []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeySpaceId.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(spaceId),
		},
		{
			RelationKey: key,
			Condition:   model.BlockContentDataviewFilter_AllIn,
			Value:       value,
		},
	}
}

func dateRangeFilters(spaceId, key string, start, end time.Time) []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeySpaceId.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(spaceId),
		},
		{
			RelationKey: key,
			Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
			Value:       pbtypes.Int64(start.Unix()),
		},
		{
			RelationKey: key,
			Condition:   model.BlockContentDataviewFilter_Less,
			Value:       pbtypes.Int64(end.Unix()),
		},
	}
}
