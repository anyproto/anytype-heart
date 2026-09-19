package clipboard

import (
	"encoding/base64"
	"errors"
	"fmt"
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

	// formerly-empty focused block that got paste content written in place;
	// forked to a fresh id at the end of Exec (see forkFilledEmptyBlock)
	filledEmptyBlockId string

	// children of the dropped first pasted block, to be re-parented onto the focused
	// block that took its place once they are in the document (see adoptPastedChildren)
	adoptedChildIds []string
	adoptParentId   string
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

// Exec runs the paste operation: determines the paste mode via configure, executes the
// mode-specific handler (multiRange / intoCodeBlock / intoBlock / singleRange), then
// inserts remaining paste blocks under the selection, removes replaced blocks, normalizes
// styles, and collects file upload requests.
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
	if err = p.adoptPastedChildren(); err != nil {
		return
	}
	if p.mode.removeSelection {
		p.removeSelection()
	}
	if err = p.forkFilledEmptyBlock(); err != nil {
		return
	}
	p.normalize()
	p.processFiles()
	return
}

// forkFilledEmptyBlock re-creates a formerly-empty block that paste filled in
// place under a fresh id. An empty block is a shared LWW register: its id is
// known to all peers, so concurrent first-writes into it clobber each other.
// A fresh id turns the concurrent case into two independent creates — both
// texts survive the merge. The new id is prepended to blockIds so the client
// re-targets focus from the replaced block.
func (p *pasteCtrl) forkFilledEmptyBlock() (err error) {
	if p.filledEmptyBlockId == "" {
		return nil
	}
	old := p.s.Pick(p.filledEmptyBlockId)
	if old == nil {
		return nil
	}
	m := old.Copy().Model()
	m.Id = ""
	forked := simple.New(m)
	p.s.Add(forked)
	if err = p.s.InsertTo(p.filledEmptyBlockId, model.Block_Top, forked.Model().Id); err != nil {
		return fmt.Errorf("insert forked block: %w", err)
	}
	p.s.Unlink(p.filledEmptyBlockId)
	p.blockIds = append([]string{forked.Model().Id}, p.blockIds...)
	return nil
}

// configure determines the paste mode based on the request parameters:
//   - FocusedBlockId set → singleRange mode (cursor is inside a block). Also sets
//     IsPartOfBlock = true so that single-text-block paste routes to intoBlock.
//     If the focused block is Code or a table cell, switches to intoBlockMergeWithoutStyle.
//   - Multiple selIds → multiRange mode (several blocks selected for replacement).
//   - Single text block in paste state + IsPartOfBlock → intoBlock mode (merge inline).
//   - Otherwise stays in singleRange mode (split at cursor, insert paste blocks in between).
//
// intoBlockCopyStyle is set to true unless the paste content has Title/Description style
// or the target is a required block (title, description), in which case the target's
// style is preserved.
func (p *pasteCtrl) configure(req *pb.RpcBlockPasteRequest) (err error) {
	if req.SelectedTextRange != nil {
		p.selRange = *req.SelectedTextRange
	}
	p.selIds = req.SelectedBlockIds
	if req.FocusedBlockId != "" {
		p.selIds = append([]string{req.FocusedBlockId}, p.selIds...)
		p.mode.singleRange = true
		req.IsPartOfBlock = true
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

// isSpecificStyle returns true if the block has a Title or Description style.
// Used to prevent copying these internal styles when pasting into regular blocks.
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

// singleRange handles pasting when the cursor is inside a single block (FocusedBlockId set)
// and the paste content has multiple blocks (so intoBlock mode was not chosen).
//
// It splits the focused block's text at the selection range via RangeSplit, producing:
//   - selText: text before the range (runes[:from]) — stays in the original block
//   - secondBlock: text after the range (runes[to:]) — inserted below
//
// Paste blocks from the paste state are then inserted between them by insertUnderSelection.
//
// Special cases:
//   - Header blocks (title/description): merge first paste text into the header, return early.
//   - Originally empty blocks (wasEmpty): merge first paste text content and marks into the
//     block instead of deleting it. This preserves the block's style (bullet, toggle, etc.).
//   - Blocks that became empty after split (had text, cursor at pos 0): mark for removal
//     so the paste content replaces the block.
func (p *pasteCtrl) singleRange() (err error) {
	var (
		selText     = p.getFirstSelectedText()
		secondBlock simple.Block
	)
	if selText == nil {
		return
	}

	targetId := selText.Model().Id
	wasEmpty := selText.GetText() == ""
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
	firstPasteText := p.getFirstPasteText()
	if isPasteToHeader && selText.GetText() == "" {
		if firstPasteText != nil {
			selText.SetText(firstPasteText.GetText(), nil)
			p.ps.Unlink(firstPasteText.Model().Id)
			return
		}
		return
	}
	if selText.GetText() == "" {
		p.mode.removeSelection = true
		// Reusing the focused block silently flattens the first pasted block: only text
		// and marks are carried over, so style, checked, colors, code language and
		// children stay behind. Skip the reuse for a block that holds nothing a user put
		// there and drop it instead, so the paste blocks land untouched. removeSelection
		// unlinks it, so GO-7311's promise that no stray empty paragraph is left above
		// the pasted content still holds.
		//
		// Anything the user did set keeps the old behaviour — in particular a style of
		// its own, which is the GO-6615 "Keep target toggle block" intent.
		if wasEmpty && firstPasteText != nil && !isDisposablePlaceholder(selText) {
			p.mode.removeSelection = false
			selText.SetText(firstPasteText.GetText(), firstPasteText.Model().GetText().Marks)
			if err = p.rehomeFirstPasteChildren(selText, firstPasteText); err != nil {
				return
			}
			p.ps.Unlink(firstPasteText.Model().Id)
			// an empty block's id is shared with peers — fork it (never a
			// required block here: those returned earlier)
			p.filledEmptyBlockId = targetId
		}
		// Either way the focused block is gone by the end of Exec: dropped by
		// removeSelection, or forked to a fresh id and unlinked. A position inside it
		// addresses a block that is no longer there, and Android takes any caretPosition
		// >= 0 in preference to blockIds, so it would pin the caret to the dead id.
		// intoBlock reports the caret through blockIds the same way.
		p.caretPos = -1
	}
	return
}

// rehomeFirstPasteChildren keeps the subtree of the first pasted block alive when the block
// itself is dropped in favour of the reused focused block. State.Unlink only removes an id
// from its parent's ChildrenIds: the subtree stays in the paste state but stops being
// reachable from its root, so insertUnderSelection, which walks the tree, never moves it into
// the document and the nested content is silently lost.
//
// Re-attaching the children to the paste root in the place their parent held is enough on its
// own: they then land in the document as blocks following the reused one. When the reused
// block's style can own children they are re-parented onto it afterwards, once
// insertUnderSelection has put them in the document — see adoptPastedChildren.
func (p *pasteCtrl) rehomeFirstPasteChildren(selText, firstPasteText text.Block) error {
	childIds := append([]string(nil), firstPasteText.Model().ChildrenIds...)
	if len(childIds) == 0 {
		return nil
	}
	for _, id := range childIds {
		p.ps.Unlink(id)
	}
	if err := p.ps.InsertTo(firstPasteText.Model().Id, model.Block_Bottom, childIds...); err != nil {
		return fmt.Errorf("re-attach children of the first pasted block: %w", err)
	}
	if text.CanHaveChildren(selText.Model().GetText().GetStyle()) {
		p.adoptedChildIds = childIds
		p.adoptParentId = selText.Model().Id
	}
	return nil
}

// adoptPastedChildren re-parents the children of the dropped first pasted block onto the
// focused block that took its place, which insertUnderSelection has just left as their
// preceding sibling. A style that cannot own children keeps them there instead.
//
// They go in front of the children the focused block already had, which GO-7513 keeps alive:
// the caret sat in its text, above everything nested under it, and pasted content belongs
// where the caret was. This also keeps the pasted subtree contiguous.
func (p *pasteCtrl) adoptPastedChildren() error {
	if len(p.adoptedChildIds) == 0 {
		return nil
	}
	for _, id := range p.adoptedChildIds {
		p.s.Unlink(id)
	}
	if err := p.s.InsertTo(p.adoptParentId, model.Block_InnerFirst, p.adoptedChildIds...); err != nil {
		return fmt.Errorf("re-parent pasted children onto the reused block: %w", err)
	}
	return nil
}

// isDisposablePlaceholder reports whether a focused text block holds nothing a user put
// there, so that paste can drop it instead of reusing it for the first pasted line.
//
// text.Block.IsEmpty covers the text itself plus marks, style, checked state, both colors,
// both icon fields and both alignments. Three things it does not cover matter just as much
// here: children, because unlinking a block orphans its whole subtree and the state apply
// then deletes it; Fields, which carries a code block's language and survives a style change
// back to Paragraph; and Restrictions, because dropping the block instead of writing to it
// would slip past the restriction check that a write would have failed.
func isDisposablePlaceholder(b text.Block) bool {
	m := b.Model()
	return b.IsEmpty() &&
		len(m.ChildrenIds) == 0 &&
		len(m.Fields.GetFields()) == 0 &&
		!hasRestrictions(m.Restrictions)
}

func hasRestrictions(r *model.BlockRestrictions) bool {
	return r != nil && (r.Read || r.Edit || r.Remove || r.Drag || r.DropOn)
}

// intoBlock handles pasting a single text block into the focused block inline.
// It calls RangeTextPaste to insert the paste text at the selection range within the
// existing block, then unlinks the paste text from the paste state.
// When intoBlockCopyStyle is true and the paste replaces all text (or fills an empty
// Paragraph block), the target block's style is overwritten with the paste block's style.
func (p *pasteCtrl) intoBlock() (err error) {
	var (
		firstSelText   = p.getFirstSelectedText()
		firstPasteText = p.getFirstPasteText()
	)
	if firstSelText == nil || firstPasteText == nil {
		return
	}
	wasEmpty := firstSelText.GetText() == "" && !state.IsRequiredBlockId(firstSelText.Model().Id)
	p.caretPos, err = firstSelText.RangeTextPaste(p.selRange.From, p.selRange.To, firstPasteText.Model(), p.mode.intoBlockCopyStyle)
	p.ps.Unlink(firstPasteText.Model().Id)
	if err == nil && wasEmpty {
		// an empty block's id is shared with peers — fork it. The caret is
		// reported through blockIds instead of a position in the (replaced)
		// focused block.
		p.filledEmptyBlockId = firstSelText.Model().Id
		p.caretPos = -1
	}
	return
}

// multiRange handles pasting when multiple blocks are selected. It merges the first paste
// text into the first selected block (if styles match), and the last paste text into the
// last selected block (if styles match). The middle selected blocks are marked for removal
// via removeSelection, and the paste blocks replace them via insertUnderSelection.
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

// insertUnderSelection moves all remaining blocks from the paste state into the document.
// Non-root paste blocks are added to the doc state and their IDs collected in blockIds.
// The paste root's children are inserted right after the first selected block (Block_Bottom).
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
			} else if file.Name != "" {
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

// normalize adjusts paste block styles that are only valid for specific system blocks:
// Title style is converted to Header1, Description style is converted to Paragraph.
// The original title/description blocks are exempt from conversion.
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

// intoCodeBlock handles pasting into a Code block or table cell. All paste text blocks
// are joined with newlines into a single string (or the TextSlot buffer is used directly),
// then inserted via RangeTextPaste with style copying enabled. The paste state children
// are cleared so insertUnderSelection does not duplicate them.
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
	// Strip trailing newlines when selection reaches the end of the code block,
	// because there is no text after the selection to absorb them. This prevents
	// clipboard artifacts (trailing \n or \r\n) from adding blank lines.
	textLen := int32(textutil.UTF16RuneCountString(selText.GetText()))
	if p.selRange.To >= textLen {
		txt = strings.TrimRight(txt, "\n\r")
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
