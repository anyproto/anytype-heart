package detailservice

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/slice"
)

var (
	ErrBundledTypeIsReadonly = fmt.Errorf("can't modify bundled object type")

	layoutDetailKeys = []domain.RelationKey{
		bundle.RelationKeyRecommendedLayout,
		bundle.RelationKeyLayoutAlign,
		bundle.RelationKeyRecommendedFeaturedRelations,
		bundle.RelationKeyForceLayoutFromType,
	}
)

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

func (s *service) ObjectTypeListConflictingRelations(spaceId, typeObjectId string) ([]string, error) {
	records, err := s.store.SpaceIndex(spaceId).QueryByIds([]string{typeObjectId})
	if err != nil {
		return nil, fmt.Errorf("failed to query object type: %w", err)
	}

	if len(records) != 1 {
		return nil, fmt.Errorf("failed to query object type, expected 1 record")
	}

	details := records[0].Details
	allRecommendedRelations := lo.Uniq(slices.Concat(
		details.GetStringList(bundle.RelationKeyRecommendedRelations),
		details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations),
		details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations),
		details.GetStringList(bundle.RelationKeyRecommendedFileRelations),
	))

	allRelationKeys := make([]string, 0, len(allRecommendedRelations))
	err = s.store.SpaceIndex(spaceId).QueryIterate(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(typeObjectId),
		},
	}}, func(details *domain.Details) {
		for _, key := range details.Keys() {
			if !slices.Contains(allRelationKeys, string(key)) {
				allRelationKeys = append(allRelationKeys, string(key))
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate over all objects to collect their relations: %w", err)
	}

	records, err = s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(allRelationKeys),
			},
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch relations by keys: %w", err)
	}

	conflictingRelations := make([]string, 0, len(records))
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		if !slices.Contains(allRecommendedRelations, id) {
			conflictingRelations = append(conflictingRelations, id)
		}
	}

	return conflictingRelations, nil
}

type relationIdDeriver struct {
	space smartblock.Space
	cache map[domain.RelationKey]string
}

func (d *relationIdDeriver) deriveId(key domain.RelationKey) (string, error) {
	if d.cache != nil {
		if id, found := d.cache[key]; found {
			return id, nil
		}
	}

	id, err := d.space.DeriveObjectID(context.Background(), domain.MustUniqueKey(coresb.SmartBlockTypeRelation, key.String()))
	if err != nil {
		return "", fmt.Errorf("failed to derive relation id: %w", err)
	}

	if d.cache == nil {
		d.cache = map[domain.RelationKey]string{}
	}
	d.cache[key] = id
	return id, nil
}

func (s *service) syncLayoutForObjectsAndTemplates(typeId string, oldDetails *domain.Details, newDetailsList []domain.Detail) error {
	newDetails := getNewDetailsForLayoutSync(newDetailsList)
	newLayout, isNewLayoutSet := newDetails.TryInt64(bundle.RelationKeyRecommendedLayout)
	if isNewLayoutSet && !isLayoutChangeApplicable(newLayout) {
		// if layout change is not applicable, then it is init of some system type. Objects' layout should not be modified
		newDetails.Delete(bundle.RelationKeyRecommendedLayout)
	}

	if newDetails.Len() == 0 {
		// layout details were not changed
		return nil
	}

	spaceId, err := s.resolver.ResolveSpaceID(typeId)
	if err != nil {
		return fmt.Errorf("failed to resolve space: %w", err)
	}

	spc, err := s.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return fmt.Errorf("failed to get space: %w", err)
	}

	var (
		resultErr error
		deriver   = relationIdDeriver{space: spc}
		index     = s.store.SpaceIndex(spc.Id())

		newForceLayout, isNewForceLayoutSet = newDetails.TryBool(bundle.RelationKeyForceLayoutFromType)

		forceLayoutUpdate = newForceLayout || // forceLayout is set to true
			oldDetails.GetBool(bundle.RelationKeyForceLayoutFromType) && !isNewForceLayoutSet // forceLayout was true and is not unset

		isConvertFromNote = oldDetails.GetInt64(bundle.RelationKeyRecommendedLayout) == int64(model.ObjectType_note) &&
			newDetails.GetInt64(bundle.RelationKeyRecommendedLayout) != int64(model.ObjectType_note)
	)

	records, err := s.queryObjectsAndTemplates(typeId, index)
	if err != nil {
		return err
	}

	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}

		changes := collectRelationsChanges(record.Details, oldDetails, newDetails, deriver)
		if len(changes.relationsToRemove) > 0 || changes.isFeaturedRelationsChanged {
			// we should modify not local relations from object, that's why we apply changes even if object is not in cache
			err = spc.Do(id, func(b smartblock.SmartBlock) error {
				st := b.NewState()
				st.RemoveDetail(changes.relationsToRemove...)
				if changes.isFeaturedRelationsChanged {
					st.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(changes.newFeaturedRelations))
				}
				return b.Apply(st)
			})
			if err != nil {
				resultErr = errors.Join(resultErr, err)
			}
			continue
		}

		if !forceLayoutUpdate && (changes.isLayoutFound || !isNewLayoutSet || record.Details.GetInt64(bundle.RelationKeyResolvedLayout) == newLayout) {
			// layout detail remains in object or recommendedLayout was not changed or relevant layout is already set, skipping
			continue
		}

		if err = s.updateResolvedLayout(id, newLayout, spc, index, isConvertFromNote); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}

	if resultErr != nil {
		return fmt.Errorf("failed to change layout details for objects: %w", resultErr)
	}
	return nil
}

func getNewDetailsForLayoutSync(details []domain.Detail) *domain.Details {
	det := domain.NewDetails()
	for _, detail := range details {
		if slices.Contains(layoutDetailKeys, detail.Key) {
			det.Set(detail.Key, detail.Value)
		}
	}
	return det
}

func isLayoutChangeApplicable(layout int64) bool {
	return slices.Contains([]model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_todo,
		model.ObjectType_profile,
		model.ObjectType_note,
		model.ObjectType_collection,
	}, model.ObjectTypeLayout(layout)) // nolint:gosec
}

func (s *service) queryObjectsAndTemplates(typeId string, index spaceindex.Store) ([]database.Record, error) {
	records, err := index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(typeId),
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("failed to get objects of single type: %w", err)
	}

	templates, err := index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyTargetObjectType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(typeId),
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("failed to get templates with this target type: %w", err)
	}

	return append(records, templates...), nil
}

func (s *service) updateResolvedLayout(id string, layout int64, spc clientspace.Space, index spaceindex.Store, addName bool) error {
	err := spc.DoLockedIfNotExists(id, func() error {
		return index.ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
			if details == nil {
				return nil, false, nil
			}
			if details.GetInt64(bundle.RelationKeyResolvedLayout) == layout {
				return nil, false, nil
			}
			if addName {
				snippet := details.GetString(bundle.RelationKeySnippet)
				cutSnippet, _, _ := strings.Cut(snippet, "\n")
				details.SetString(bundle.RelationKeyName, cutSnippet)
			}
			details.Set(bundle.RelationKeyResolvedLayout, domain.Int64(layout))
			return details, true, nil
		})
	})

	if err == nil {
		return nil
	}

	if !errors.Is(err, ocache.ErrExists) {
		return err
	}

	return spc.Do(id, func(b smartblock.SmartBlock) error {
		if cr, ok := b.(source.ChangeReceiver); ok && !addName {
			// we can do StateAppend here, so resolvedLayout will be injected automatically
			return cr.StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error) {
				return d.NewState(), nil, nil
			})
		}
		// we need to call Apply to generate and push changes on Title and Name addition
		return b.Apply(b.NewState(), smartblock.KeepInternalFlags)
	})
}

type layoutRelationsChanges struct {
	relationsToRemove          []domain.RelationKey
	isLayoutFound              bool
	isFeaturedRelationsChanged bool
	newFeaturedRelations       []string
}

func collectRelationsChanges(details, oldTypeDetails, newTypeDetails *domain.Details, deriver relationIdDeriver) (changes layoutRelationsChanges) {
	changes.relationsToRemove = make([]domain.RelationKey, 0, 2)
	if newLayout, ok := newTypeDetails.TryInt64(bundle.RelationKeyRecommendedLayout); ok {
		layout, found := details.TryInt64(bundle.RelationKeyLayout)
		if found {
			changes.isLayoutFound = true
			if layout == newLayout || layout == oldTypeDetails.GetInt64(bundle.RelationKeyRecommendedLayout) {
				changes.relationsToRemove = append(changes.relationsToRemove, bundle.RelationKeyLayout)
			}
		}
	}

	if newLayoutAlign, ok := newTypeDetails.TryInt64(bundle.RelationKeyRecommendedLayout); ok {
		layoutAlign, found := details.TryInt64(bundle.RelationKeyLayoutAlign)
		if found && (layoutAlign == newLayoutAlign || layoutAlign == oldTypeDetails.GetInt64(bundle.RelationKeyLayoutAlign)) {
			changes.relationsToRemove = append(changes.relationsToRemove, bundle.RelationKeyLayoutAlign)
		}
	}

	if newFR, ok := newTypeDetails.TryStringList(bundle.RelationKeyRecommendedFeaturedRelations); ok {
		oldFR := oldTypeDetails.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations)
		featuredRelations, found := details.TryStringList(bundle.RelationKeyFeaturedRelations)
		if found && isFeaturedRelationsCorrespondToType(featuredRelations, newFR, oldFR, deriver) {
			changes.isFeaturedRelationsChanged = true
			changes.newFeaturedRelations = []string{}
			if slices.Contains(featuredRelations, bundle.RelationKeyDescription.String()) {
				changes.newFeaturedRelations = append(changes.newFeaturedRelations, bundle.RelationKeyDescription.String())
			}
		}
	}
	return changes
}

func isFeaturedRelationsCorrespondToType(objectFR, newFR, oldFR []string, deriver relationIdDeriver) bool {
	featuredRelationIds := make([]string, 0, len(objectFR))
	for _, key := range objectFR {
		id, err := deriver.deriveId(domain.RelationKey(key))
		if err != nil {
			log.Error("failed to derive relation key", zap.String("key", key))
			return false // let us fallback to false, so featuredRelations won't be changed
		}
		featuredRelationIds = append(featuredRelationIds, id)
	}

	if slices.Equal(featuredRelationIds, newFR) {
		return true
	}

	return slices.Equal(featuredRelationIds, oldFR)
}
