package clipboard

import (
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type pasteCtrl struct {
	// doc state
	s *state.State
	// paste state
	ps       *state.State
	mode     pasteMode
	selIds   []string
	selRange model.Range
}

type pasteMode struct {
	hasRange                   bool
	singleSelection            bool
	multiSelection             bool
	singleTextPaste            bool
	singleTextSelectionIsEmpty bool
}

func (p *pasteCtrl) Exec(req pb.RpcBlockPasteRequest) (err error) {

	if err = p.configure(req); err != nil {
		return
	}
	if p.mode.hasRange {
		if err = p.multiRange(); err != nil {
			return
		}
	}
	if err = p.insertUnderSelection(); err != nil {
		return
	}
	p.removeSelection()
	return
}

func (p *pasteCtrl) configure(req pb.RpcBlockPasteRequest) (err error) {
	if req.SelectedTextRange != nil {
		p.selRange = *req.SelectedTextRange
	}
	p.selIds = req.SelectedBlockIds
	if req.FocusedBlockId != "" {
		p.selIds = append([]string{req.FocusedBlockId}, p.selIds...)
	}

	return
}

func (p *pasteCtrl) getFirstSelectedText() text.Block {
	if len(p.selIds) > 0 {
		b := p.s.Get(p.selIds[0])
		if b != nil {
			tb, _ := b.(text.Block)
			return tb
		}
	}
	return nil
}

func (p *pasteCtrl) getLastSelectedText() text.Block {
	if len(p.selIds) > 1 {
		b := p.s.Get(p.selIds[len(p.selIds)-1])
		if b != nil {
			tb, _ := b.(text.Block)
			return tb
		}
	}
	return nil
}

func (p *pasteCtrl) getFirstPasteText() (tb text.Block) {
	p.ps.Iterate(func(b simple.Block) (isContinue bool) {
		if b.Model().Id != p.s.RootId() {
			tb, _ = b.(text.Block)
			return false
		}
		return true
	})
	return
}

func (p *pasteCtrl) getLastPasteText() (tb text.Block) {
	var lastBlock simple.Block
	p.ps.Iterate(func(b simple.Block) (isContinue bool) {
		lastBlock = b
		return true
	})
	tb, _ = lastBlock.(text.Block)
	return
}

func (p *pasteCtrl) multiRange() (err error) {
	// handles first and last blocks only
	var (
		firstSelText   = p.getFirstSelectedText()
		firstPasteText = p.getFirstPasteText()
		lastSelText    = p.getLastSelectedText()
		lastPasteText  = p.getLastPasteText()
	)
	if firstSelText != nil && firstSelText.GetText() == "" {
		if _, err = firstSelText.RangeSplit(p.selRange.From, p.selRange.From, false); err != nil {
			return
		}
		if firstPasteText != nil && firstPasteText.Model().GetText().Style == firstSelText.Model().GetText().Style {
			if err = firstSelText.Merge(firstPasteText); err != nil {
				return
			}
			p.ps.Unlink(firstPasteText.Model().Id)
		}
		p.selIds = p.selIds[1:]
	}
	if lastSelText != nil && p.selRange.To > 0 && p.selRange.To < int32(utf8.RuneCountInString(lastSelText.GetText())) {
		if _, err = lastSelText.RangeSplit(p.selRange.To, p.selRange.To, true); err != nil {
			return
		}
		if lastPasteText != nil && lastPasteText.Model().GetText().Style == lastSelText.Model().GetText().Style {
			if err = lastPasteText.Merge(lastSelText); err != nil {
				return
			}
		} else {
			p.selIds = p.selIds[0 : len(p.selIds)-1]
		}
	}
	return
}

func (p *pasteCtrl) insertUnderSelection() (err error) {
	var (
		targetId  string
		targetPos model.BlockPosition
	)
	if len(p.selIds) > 0 {
		targetId = p.selIds[0]
		targetPos = model.Block_Top
	}
	return p.ps.Iterate(func(b simple.Block) (isContinue bool) {
		if b.Model().Id != p.ps.RootId() {
			p.s.Add(b)
		} else {
			p.s.InsertTo(targetId, targetPos, b.Model().ChildrenIds...)
		}
		return true
	})
}

func (p *pasteCtrl) removeSelection() {
	for _, toRemove := range p.selIds {
		p.s.Unlink(toRemove)
	}
}
