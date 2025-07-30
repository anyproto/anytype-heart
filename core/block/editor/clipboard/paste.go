package clipboard

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/table"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

const base64ImagePrefix = "data:image"
const base64Prefix = ";base64,"

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
	blockIds  []string
}

type pasteMode struct {
	removeSelection            bool
	multiRange                 bool
	singleRange                bool
	intoBlock                  bool
	intoBlockCopyStyle         bool
	intoBlockMergeWithoutStyle bool
	textBuf                    string
}

func (p *pasteCtrl) Exec(req *pb.RpcBlockPasteRequest) (err error) {
	if err = p.configure(req); err != nil {
		return
	}
	if p.mode.multiRange {
		if err = p.multiRange(); err != nil {
			return
		}
	} else if p.mode.intoBlockMergeWithoutStyle {
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

func (p *pasteCtrl) configure(req *pb.RpcBlockPasteRequest) (err error) {
	if req.SelectedTextRange != nil {
		p.selRange = *req.SelectedTextRange
	}
	p.selIds = req.SelectedBlockIds
	if req.FocusedBlockId != "" {
		p.selIds = append([]string{req.FocusedBlockId}, p.selIds...)
		p.mode.singleRange = true
		if firstSelText := p.getFirstSelectedText(); firstSelText != nil {
			p.mode.intoBlockMergeWithoutStyle = firstSelText.Model().GetText().Style == model.BlockContentText_Code ||
				table.IsTableCell(firstSelText.Model().Id)
			if p.mode.intoBlockMergeWithoutStyle {
				p.mode.textBuf = req.TextSlot
				p.mode.removeSelection = false
				return
			}
		}
	} else {
		p.mode.removeSelection = true
	}
	selRangeNotEmpty := p.selRange.From+p.selRange.To > 0
	if !req.IsPartOfBlock && selRangeNotEmpty {
		req.IsPartOfBlock = true
	}
	p.mode.multiRange = len(p.selIds) > 1
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

		selectedText := p.getFirstSelectedText()
		p.mode.intoBlockCopyStyle = !(isSpecificStyle(p.getFirstPasteText()) || isRequiredBlock(selectedText))

		if selectedText != nil && textCount == 1 && nonTextCount == 0 && req.IsPartOfBlock {
			p.mode.intoBlock = true
		} else {
			p.mode.intoBlock = selectedText != nil && selectedText.Model().GetText().Style == model.BlockContentText_Code
		}
	} else {
		p.mode.singleRange = false
	}
	return
}

func isSpecificStyle(block text.Block) bool {
	if block == nil {
		return false
	}
	return lo.Contains([]model.BlockContentTextStyle{
		model.BlockContentText_Description,
		model.BlockContentText_Title,
	}, block.Model().GetText().Style)

}

func isRequiredBlock(block text.Block) bool {
	return block != nil && state.IsRequiredBlockId(block.Model().Id)
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

	targetId := selText.Model().Id
	if secondBlock, err = selText.RangeSplit(p.selRange.From, p.selRange.To, false); err != nil {
		return
	}
	p.s.Add(secondBlock)

	if target := resolvePasteTarget(p.s.Get(targetId)); target != nil {
		return target.PasteInside(p.s, p.ps, secondBlock)
	}

	isPasteToHeader := state.IsRequiredBlockId(targetId)

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
	p.caretPos, err = firstSelText.RangeTextPaste(p.selRange.From, p.selRange.To, firstPasteText.Model(), p.mode.intoBlockCopyStyle)
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
	if lastSelText != nil && p.selRange.To > 0 && p.selRange.To < int32(textutil.UTF16RuneCountString(lastSelText.GetText())) {
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
		if state.IsRequiredBlockId(targetId) {
			targetId = template.HeaderLayoutId
		}
		targetPos = model.Block_Bottom
	}

	return p.ps.Iterate(func(b simple.Block) (isContinue bool) {
		if b.Model().Id != p.ps.RootId() {
			p.s.Add(b)
			p.blockIds = append(p.blockIds, b.Model().Id)
		} else {
			p.s.InsertTo(targetId, targetPos, b.Model().ChildrenIds...)
		}
		return true
	})
}

func (p *pasteCtrl) removeSelection() {
	for _, toRemove := range p.selIds {
		if !state.IsRequiredBlockId(toRemove) {
			p.s.Unlink(toRemove)
		}
	}
}

func (p *pasteCtrl) processFiles() (err error) {
	p.ps.Iterate(func(b simple.Block) (isContinue bool) {
		if file := b.Model().GetFile(); file != nil && file.State == model.BlockContentFile_Empty {
			if strings.HasPrefix(file.Name, base64ImagePrefix) {
				err = p.handleBase64(b, file)
				if err != nil {
					log.Errorf("error handling base64 image: %v", err)
				}
			} else {
				p.uploadArr = append(p.uploadArr, pb.RpcBlockUploadRequest{
					ContextId: p.s.RootId(),
					BlockId:   b.Model().Id,
					Url:       file.Name,
				})
			}
		}
		return true
	})
	return
}

func (p *pasteCtrl) handleBase64(b simple.Block, file *model.BlockContentFile) error {
	index := strings.Index(file.Name, base64Prefix)
	if index > 0 {
		file.Name = file.Name[index+len(base64Prefix):]
		fileContent, err := base64.StdEncoding.DecodeString(file.Name)
		if err != nil {
			return err
		}
		file.Name = "image"
		p.uploadArr = append(p.uploadArr, pb.RpcBlockUploadRequest{
			ContextId: p.s.RootId(),
			BlockId:   b.Model().Id,
			Bytes:     fileContent,
		})
		return nil
	}
	return errors.New("invalid base64 image")
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
