package clipboard

import (
	"strings"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
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

	caretPos  int32
	uploadArr []pb.RpcBlockUploadRequest
}

type pasteMode struct {
	toTitle             bool
	removeSelection     bool
	multiRange          bool
	singleRange         bool
	intoBlock           bool
	intoBlockPasteStyle bool
	intoCodeBlock       bool
	textBuf             string
}

func (p *pasteCtrl) Exec(req pb.RpcBlockPasteRequest) (err error) {
	if err = p.configure(req); err != nil {
		return
	}
	if p.mode.multiRange {
		if err = p.multiRange(); err != nil {
			return
		}
	} else if p.mode.intoCodeBlock {
		if err = p.intoCodeBlock(); err != nil {
			return
		}
	} else if p.mode.intoBlock {
		if err = p.intoBlock(); err != nil {
			return
		}
	} else if p.mode.singleRange {
		if err = p.singleRange(); err != nil {
			return
		}
	}
	if err = p.insertUnderSelection(); err != nil {
		return
	}
	if p.mode.removeSelection {
		p.removeSelection()
	}
	p.normalize()

	p.processFiles()
	return
}

func (p *pasteCtrl) configure(req pb.RpcBlockPasteRequest) (err error) {
	if req.SelectedTextRange != nil {
		p.selRange = *req.SelectedTextRange
	}
	p.selIds = req.SelectedBlockIds
	if req.FocusedBlockId != "" {
		p.selIds = append([]string{req.FocusedBlockId}, p.selIds...)
		p.mode.singleRange = true
		if firstSelText := p.getFirstSelectedText(); firstSelText != nil {
			p.mode.intoCodeBlock = firstSelText.Model().GetText().Style == model.BlockContentText_Code
			if p.mode.intoCodeBlock {
				p.mode.textBuf = req.TextSlot
				p.mode.removeSelection = false
				return
			}
		}
	} else {
		p.mode.removeSelection = true
	}
	p.mode.multiRange = len(p.selIds) > 1 && p.selRange.From+p.selRange.To > 0
	if !p.mode.multiRange {
		var (
			textCount, nonTextCount int
		)
		if err = p.ps.Iterate(func(b simple.Block) (isContinue bool) {
			if b.Model().Id != p.ps.RootId() {
				if _, ok := b.(text.Block); ok {
					textCount++
				} else {
					nonTextCount++
				}
			}
			return true
		}); err != nil {
			return
		}
		selText := p.getFirstSelectedText()
		p.mode.toTitle = selText != nil && p.s.HasParent(selText.Model().Id, template.HeaderLayoutId)
		p.mode.intoBlockPasteStyle = p.mode.toTitle
		if selText != nil && textCount == 1 && nonTextCount == 0 {
			p.mode.intoBlock = true
			if selText.GetText() == "" {
				p.mode.intoBlockPasteStyle = true
			}
		} else {
			p.mode.intoBlock = selText != nil && selText.Model().GetText().Style == model.BlockContentText_Code
		}
	} else {
		p.mode.singleRange = false
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
		if b.Model().Id != p.ps.RootId() {
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

func (p *pasteCtrl) singleRange() (err error) {
	var (
		selText     = p.getFirstSelectedText()
		secondBlock simple.Block
	)
	if selText == nil {
		return
	}
	if secondBlock, err = selText.RangeSplit(p.selRange.From, p.selRange.To, false); err != nil {
		return
	}
	p.s.Add(secondBlock)
	targetId := selText.Model().Id
	isPasteToHeader := targetId == template.TitleBlockId || targetId == template.DescriptionBlockId
	pos := model.Block_Bottom
	if isPasteToHeader {
		targetId = template.HeaderLayoutId
	}
	if err = p.s.InsertTo(targetId, pos, secondBlock.Model().Id); err != nil {
		return
	}
	if secondBlock.Model().GetText().Text == "" {
		p.s.Unlink(secondBlock.Model().Id)
	}
	if isPasteToHeader && selText.GetText() == "" {
		firstPasteText := p.getFirstPasteText()
		if firstPasteText != nil {
			selText.SetText(firstPasteText.GetText(), nil)
			p.ps.Unlink(firstPasteText.Model().Id)
			return
		}
		return
	}
	if selText.GetText() == "" {
		p.mode.removeSelection = true
	}
	return
}

func (p *pasteCtrl) intoBlock() (err error) {
	var (
		firstSelText   = p.getFirstSelectedText()
		firstPasteText = p.getFirstPasteText()
	)
	if firstSelText == nil || firstPasteText == nil {
		return
	}
	p.caretPos, err = firstSelText.RangeTextPaste(p.selRange.From, p.selRange.To, firstPasteText.Model(), p.mode.intoBlockPasteStyle)
	p.ps.Unlink(firstPasteText.Model().Id)
	return
}

func (p *pasteCtrl) multiRange() (err error) {
	var (
		firstSelText   = p.getFirstSelectedText()
		firstPasteText = p.getFirstPasteText()
		lastSelText    = p.getLastSelectedText()
		lastPasteText  = p.getLastPasteText()
	)
	if firstSelText != nil && firstSelText.GetText() != "" {
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
		if targetId == template.TitleBlockId || targetId == template.DescriptionBlockId {
			targetId = template.HeaderLayoutId
		}
		targetPos = model.Block_Bottom
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

func (p *pasteCtrl) processFiles() {
	p.ps.Iterate(func(b simple.Block) (isContinue bool) {
		if file := b.Model().GetFile(); file != nil && file.State == model.BlockContentFile_Empty {
			p.uploadArr = append(p.uploadArr, pb.RpcBlockUploadRequest{
				BlockId: b.Model().Id,
				Url:     file.Name,
			})
		}
		return true
	})
}

func (p *pasteCtrl) normalize() {
	p.ps.Iterate(func(b simple.Block) (isContinue bool) {
		if txtBlock := b.Model().GetText(); txtBlock != nil {
			if txtBlock.Style == model.BlockContentText_Title && b.Model().Id != template.TitleBlockId {
				txtBlock.Style = model.BlockContentText_Header1
			} else if txtBlock.Style == model.BlockContentText_Description && b.Model().Id != template.DescriptionBlockId {
				txtBlock.Style = model.BlockContentText_Paragraph
			}
		}
		return true
	})
}

func (p *pasteCtrl) intoCodeBlock() (err error) {
	selText := p.getFirstSelectedText()
	var txt = p.mode.textBuf
	if txt == "" {
		var txtArr []string
		p.ps.Iterate(func(b simple.Block) (isContinue bool) {
			if tb, ok := b.(text.Block); ok {
				txtArr = append(txtArr, tb.GetText())
			}
			return true
		})
		txt = strings.Join(txtArr, "\n")
	}
	tb := &model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  txt,
				Marks: &model.BlockContentTextMarks{},
			},
		},
	}
	p.ps.Get(p.ps.RootId()).Model().ChildrenIds = nil
	p.caretPos, err = selText.RangeTextPaste(p.selRange.From, p.selRange.To, tb, true)
	return err
}
