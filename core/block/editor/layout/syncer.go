package layout

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/app/ocache"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	log = logger.NewNamed("layout-syncer")

	applicableLayouts = []model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_todo,
		model.ObjectType_profile,
		model.ObjectType_note,
		model.ObjectType_collection,
	}
)

type Syncer interface {
	SyncLayoutWithType(oldLayout, newLayout LayoutState, forceUpdate, needApply, withTemplates bool) error
}

func NewSyncer(typeId string, space smartblock.Space, index spaceindex.Store) Syncer {
	return &syncer{
		typeId: typeId,
		space:  space,
		index:  index,
	}
}

type LayoutState struct {
	recommendedLayout int64
	layoutAlign       int64
	featuredRelations []string

	isRecommendedLayoutSet bool
	isLayoutAlignSet       bool
	isFeaturedRelationsSet bool
}

func (ls LayoutState) isAllSet() bool {
	return ls.isRecommendedLayoutSet && ls.isLayoutAlignSet && ls.isFeaturedRelationsSet
}

func (ls LayoutState) Copy() LayoutState {
	return LayoutState{
		recommendedLayout:      ls.recommendedLayout,
		layoutAlign:            ls.layoutAlign,
		featuredRelations:      slices.Clone(ls.featuredRelations),
		isRecommendedLayoutSet: ls.isRecommendedLayoutSet,
		isLayoutAlignSet:       ls.isLayoutAlignSet,
		isFeaturedRelationsSet: ls.isFeaturedRelationsSet,
	}
}

func NewLayoutStateFromDetails(details *domain.Details) LayoutState {
	ls := LayoutState{}
	if details == nil {
		return ls
	}
	if layout, ok := details.TryInt64(bundle.RelationKeyRecommendedLayout); ok {
		ls.recommendedLayout = layout
		ls.isRecommendedLayoutSet = true
	}
	if layoutAlign, ok := details.TryInt64(bundle.RelationKeyLayoutAlign); ok {
		ls.layoutAlign = layoutAlign
		ls.isLayoutAlignSet = true
	}
	if featuredRelations, ok := details.TryStringList(bundle.RelationKeyRecommendedFeaturedRelations); ok {
		ls.featuredRelations = featuredRelations
		ls.isFeaturedRelationsSet = true
	}
	return ls
}

func NewLayoutStateFromEvents(events []simple.EventMessage) LayoutState {
	ls := LayoutState{}
	for _, ev := range events {
		if amend := ev.Msg.GetObjectDetailsAmend(); amend != nil {
			for _, detail := range amend.Details {
				switch detail.Key {
				case bundle.RelationKeyRecommendedLayout.String():
					ls.recommendedLayout = int64(detail.Value.GetNumberValue())
					ls.isRecommendedLayoutSet = true
				case bundle.RelationKeyRecommendedFeaturedRelations.String():
					ls.featuredRelations = pbtypes.GetStringListValue(detail.Value)
					ls.isFeaturedRelationsSet = true
				case bundle.RelationKeyLayoutAlign.String():
					ls.layoutAlign = int64(detail.Value.GetNumberValue())
					ls.isLayoutAlignSet = true
				}
			}
			if ls.isAllSet() {
				return ls
			}
		}
		if detailsSet := ev.Msg.GetObjectDetailsSet(); detailsSet != nil {
			if v := detailsSet.Details.Fields[bundle.RelationKeyRecommendedLayout.String()]; v != nil {
				ls.recommendedLayout = int64(v.GetNumberValue())
				ls.isRecommendedLayoutSet = true
			}
			if v := detailsSet.Details.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()]; v != nil {
				ls.featuredRelations = pbtypes.GetStringListValue(v)
				ls.isFeaturedRelationsSet = true
			}
			if v := detailsSet.Details.Fields[bundle.RelationKeyLayoutAlign.String()]; v != nil {
				ls.layoutAlign = int64(v.GetNumberValue())
				ls.isLayoutAlignSet = true
			}
			if ls.isAllSet() {
				return ls
			}
		}
	}
	return ls
}

type syncer struct {
	space smartblock.Space
	index spaceindex.Store

	typeId string
	cache  map[domain.RelationKey]string
}

func (s *syncer) SyncLayoutWithType(oldLayout, newLayout LayoutState, forceUpdate, needApply, withTemplates bool) error {
	if newLayout.isRecommendedLayoutSet && !isLayoutChangeApplicable(newLayout.recommendedLayout) {
		// if layout change is not applicable, then it is init of some system type. Objects' layout should not be modified
		newLayout.isRecommendedLayoutSet = false
	}

	var (
		resultErr         error
		isConvertFromNote = oldLayout.recommendedLayout == int64(model.ObjectType_note) && newLayout.recommendedLayout != int64(model.ObjectType_note)
	)

	records, err := s.queryObjects(withTemplates)
	if err != nil {
		return fmt.Errorf("failed to query objects: %w", err)
	}

	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}

		changes := s.collectRelationsChanges(record.Details, newLayout, oldLayout, forceUpdate)
		if needApply && (len(changes.relationsToRemove) > 0 || changes.isFeaturedRelationsChanged) {
			// we should modify not local relations from object, that's why we apply changes even if object is not in cache
			err = s.space.Do(id, func(b smartblock.SmartBlock) error {
				st := b.NewState()
				st.RemoveDetail(changes.relationsToRemove...)
				if changes.isFeaturedRelationsChanged {
					st.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(changes.newFeaturedRelations))
				}
				st.SetChangeType(domain.LayoutSync)
				return b.Apply(st)
			})
			if err != nil {
				resultErr = errors.Join(resultErr, err)
			}
			if _, err = s.space.TryRemove(id); err != nil && !errors.Is(err, domain.ErrObjectNotFound) {
				log.Error("failed to remove object from cache", zap.String("id", id), zap.Error(err))
			}
			continue
		}

		if !forceUpdate && (changes.isLayoutFound || !newLayout.isRecommendedLayoutSet) || record.Details.GetInt64(bundle.RelationKeyResolvedLayout) == newLayout.recommendedLayout {
			// layout detail remains in object or recommendedLayout was not changed or relevant layout is already set, skipping
			continue
		}

		if err = s.updateResolvedLayout(id, newLayout.recommendedLayout, isConvertFromNote, needApply); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}

	if resultErr != nil {
		return fmt.Errorf("failed to change layout details for objects: %w", resultErr)
	}
	return nil
}

func (s *syncer) queryObjects(withTemplates bool) ([]database.Record, error) {
	records, err := s.index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(s.typeId),
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("failed to get objects of single type: %w", err)
	}

	if !withTemplates {
		return records, nil
	}

	templates, err := s.index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyTargetObjectType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(s.typeId),
		},
	}})
	if err != nil {
		return nil, fmt.Errorf("failed to get templates with this target type: %w", err)
	}

	return append(records, templates...), nil
}

func (s *syncer) updateResolvedLayout(id string, layout int64, addName, needApply bool) error {
	err := s.space.DoLockedIfNotExists(id, func() error {
		return s.index.ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
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

	if !errors.Is(err, ocache.ErrExists) {
		return err
	}

	if err == nil || !needApply {
		return nil
	}

	return s.space.Do(id, func(b smartblock.SmartBlock) error {
		if cr, ok := b.(source.ChangeReceiver); ok && !addName {
			// we can do StateAppend here, so resolvedLayout will be injected automatically
			return cr.StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error) {
				return d.NewState(), nil, nil
			})
		}
		st := b.NewState()
		st.SetChangeType(domain.LayoutSync)
		// we need to call Apply to generate and push changes on Title and Name addition
		return b.Apply(st, smartblock.KeepInternalFlags)
	})
}

type layoutRelationsChanges struct {
	relationsToRemove          []domain.RelationKey
	isLayoutFound              bool
	isFeaturedRelationsChanged bool
	newFeaturedRelations       []string
}

func (s *syncer) collectRelationsChanges(details *domain.Details, newLayout, oldLayout LayoutState, forceUpdate bool) (changes layoutRelationsChanges) {
	if forceUpdate {
		return enforcedRelationsChanges(details)
	}
	changes.relationsToRemove = make([]domain.RelationKey, 0, 2)

	if newLayout.isRecommendedLayoutSet {
		layout, found := details.TryInt64(bundle.RelationKeyLayout)
		if found {
			changes.isLayoutFound = true
			if layout == newLayout.recommendedLayout || oldLayout.isRecommendedLayoutSet && layout == oldLayout.recommendedLayout {
				changes.relationsToRemove = append(changes.relationsToRemove, bundle.RelationKeyLayout)
			}
		}
	}

	if newLayout.isLayoutAlignSet {
		layoutAlign, found := details.TryInt64(bundle.RelationKeyLayoutAlign)
		if found && (layoutAlign == newLayout.layoutAlign || oldLayout.isLayoutAlignSet && layoutAlign == oldLayout.layoutAlign) {
			changes.relationsToRemove = append(changes.relationsToRemove, bundle.RelationKeyLayoutAlign)
		}
	}

	if newLayout.isFeaturedRelationsSet {
		featuredRelations, found := details.TryStringList(bundle.RelationKeyFeaturedRelations)
		if found && s.isFeaturedRelationsCorrespondToType(featuredRelations, newLayout, oldLayout) {
			changes.isFeaturedRelationsChanged = true
			changes.newFeaturedRelations = []string{}
			if slices.Contains(featuredRelations, bundle.RelationKeyDescription.String()) {
				changes.newFeaturedRelations = append(changes.newFeaturedRelations, bundle.RelationKeyDescription.String())
			}
		}
	}
	return changes
}

func enforcedRelationsChanges(details *domain.Details) layoutRelationsChanges {
	changes := layoutRelationsChanges{
		relationsToRemove: make([]domain.RelationKey, 0, 2),
	}
	_, found := details.TryInt64(bundle.RelationKeyLayout)
	if found {
		changes.isLayoutFound = true
		changes.relationsToRemove = append(changes.relationsToRemove, bundle.RelationKeyLayout)
	}

	_, found = details.TryInt64(bundle.RelationKeyLayoutAlign)
	if found {
		changes.relationsToRemove = append(changes.relationsToRemove, bundle.RelationKeyLayoutAlign)
	}

	featuredRelations, found := details.TryStringList(bundle.RelationKeyFeaturedRelations)
	if found {
		changes.isFeaturedRelationsChanged = true
		if slices.Contains(featuredRelations, bundle.RelationKeyDescription.String()) {
			changes.newFeaturedRelations = append(changes.newFeaturedRelations, bundle.RelationKeyDescription.String())
		}
	}
	return changes
}

func (s *syncer) isFeaturedRelationsCorrespondToType(fr []string, newLayout, oldLayout LayoutState) bool {
	featuredRelationIds := make([]string, 0, len(fr))
	for _, key := range fr {
		id, err := s.deriveId(domain.RelationKey(key))
		if err != nil {
			log.Error("failed to derive relation id", zap.String("key", key), zap.Error(err))
			return true // let us fallback to false, so featuredRelations won't be changed
		}
		featuredRelationIds = append(featuredRelationIds, id)
	}

	if newLayout.isFeaturedRelationsSet && slices.Equal(featuredRelationIds, newLayout.featuredRelations) {
		return true
	}

	return oldLayout.isFeaturedRelationsSet && slices.Equal(featuredRelationIds, oldLayout.featuredRelations)
}

func (s *syncer) deriveId(key domain.RelationKey) (string, error) {
	if s.cache != nil {
		if id, found := s.cache[key]; found {
			return id, nil
		}
	}

	id, err := s.space.DeriveObjectID(context.Background(), domain.MustUniqueKey(coresb.SmartBlockTypeRelation, key.String()))
	if err != nil {
		return "", fmt.Errorf("failed to derive relation id: %w", err)
	}

	if s.cache == nil {
		s.cache = map[domain.RelationKey]string{}
	}
	s.cache[key] = id
	return id, nil
}

func isLayoutChangeApplicable(layout int64) bool {
	return slices.Contains(applicableLayouts, model.ObjectTypeLayout(layout)) // nolint:gosec
}
