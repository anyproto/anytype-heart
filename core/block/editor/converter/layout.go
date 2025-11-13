package converter

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/any-sync/app/logger"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/slice"
)

type LayoutConverter interface {
	Convert(st *state.State, fromLayout, toLayout model.ObjectTypeLayout, ignoreIntergroupConversion bool) error
	CheckRecommendedLayoutConversionAllowed(st *state.State, layout model.ObjectTypeLayout) error
	app.Component
}

type layoutConverter struct {
	objectStore objectstore.ObjectStore
	sbtProvider typeprovider.SmartBlockTypeProvider
}

var log = logger.NewNamed("layout.converter")

func NewLayoutConverter() LayoutConverter {
	return &layoutConverter{}
}

func (c *layoutConverter) Init(a *app.App) error {
	c.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	c.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	return nil
}

func (c *layoutConverter) Name() string {
	return "layout-converter"
}

func (c *layoutConverter) CheckRecommendedLayoutConversionAllowed(st *state.State, layout model.ObjectTypeLayout) error {
	fromLayout := st.Details().GetInt64(bundle.RelationKeyRecommendedLayout)
	if !c.isConversionAllowed(model.ObjectTypeLayout(fromLayout), layout) { //nolint:gosec
		return fmt.Errorf("can't change object type recommended layout from '%s' to '%s'",
			model.ObjectTypeLayout_name[int32(fromLayout)], model.ObjectTypeLayout_name[int32(layout)]) //nolint:gosec
	}
	return nil
}

// isConversionAllowed provides more strict check of layout conversion with introduction of primitives.
// Only conversion between page layouts (page/note/task/profile) and list layouts (set->collection) is allowed
func (c *layoutConverter) isConversionAllowed(from, to model.ObjectTypeLayout) bool {
	if from == to {
		return true
	}
	if isPageLayout(from) && isPageLayout(to) {
		return true
	}
	if from == model.ObjectType_set && to == model.ObjectType_collection {
		return true
	}
	return false
}

func isPageLayout(layout model.ObjectTypeLayout) bool {
	return slices.Contains([]model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_todo,
		model.ObjectType_note,
		model.ObjectType_profile,
		model.ObjectType_bookmark,
	}, layout)
}

func (c *layoutConverter) Convert(st *state.State, fromLayout, toLayout model.ObjectTypeLayout, ignoreIntergroupConversion bool) error {
	if fromLayout == toLayout {
		return nil
	}

	if !ignoreIntergroupConversion && !c.isConversionAllowed(fromLayout, toLayout) {
		return fmt.Errorf("layout conversion from %s to %s is not allowed", model.ObjectTypeLayout_name[int32(fromLayout)], model.ObjectTypeLayout_name[int32(toLayout)])
	}

	if fromLayout == model.ObjectType_chatDeprecated || fromLayout == model.ObjectType_chatDerived {
		return fmt.Errorf("can't convert from chat")
	}
	if toLayout == model.ObjectType_chatDeprecated || toLayout == model.ObjectType_chatDerived {
		return fmt.Errorf("can't convert to chat")
	}

	if fromLayout == model.ObjectType_note && toLayout == model.ObjectType_collection {
		return c.fromNoteToCollection(st)
	}
	if fromLayout == model.ObjectType_set && toLayout == model.ObjectType_collection {
		return c.fromSetToCollection(st)
	}
	if toLayout == model.ObjectType_collection {
		return c.fromAnyToCollection(st)
	}

	if fromLayout == model.ObjectType_note && toLayout == model.ObjectType_set {
		return c.fromNoteToSet(st)
	}
	if toLayout == model.ObjectType_set {
		return c.fromAnyToSet(st)
	}

	if toLayout == model.ObjectType_note {
		return c.fromAnyToNote(st)
	}
	if fromLayout == model.ObjectType_note {
		if err := c.fromNoteToAny(st); err != nil {
			return err
		}
	}

	if toLayout == model.ObjectType_todo {
		return c.fromAnyToTodo(st)
	}

	if toLayout == model.ObjectType_bookmark {
		return c.fromAnyToBookmark(st)
	}

	// TODO We need more granular cases (not catch-all)

	return c.fromAnyToAny(st)
}

func (c *layoutConverter) fromAnyToAny(st *state.State) error {
	template.InitTemplate(st,
		template.WithTitle,
	)
	return nil
}

func (c *layoutConverter) fromAnyToBookmark(st *state.State) error {
	template.InitTemplate(st,
		template.WithTitle,
		template.WithDescription,
		template.WithBookmarkBlocks,
	)
	return nil
}

func (c *layoutConverter) fromAnyToTodo(st *state.State) error {
	template.InitTemplate(st,
		template.WithTitle,
		template.WithRelations([]domain.RelationKey{bundle.RelationKeyDone}),
	)
	return nil
}

func (c *layoutConverter) fromNoteToSet(st *state.State) error {
	if err := c.fromNoteToAny(st); err != nil {
		return err
	}

	template.InitTemplate(st,
		template.WithTitle,
	)
	return c.fromAnyToSet(st)
}

func (c *layoutConverter) fromAnyToSet(st *state.State) (err error) {
	dvBlock, err := c.buildDataviewBlock(st)
	if err != nil {
		return err
	}
	if err = c.insertTypeLevelFieldsToDataview(dvBlock, st); err != nil {
		log.Error("failed to insert type level fields to dataview block", zap.Error(err))
	}
	template.InitTemplate(st, template.WithDataview(dvBlock, false))
	return nil
}

func (c *layoutConverter) fromSetToCollection(st *state.State) error {
	dvBlock := st.Get(template.DataviewBlockId)
	if dvBlock == nil {
		return fmt.Errorf("dataview block is not found")
	}
	details := st.Details()
	err := c.addDefaultCollectionRelationIfNotPresent(st)
	if err != nil {
		return err
	}
	setSourceIds := details.GetStringList(bundle.RelationKeySetOf)
	spaceId := st.SpaceID()

	c.removeRelationSetOf(st)

	dvBlock.Model().GetDataview().IsCollection = true

	ids, err := c.listIDsFromSet(spaceId, setSourceIds)
	if err != nil {
		return err
	}
	st.UpdateStoreSlice(template.CollectionStoreKey, ids)
	return nil
}

func (c *layoutConverter) addDefaultCollectionRelationIfNotPresent(st *state.State) error {
	relationExists := func(relations []*model.BlockContentDataviewRelation, relationKey domain.RelationKey) bool {
		return lo.ContainsBy(relations, func(item *model.BlockContentDataviewRelation) bool {
			return item.Key == relationKey.String()
		})
	}

	addRelationToView := func(view *model.BlockContentDataviewView, dv *model.BlockContentDataview, relationKey domain.RelationKey) {
		if !relationExists(view.Relations, relationKey) {
			bundleRelation := bundle.MustGetRelation(relationKey)
			view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{Key: bundleRelation.Key})
			dv.RelationLinks = append(dv.RelationLinks, &model.RelationLink{
				Key:    bundleRelation.Key,
				Format: bundleRelation.Format,
			})
		}
	}

	return st.Iterate(func(block simple.Block) (isContinue bool) {
		dataview := block.Model().GetDataview()
		if dataview == nil {
			return true
		}
		for _, view := range dataview.Views {
			for _, defaultRelation := range template.DefaultCollectionRelations() {
				addRelationToView(view, dataview, defaultRelation)
			}
		}
		return false
	})
}

func (c *layoutConverter) listIDsFromSet(spaceID string, typesFromSet []string) ([]string, error) {
	filters, err := c.generateFilters(spaceID, typesFromSet)
	if err != nil {
		return nil, fmt.Errorf("generate filters: %w", err)
	}
	if len(filters) == 0 {
		return []string{}, nil
	}

	records, err := c.objectStore.SpaceIndex(spaceID).Query(
		database.Query{
			Filters: filters,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("can't get records for collection: %w", err)
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.Details.GetString(bundle.RelationKeyId))
	}
	return ids, nil
}

func (c *layoutConverter) fromNoteToCollection(st *state.State) error {
	if err := c.fromNoteToAny(st); err != nil {
		return err
	}

	template.InitTemplate(st,
		template.WithTitle,
	)

	return c.fromAnyToCollection(st)
}

func (c *layoutConverter) fromAnyToCollection(st *state.State) error {
	blockContent := template.MakeDataviewContent(true, nil, nil, nil)
	if err := c.insertTypeLevelFieldsToDataview(blockContent, st); err != nil {
		log.Error("failed to insert type level fields to dataview block", zap.Error(err))
	}
	template.InitTemplate(st, template.WithDataview(blockContent, false))
	return nil
}

func (c *layoutConverter) fromNoteToAny(st *state.State) error {
	template.InitTemplate(st, template.WithNameFromFirstBlock)
	return nil
}

func (c *layoutConverter) fromAnyToNote(st *state.State) error {
	template.InitTemplate(st,
		template.WithNameToFirstBlock,
		template.WithNoTitle,
		template.WithNoDescription,
	)
	return nil
}

func (c *layoutConverter) buildDataviewBlock(st *state.State) (*model.BlockContentOfDataview, error) {
	sources := st.Details().GetStringList(bundle.RelationKeySetOf)
	if len(sources) == 0 {
		return template.MakeDataviewContent(false, nil, nil, nil), nil
	}

	index := c.objectStore.SpaceIndex(st.SpaceID())
	ot, err := index.GetObjectType(sources[0])
	if err == nil {
		return template.MakeDataviewContent(false, ot, nil, nil), nil
	}

	relations := make([]*model.RelationLink, 0, len(sources))
	for _, relId := range sources {
		rel, err := index.GetRelationById(relId)
		if err != nil {
			return nil, fmt.Errorf("failed to get relation %s: %w", relId, err)
		}

		relations = append(relations, (&relationutils.Relation{Relation: rel}).RelationLink())
	}

	return template.MakeDataviewContent(false, nil, relations, nil), nil
}

func (c *layoutConverter) removeRelationSetOf(st *state.State) {
	st.RemoveDetail(bundle.RelationKeySetOf)

	fr := st.Details().GetStringList(bundle.RelationKeyFeaturedRelations)
	if len(fr) == 0 {
		return
	}

	fr = slice.RemoveMut(fr, bundle.RelationKeySetOf.String())
	st.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(fr))
}

func (c *layoutConverter) generateFilters(spaceId string, typesAndRelations []string) ([]database.FilterRequest, error) {
	var filters []database.FilterRequest
	m, err := c.sbtProvider.PartitionIDsByType(spaceId, typesAndRelations)
	if err != nil {
		return nil, fmt.Errorf("partition ids by sb type: %w", err)
	}
	filters = c.appendTypesFilter(m[coresb.SmartBlockTypeObjectType], filters)
	filters, err = c.appendRelationFilters(spaceId, m[coresb.SmartBlockTypeRelation], filters)
	if err != nil {
		return nil, fmt.Errorf("append relation filters: %w", err)
	}
	return filters, nil
}

func (c *layoutConverter) appendRelationFilters(spaceId string, relationIDs []string, filters []database.FilterRequest) ([]database.FilterRequest, error) {
	for _, relationID := range relationIDs {
		relation, err := c.objectStore.SpaceIndex(spaceId).GetRelationById(relationID)
		if err != nil {
			return nil, fmt.Errorf("get relation by id %s: %w", relationID, err)
		}
		filters = append(filters, database.FilterRequest{
			RelationKey: domain.RelationKey(relation.Key),
			Condition:   model.BlockContentDataviewFilter_Exists,
		})
	}
	return filters, nil
}

func (c *layoutConverter) appendTypesFilter(types []string, filters []database.FilterRequest) []database.FilterRequest {
	if len(types) != 0 {
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList(types),
		})
	}
	return filters
}

func (c *layoutConverter) insertTypeLevelFieldsToDataview(block *model.BlockContentOfDataview, st *state.State) error {
	typeId := st.LocalDetails().GetString(bundle.RelationKeyType)
	records, err := c.objectStore.SpaceIndex(st.SpaceID()).QueryByIds([]string{typeId})
	if err != nil {
		return fmt.Errorf("failed to get type object from store: %w", err)
	}
	if len(records) != 1 {
		return fmt.Errorf("failed to get type object: expected 1 record")
	}

	rawViewType := records[0].Details.GetInt64(bundle.RelationKeyDefaultViewType)
	defaultTypeIds := records[0].Details.WrapToStringList(bundle.RelationKeyDefaultTypeId)
	var defaultTypeId string
	if len(defaultTypeIds) > 0 {
		defaultTypeId = defaultTypeIds[0]
	}

	// nolint:gosec
	viewType := model.BlockContentDataviewViewType(rawViewType)
	block.Dataview.Views[0].Type = viewType
	block.Dataview.Views[0].DefaultObjectTypeId = defaultTypeId
	insertGroupRelationKey(block, viewType)

	return nil
}

func insertGroupRelationKey(block *model.BlockContentOfDataview, viewType model.BlockContentDataviewViewType) {
	var formats map[model.RelationFormat]struct{}
	switch viewType {
	case model.BlockContentDataviewView_Kanban:
		formats = map[model.RelationFormat]struct{}{
			model.RelationFormat_status:   {},
			model.RelationFormat_tag:      {},
			model.RelationFormat_checkbox: {},
		}
	case model.BlockContentDataviewView_Calendar:
		formats = map[model.RelationFormat]struct{}{model.RelationFormat_date: {}}
	default:
		return
	}

	for _, relLink := range block.Dataview.RelationLinks {
		_, found := formats[relLink.Format]
		if !found {
			continue
		}
		relation, err := bundle.GetRelation(domain.RelationKey(relLink.Key))
		if errors.Is(err, bundle.ErrNotFound) || (relation != nil && !relation.Hidden) {
			block.Dataview.Views[0].GroupRelationKey = relLink.Key
			return
		}
	}
}
