package converter

import (
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

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

func (c LayoutConverter) Convert(st *state.State, fromLayout, toLayout model.ObjectTypeLayout) error {
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

func (c LayoutConverter) fromAnyToAny(st *state.State) error {
	template.InitTemplate(st,
		template.WithTitle,
		template.WithDescription,
	)
	return nil
}

func (c LayoutConverter) fromAnyToBookmark(st *state.State) error {
	template.InitTemplate(st,
		template.WithTitle,
		template.WithDescription,
		template.WithBookmarkBlocks,
	)
	return nil
}

func (c LayoutConverter) fromAnyToTodo(st *state.State) error {
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
	if err := c.fromAnyToSet(st); err != nil {
		return err
	}
	return nil
}

func (c *LayoutConverter) fromAnyToSet(st *state.State) error {
	source := pbtypes.GetStringList(st.Details(), bundle.RelationKeySetOf.String())
	if len(source) == 0 {
		return fmt.Errorf("source detail is not set")
	}

	addFeaturedRelationSetOf(st)

	dvBlock, _, err := dataview.DataviewBlockBySource(c.sbtProvider, c.objectStore, source)
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
	st.StoreSlice(template.CollectionStoreKey, ids)
	return nil
}

func (c *LayoutConverter) listIDsFromSet(typesFromSet []string) ([]string, error) {
	recs, _, qErr := c.objectStore.Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(typesFromSet),
			},
		},
	})
	if qErr != nil {
		return nil, fmt.Errorf("can't get records for collection: %w", qErr)
	}
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, pbtypes.GetString(r.Details, bundle.RelationKeyId.String()))
	}
	return ids, nil
}

func (c *LayoutConverter) fromNoteToCollection(st *state.State) error {
	if err := c.fromNoteToAny(st); err != nil {
		return err
	}

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
	name, ok := st.Details().Fields[bundle.RelationKeyName.String()]
	if ok && name.GetStringValue() != "" {
		newBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: name.GetStringValue()},
			},
		})
		st.Add(newBlock)

		if err := st.InsertTo(template.HeaderLayoutId, model.Block_Bottom, newBlock.Model().Id); err != nil {
			return err
		}

		st.RemoveDetail(bundle.RelationKeyName.String())
	}

	template.InitTemplate(st,
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
