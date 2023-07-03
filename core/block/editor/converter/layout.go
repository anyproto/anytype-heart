package converter

import (
	"fmt"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const DefaultSetSource = bundle.TypeKeyPage

type LayoutConverter struct {
	objectStore objectstore.ObjectStore
	sbtProvider typeprovider.SmartBlockTypeProvider
}

func NewLayoutConverter(objectStore objectstore.ObjectStore, sbtProvider typeprovider.SmartBlockTypeProvider) LayoutConverter {
	return LayoutConverter{
		objectStore: objectStore,
		sbtProvider: sbtProvider,
	}
}

func (c *LayoutConverter) Convert(st *state.State, fromLayout, toLayout model.ObjectTypeLayout) error {
	if fromLayout == toLayout {
		return nil
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

func (c *LayoutConverter) fromAnyToAny(st *state.State) error {
	template.InitTemplate(st,
		template.WithTitle,
		template.WithDescription,
	)
	return nil
}

func (c *LayoutConverter) fromAnyToBookmark(st *state.State) error {
	template.InitTemplate(st,
		template.WithTitle,
		template.WithDescription,
		template.WithBookmarkBlocks,
	)
	return nil
}

func (c *LayoutConverter) fromAnyToTodo(st *state.State) error {
	if err := st.SetAlign(model.Block_AlignLeft); err != nil {
		return err
	}
	template.InitTemplate(st,
		template.WithTitle,
		template.WithDescription,
		template.WithRelations([]bundle.RelationKey{bundle.RelationKeyDone}),
	)
	return nil
}

func (c *LayoutConverter) fromNoteToSet(st *state.State) error {
	if err := c.fromNoteToAny(st); err != nil {
		return err
	}

	template.InitTemplate(st,
		template.WithTitle,
		template.WithDescription,
	)
	err2 := c.fromAnyToSet(st)
	if err2 != nil {
		return err2
	}
	return nil
}

func (c *LayoutConverter) fromAnyToSet(st *state.State) error {
	source := pbtypes.GetStringList(st.Details(), bundle.RelationKeySetOf.String())
	if len(source) == 0 {
		source = []string{DefaultSetSource.URL()}
	}

	addFeaturedRelationSetOf(st)

	dvBlock, _, err := dataview.DataviewBlockBySource(st.SpaceID(), c.sbtProvider, c.objectStore, source)
	if err != nil {
		return err
	}
	template.InitTemplate(st, template.WithDataview(dvBlock, false))
	return nil
}

func addFeaturedRelationSetOf(st *state.State) {
	fr := pbtypes.GetStringList(st.Details(), bundle.RelationKeyFeaturedRelations.String())
	if !slices.Contains(fr, bundle.RelationKeySetOf.String()) {
		fr = append(fr, bundle.RelationKeySetOf.String())
	}
	st.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(fr))
}

func (c *LayoutConverter) fromSetToCollection(st *state.State) error {
	dvBlock := st.Get(template.DataviewBlockId)
	if dvBlock == nil {
		return fmt.Errorf("dataview block is not found")
	}
	details := st.Details()
	typesFromSet := pbtypes.GetStringList(details, bundle.RelationKeySetOf.String())

	c.removeRelationSetOf(st)

	dvBlock.Model().GetDataview().IsCollection = true

	ids, err := c.listIDsFromSet(typesFromSet)
	if err != nil {
		return err
	}
	st.UpdateStoreSlice(template.CollectionStoreKey, ids)
	return nil
}

func (c *LayoutConverter) listIDsFromSet(typesFromSet []string) ([]string, error) {
	records, _, err := c.objectStore.Query(
		nil,
		database.Query{
			Filters: generateFilters(typesFromSet),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("can't get records for collection: %w", err)
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, pbtypes.GetString(record.Details, bundle.RelationKeyId.String()))
	}
	return ids, nil
}

func (c *LayoutConverter) fromNoteToCollection(st *state.State) error {
	if err := c.fromNoteToAny(st); err != nil {
		return err
	}

	template.InitTemplate(st,
		template.WithTitle,
		template.WithDescription,
	)

	return c.fromAnyToCollection(st)
}

func (c *LayoutConverter) fromAnyToCollection(st *state.State) error {
	blockContent := template.MakeCollectionDataviewContent()
	template.InitTemplate(st, template.WithDataview(*blockContent, false))
	return nil
}

func (c *LayoutConverter) fromNoteToAny(st *state.State) error {
	name, ok := st.Details().Fields[bundle.RelationKeyName.String()]

	if !ok || name.GetStringValue() == "" {
		textBlock, err := getFirstTextBlock(st)
		if err != nil {
			return err
		}
		if textBlock == nil {
			return nil
		}
		st.SetDetail(bundle.RelationKeyName.String(), pbtypes.String(textBlock.Model().GetText().GetText()))

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

func (c *LayoutConverter) fromAnyToNote(st *state.State) error {
	template.InitTemplate(st,
		template.WithNameToFirstBlock,
		template.WithNoTitle,
		template.WithNoDescription,
	)
	return nil
}

func (c *LayoutConverter) removeRelationSetOf(st *state.State) {
	st.RemoveDetail(bundle.RelationKeySetOf.String())

	fr := pbtypes.GetStringList(st.Details(), bundle.RelationKeyFeaturedRelations.String())
	fr = slice.Remove(fr, bundle.RelationKeySetOf.String())
	st.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(fr))
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

func generateFilters(typesAndRelations []string) []*model.BlockContentDataviewFilter {
	var filters []*model.BlockContentDataviewFilter
	types, relations := separate(typesAndRelations)
	filters = appendTypesFilter(types, filters)
	filters = appendRelationFilters(relations, filters)
	return filters
}

func appendRelationFilters(rels []string, filters []*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
	if len(rels) != 0 {
		for _, rel := range rels {
			filters = append(filters, &model.BlockContentDataviewFilter{
				RelationKey: rel,
				Condition:   model.BlockContentDataviewFilter_Exists,
			})
		}
	}
	return filters
}

func appendTypesFilter(types []string, filters []*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
	if len(types) != 0 {
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyType.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       pbtypes.StringList(types),
		})
	}
	return filters
}

func separate(typesAndRels []string) (types []string, rels []string) {
	for _, id := range typesAndRels {
		if strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) {
			types = append(types, id)
		} else if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
			rels = append(rels, strings.TrimPrefix(id, addr.RelationKeyToIdPrefix))
		}
	}
	return
}
