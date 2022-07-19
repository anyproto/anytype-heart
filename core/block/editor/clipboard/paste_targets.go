package clipboard

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/table"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var pasteTargetCreator []func(b simple.Block) PasteTarget

type PasteTarget interface {
	PasteInside(targetState, clipboardState *state.State) error
}

func resolvePasteTarget(b simple.Block) PasteTarget {
	for _, c := range pasteTargetCreator {
		if res := c(b); res != nil {
			return res
		}
	}
	return nil
}

func registerPasteTarget(c func(b simple.Block) PasteTarget) {
	pasteTargetCreator = append(pasteTargetCreator, c)
}

func init() {
	registerPasteTarget(newCellTarget)
}

func newCellTarget(b simple.Block) PasteTarget {
	if _, _, err := table.ParseCellId(b.Model().Id); err == nil {
		return &cellTarget{
			Block: b.Model(),
		}
	}
	return nil
}

type cellTarget struct {
	*model.Block
}

func (c *cellTarget) PasteInside(targetState, clipboardState *state.State) error {
	b := targetState.Get(c.Id).(text.Block)

	var nonTextBlocks []simple.Block
	var textBlocks []text.Block

	textBlockIds := map[string]struct{}{}

	clipboardState.Iterate(func(b simple.Block) (isContinue bool) {
		if b.Model().Id != clipboardState.RootId() {
			tb, ok := b.(text.Block)
			if ok {
				textBlocks = append(textBlocks, tb)
				textBlockIds[b.Model().Id] = struct{}{}
			} else {
				nonTextBlocks = append(nonTextBlocks, b)
			}
		}
		return true
	})

	for _, b := range nonTextBlocks {
		b.Model().ChildrenIds = slice.Filter(b.Model().ChildrenIds, func(id string) bool {
			_, ok := textBlockIds[id]
			return !ok
		})
	}

	sep := ""
	for _, tb := range textBlocks {
		marks := tb.Model().GetText().Marks
		txt := tb.GetText()

		if err := tb.SetText(sep+txt, marks); err != nil {
			return fmt.Errorf("set text in block %s: %w", tb.Model().Id, err)
		}
		tb.SetStyle(model.BlockContentText_Paragraph)
		if err := b.Merge(tb); err != nil {
			return fmt.Errorf("merge %s into %s: %w", tb.Model().Id, b.Model().Id, err)
		}

		sep = "\n"
	}

	tblock, err := table.NewTable(targetState, c.Id)
	if err != nil {
		return fmt.Errorf("init table: %w", err)
	}
	ids := make([]string, 0, len(nonTextBlocks))
	for _, b := range nonTextBlocks {
		targetState.Add(b)
		ids = append(ids, b.Model().Id)
	}

	return targetState.InsertTo(tblock.Block().Model().Id, model.Block_Bottom, ids...)
}
