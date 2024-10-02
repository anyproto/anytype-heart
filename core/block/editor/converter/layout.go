package converter

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/slice"
)

const DefaultSetSource = bundle.TypeKeyPage

type LayoutConverter interface {
	Convert(space smartblock.Space, st *state.State, fromLayout, toLayout model.ObjectTypeLayout) error
	app.Component
}

type layoutConverter struct {
	objectStore objectstore.ObjectStore
	sbtProvider typeprovider.SmartBlockTypeProvider
}

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

func (c *layoutConverter) Convert(space smartblock.Space, st *state.State, fromLayout, toLayout model.ObjectTypeLayout) error {
	if fromLayout == toLayout {
		return nil
	}

	if fromLayout == model.ObjectType_chat || fromLayout == model.ObjectType_chatDerived {
		return fmt.Errorf("can't convert from chat")
	}
	if toLayout == model.ObjectType_chat || toLayout == model.ObjectType_chatDerived {
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
		return c.fromNoteToSet(space, st)
	}
	if toLayout == model.ObjectType_set {
		return c.fromAnyToSet(space, st)
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
	if err := st.SetAlign(model.Block_AlignLeft); err != nil {
		return err
	}
	template.InitTemplate(st,
		template.WithTitle,
		template.WithRelations([]domain.RelationKey{bundle.RelationKeyDone}),
	)
	return nil
}

func (c *layoutConverter) fromNoteToSet(space smartblock.Space, st *state.State) error {
	if err := c.fromNoteToAny(st); err != nil {
		return err
	}

	template.InitTemplate(st,
		template.WithTitle,
	)
	err2 := c.fromAnyToSet(space, st)
	if err2 != nil {
		return err2
	}
	return nil
}

func (c *layoutConverter) fromAnyToSet(space smartblock.Space, st *state.State) error {
	source := st.Details().GetStringList(bundle.RelationKeySetOf)
	if len(source) == 0 && space != nil {
		defaultTypeID, err := space.GetTypeIdByKey(context.Background(), DefaultSetSource)
		if err != nil {
			return fmt.Errorf("get default type id: %w", err)
		}
		source = []string{defaultTypeID}
	}
	addFeaturedRelationSetOf(st)

	dvBlock, err := dataview.BlockBySource(c.objectStore, source)
	if err != nil {
		return err
	}
	template.InitTemplate(st, template.WithDataview(dvBlock, false))
	return nil
}

func addFeaturedRelationSetOf(st *state.State) {
	fr := st.Details().GetStringList(bundle.RelationKeyFeaturedRelations)
	if !slices.Contains(fr, bundle.RelationKeySetOf.String()) {
		fr = append(fr, bundle.RelationKeySetOf.String())
	}
	st.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(fr))
}

func (c *layoutConverter) fromSetToCollection(st *state.State) error {
	dvBlock := st.Get(template.DataviewBlockId)
	if dvBlock == nil {
		return fmt.Errorf("dataview block is not found")
	}
	details := st.Details()
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

func (c *layoutConverter) listIDsFromSet(spaceID string, typesFromSet []string) ([]string, error) {
	filters, err := c.generateFilters(spaceID, typesFromSet)
	if err != nil {
		return nil, fmt.Errorf("generate filters: %w", err)
	}
	if len(filters) == 0 {
		return []string{}, nil
	}

	records, err := c.objectStore.Query(
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
	blockContent := template.MakeDataviewContent(true, nil, nil)
	template.InitTemplate(st, template.WithDataview(blockContent, false))
	return nil
}

func (c *layoutConverter) fromNoteToAny(st *state.State) error {
	name, ok := st.Details().TryString(bundle.RelationKeyName)

	if !ok || name == "" {
		textBlock, err := getFirstTextBlock(st)
		if err != nil {
			return err
		}
		if textBlock == nil {
			return nil
		}
		st.SetDetail(bundle.RelationKeyName, domain.String(textBlock.Model().GetText().GetText()))

		for _, id := range textBlock.Model().ChildrenIds {
			st.Unlink(id)
		}
		err = st.InsertTo(textBlock.Model().Id, model.Block_Bottom, textBlock.Model().ChildrenIds...)
		if err != nil {
			return fmt.Errorf("insert children: %w", err)
		}
		st.Unlink(textBlock.Model().Id)
	}
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

func (c *layoutConverter) removeRelationSetOf(st *state.State) {
	st.RemoveDetail(bundle.RelationKeySetOf)

	fr := st.Details().GetStringList(bundle.RelationKeyFeaturedRelations)
	fr = slice.RemoveMut(fr, bundle.RelationKeySetOf.String())
	st.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(fr))
}

func getFirstTextBlock(st *state.State) (simple.Block, error) {
	var res simple.Block
	err := st.Iterate(func(b simple.Block) (isContinue bool) {
		if b.Model().GetText() != nil {
			res = b
			return false
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *layoutConverter) generateFilters(spaceId string, typesAndRelations []string) ([]database.FilterRequest, error) {
	var filters []database.FilterRequest
	m, err := c.sbtProvider.PartitionIDsByType(spaceId, typesAndRelations)
	if err != nil {
		return nil, fmt.Errorf("partition ids by sb type: %w", err)
	}
	filters = c.appendTypesFilter(m[coresb.SmartBlockTypeObjectType], filters)
	filters, err = c.appendRelationFilters(m[coresb.SmartBlockTypeRelation], filters)
	if err != nil {
		return nil, fmt.Errorf("append relation filters: %w", err)
	}
	return filters, nil
}

func (c *layoutConverter) appendRelationFilters(relationIDs []string, filters []database.FilterRequest) ([]database.FilterRequest, error) {
	if len(relationIDs) != 0 {
		for _, relationID := range relationIDs {
			relation, err := c.objectStore.GetRelationByID(relationID)
			if err != nil {
				return nil, fmt.Errorf("get relation by id %s: %w", relationID, err)
			}
			filters = append(filters, database.FilterRequest{
				RelationKey: domain.RelationKey(relation.Key),
				Condition:   model.BlockContentDataviewFilter_Exists,
			})
		}
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
