package converter

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func ConvertLayout(st *state.State, fromLayout, toLayout model.ObjectTypeLayout) error {
	if toLayout == model.ObjectType_note {
		if name, ok := st.Details().Fields[bundle.RelationKeyName.String()]; ok && name.GetStringValue() != "" {
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
	}

	if fromLayout == model.ObjectType_note {
		if name, ok := st.Details().Fields[bundle.RelationKeyName.String()]; !ok || name.GetStringValue() == "" {
			textBlock, err := st.GetFirstTextBlock()
			if err != nil {
				return err
			}
			if textBlock != nil {
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
		}
	}
	return nil
}
