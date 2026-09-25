package clipboard

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark/whitespace"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/converter/html"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

const clipboardRootId = "cbRoot"

var (
	ErrAllSlotsEmpty = errors.New("all slots are empty")
	log              = logging.Logger("anytype-clipboard")
)

type Clipboard interface {
	Cut(ctx session.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Paste(ctx session.Context, req *pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)
	Copy(ctx session.Context, req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
}

func NewClipboard(sb smartblock.SmartBlock, file file.File, tempDirProvider core.TempDirProvider, objectStore spaceindex.Store, fileService files.Service, fileObjectService fileobject.Service) Clipboard {
	return &clipboard{
		SmartBlock:        sb,
		file:              file,
		tempDirProvider:   tempDirProvider,
		objectStore:       objectStore,
		fileService:       fileService,
		fileObjectService: fileObjectService,
	}
}

type clipboard struct {
	smartblock.SmartBlock
	file              file.File
	tempDirProvider   core.TempDirProvider
	objectStore       spaceindex.Store
	fileService       files.Service
	fileObjectService fileobject.Service
}

func (cb *clipboard) Paste(ctx session.Context, req *pb.RpcBlockPasteRequest, groupId string) (
	blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error,
) {
	caretPosition = -1
	if err = cb.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
		return nil, nil, caretPosition, false, err
	}

	for _, b := range req.AnySlot {
		if b == nil {
			continue
		}
		if b.Id == template.FeaturedRelationsId {
			return nil, nil, caretPosition, false, fmt.Errorf("paste: block %q is a system block and cannot be pasted", b.Id)
		}
	}

	if len(req.FileSlot) > 0 {
		blockIds, err = cb.pasteFiles(ctx, req)
		return
	} else if len(req.AnySlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteAny(ctx, req, groupId)
	} else if len(req.HtmlSlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteHtml(ctx, req, groupId)

		if err != nil {
			blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteText(ctx, req, groupId)
		}

	} else if len(req.TextSlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteText(ctx, req, groupId)

	} else {
		return nil, nil, caretPosition, isSameBlockCaret, ErrAllSlotsEmpty
	}

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
}

func (cb *clipboard) Copy(ctx session.Context, req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	anySlot = req.Blocks
	textSlot = ""
	htmlSlot = ""

	if len(req.Blocks) == 0 {
		return textSlot, htmlSlot, anySlot, fmt.Errorf("copy: no blocks")
	}

	s := cb.blocksToState(req.Blocks)

	textSlot = renderText(s, len(req.Blocks) == 1)

	// scenario: multiBlockRangeCopy
	if req.SelectedTextRangeLastBlock != nil && len(req.Blocks) > 1 {
		firstBlock, lastBlock := req.Blocks[0], req.Blocks[len(req.Blocks)-1]
		if err = trimBlockToRange(s, firstBlock, req.SelectedTextRange); err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("trim first block to range: %w", err)
		}
		if err = trimBlockToRange(s, lastBlock, req.SelectedTextRangeLastBlock); err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("trim last block to range: %w", err)
		}
		textSlot = renderText(s, false)
		htmlSlot = cb.newHTMLConverter(s).Convert()
		anySlot = cb.stateToBlocks(s)
		return textSlot, htmlSlot, anySlot, nil
	}

	var firstTextBlock, lastTextBlock *model.Block
	for _, b := range req.Blocks {
		if b.GetText() != nil {
			if firstTextBlock == nil {
				firstTextBlock = b
			} else {
				lastTextBlock = b
			}
		}
	}

	// scenario: rangeCopy
	if isRangeSelect(firstTextBlock, lastTextBlock, req.SelectedTextRange) {
		cutBlock, _, err := simple.New(firstTextBlock).(text.Block).RangeCut(req.SelectedTextRange.From, req.SelectedTextRange.To)
		if err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("error while cut: %w", err)
		}

		shiftMarks(cutBlock, req.SelectedTextRange.From)
		tryClearStyle(cutBlock, req.SelectedTextRange)

		textSlot = cutBlock.GetText().Text
		s.Set(simple.New(cutBlock))
		htmlSlot = cb.newHTMLConverter(s).Convert()
		textSlot = cutBlock.GetText().Text
		anySlot = cb.stateToBlocks(s)
		return textSlot, htmlSlot, anySlot, nil
	}

	// scenario: ordinary copy
	htmlSlot = cb.newHTMLConverter(s).Convert()
	anySlot = cb.stateToBlocks(s)
	return textSlot, htmlSlot, anySlot, nil
}

func tryClearStyle(block *model.Block, rang *model.Range) {
	if rang.To-rang.From > 0 {
		block.GetText().Style = model.BlockContentText_Paragraph
		block.BackgroundColor = ""
	}
}

// shiftMarks moves mark ranges of a text block to the left by offset, so marks of a block
// cut out of a bigger one point to the right positions in the new text
func shiftMarks(block *model.Block, offset int32) {
	if block.GetText() == nil || block.GetText().Marks == nil {
		return
	}
	for _, m := range block.GetText().Marks.Marks {
		m.Range.From -= offset
		m.Range.To -= offset
	}
}

// isPartialRange reports whether r selects a proper part of a text block.
// A nil range, a {0,0} range or a range covering the whole text mean the whole block is selected.
func isPartialRange(b *model.Block, r *model.Range) bool {
	if b == nil || b.GetText() == nil || r == nil {
		return false
	}
	if r.From == 0 && (r.To == 0 || int(r.To) >= textutil.UTF16RuneCountString(b.GetText().Text)) {
		return false
	}
	return true
}

// trimBlockToRange replaces the block in s with a copy containing only the selected part of its text
func trimBlockToRange(s *state.State, b *model.Block, r *model.Range) error {
	if !isPartialRange(b, r) {
		return nil
	}
	cutBlock, _, err := simple.New(b).(text.Block).RangeCut(r.From, r.To)
	if err != nil {
		return fmt.Errorf("cut range %d-%d: %w", r.From, r.To, err)
	}
	shiftMarks(cutBlock, r.From)
	s.Set(simple.New(cutBlock))
	return nil
}

func (cb *clipboard) Cut(ctx session.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	s := cb.NewStateCtx(ctx)
	textSlot = ""

	stateBlocks, err := assertBlocks(s.Blocks(), req.Blocks)
	if err != nil {
		return textSlot, htmlSlot, anySlot, err
	}

	// scenario: multiBlockRangeCut
	if req.SelectedTextRangeLastBlock != nil && len(req.Blocks) > 1 {
		return cb.cutWithRanges(s, stateBlocks, req)
	}

	var firstTextBlock, lastTextBlock *model.Block
	for _, b := range req.Blocks {
		if b.GetText() != nil {
			if firstTextBlock == nil {
				firstTextBlock = b
			} else {
				lastTextBlock = b
			}
		} else {
			// if text block + object block - go to cutBlocks scenario imediately
			firstTextBlock = nil
			lastTextBlock = nil
			break
		}
	}

	if req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 && firstTextBlock != nil && lastTextBlock == nil {
		req.SelectedTextRange.To = int32(textutil.UTF16RuneCountString(firstTextBlock.GetText().Text))
	}

	// scenario: rangeCut
	if isRangeSelect(firstTextBlock, lastTextBlock, req.SelectedTextRange) {
		first := s.Get(firstTextBlock.Id).(text.Block)
		cutBlock, initialBlock, err := first.RangeCut(req.SelectedTextRange.From, req.SelectedTextRange.To)

		if err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("error while cut: %w", err)
		}

		first.SetText(initialBlock.GetText().Text, initialBlock.GetText().Marks)

		shiftMarks(cutBlock, req.SelectedTextRange.From)
		tryClearStyle(cutBlock, req.SelectedTextRange)
		textSlot = cutBlock.GetText().Text
		anySlot = []*model.Block{cutBlock}
		cbs := cb.blocksToState(req.Blocks)
		cbs.Set(simple.New(cutBlock))
		htmlSlot = cb.newHTMLConverter(cbs).Convert()

		return textSlot, htmlSlot, anySlot, cb.Apply(s)
	}

	// scenario: cutBlocks
	state := cb.blocksToState(req.Blocks)
	var ids []string
	for _, b := range req.Blocks {
		ids = append(ids, b.Id)
	}
	textSlot = renderText(state, len(req.Blocks) == 1)

	htmlSlot = cb.newHTMLConverter(state).Convert()
	anySlot = req.Blocks

	unlinkAndClearBlocks(s, stateBlocks, req.Blocks)
	return textSlot, htmlSlot, anySlot, cb.Apply(s)
}

// cutWithRanges cuts multiple blocks at once: the first and the last blocks may be cut partially
// by req.SelectedTextRange and req.SelectedTextRangeLastBlock, the blocks in between are cut whole.
// Partially cut blocks keep the not selected part of their text in the document
func (cb *clipboard) cutWithRanges(s *state.State, stateBlocks map[string]*model.Block, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	firstBlock, lastBlock := req.Blocks[0], req.Blocks[len(req.Blocks)-1]
	cbs := cb.blocksToState(req.Blocks)
	wholeBlocks := req.Blocks

	if isPartialRange(stateBlocks[firstBlock.Id], req.SelectedTextRange) {
		cutBlock, err := cutRangeFromBlock(s, firstBlock.Id, req.SelectedTextRange)
		if err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("cut range from first block: %w", err)
		}
		cbs.Set(simple.New(cutBlock))
		wholeBlocks = wholeBlocks[1:]
	}
	if isPartialRange(stateBlocks[lastBlock.Id], req.SelectedTextRangeLastBlock) {
		cutBlock, err := cutRangeFromBlock(s, lastBlock.Id, req.SelectedTextRangeLastBlock)
		if err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("cut range from last block: %w", err)
		}
		cbs.Set(simple.New(cutBlock))
		wholeBlocks = wholeBlocks[:len(wholeBlocks)-1]
	}

	textSlot = renderText(cbs, false)
	htmlSlot = cb.newHTMLConverter(cbs).Convert()
	anySlot = cb.stateToBlocks(cbs)

	unlinkAndClearBlocks(s, stateBlocks, wholeBlocks)
	return textSlot, htmlSlot, anySlot, cb.Apply(s)
}

// cutRangeFromBlock cuts the selected range out of a text block in the document state
// and returns the cut part with mark positions adjusted to the new text
func cutRangeFromBlock(s *state.State, id string, r *model.Range) (*model.Block, error) {
	textBlock, ok := s.Get(id).(text.Block)
	if !ok {
		return nil, fmt.Errorf("block %s is not a text block", id)
	}
	cutBlock, initialBlock, err := textBlock.RangeCut(r.From, r.To)
	if err != nil {
		return nil, fmt.Errorf("range cut: %w", err)
	}
	textBlock.SetText(initialBlock.GetText().Text, initialBlock.GetText().Marks)
	shiftMarks(cutBlock, r.From)
	return cutBlock, nil
}

func isRangeSelect(firstTextBlock *model.Block, lastTextBlock *model.Block, rang *model.Range) bool {
	return firstTextBlock != nil &&
		lastTextBlock == nil &&
		rang != nil &&
		rang.To-rang.From != int32(textutil.UTF16RuneCountString(firstTextBlock.GetText().Text)) &&
		rang.To > 0
}

func unlinkAndClearBlocks(
	s *state.State,
	stateBlocks map[string]*model.Block,
	requestBlocks []*model.Block,
) {
	for _, block := range requestBlocks {
		if block.GetLayout() != nil {
			continue
		}
		stateBlock := stateBlocks[block.Id]
		if stateBlock.Restrictions == nil || !stateBlock.Restrictions.Remove {
			s.Unlink(block.Id)
		} else {
			if textBlock, ok := s.Get(block.Id).(text.Block); ok {
				textBlock.SetText("", nil)
			}
		}
	}
}

func assertBlocks(stateBlocks []*model.Block, requestBlocks []*model.Block) (map[string]*model.Block, error) {
	if len(requestBlocks) == 0 || requestBlocks[0].GetId() == "" {
		return nil, errors.New("nothing to cut")
	}

	idToBlockMap := make(map[string]*model.Block)
	for _, stateBlock := range stateBlocks {
		idToBlockMap[stateBlock.GetId()] = stateBlock
	}

	for _, requestBlock := range requestBlocks {
		reqId := requestBlock.GetId()
		if reqId == "" {
			return nil, errors.New("empty requestBlock id")
		}
		if _, ok := idToBlockMap[reqId]; !ok {
			return nil, fmt.Errorf("requestBlock with id %s not found", reqId)
		}
	}
	return idToBlockMap, nil
}

func (cb *clipboard) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	s := cb.blocksToState(req.Blocks)
	htmlData := cb.newHTMLConverter(s).Export()

	dir := cb.tempDirProvider.TempDir()
	fileName := "export-" + cb.Id() + ".html"
	filePath := filepath.Join(dir, fileName)
	err = ioutil.WriteFile(filePath, []byte(htmlData), 0644)

	if err != nil {
		return "", err
	}

	return filePath, nil
}

func (cb *clipboard) pasteHtml(ctx session.Context, req *pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	blocks, _, err := anymark.HTMLToBlocks([]byte(req.HtmlSlot), req.Url)

	if err != nil {
		return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
	}

	// See GO-250 for more details
	// In short: if we paste plaintext blocks into a styled block, we make first ones to inherit style from this block
	if focused := cb.Pick(req.FocusedBlockId); focused != nil {
		if focusedTxt := focused.Model().GetText(); focusedTxt != nil && focusedTxt.Style != model.BlockContentText_Paragraph {
			for _, b := range blocks {
				if txt := b.GetText(); txt != nil && txt.Style == model.BlockContentText_Paragraph {
					txt.Style = focusedTxt.Style
				} else {
					break
				}
			}
		}
	}

	req.AnySlot = blocks
	return cb.pasteAny(ctx, req, groupId)
}

func (cb *clipboard) pasteText(ctx session.Context, req *pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	if len(req.TextSlot) == 0 {
		return blockIds, uploadArr, caretPosition, isSameBlockCaret, nil
	}

	if len(req.FocusedBlockId) > 0 {
		block := cb.Pick(req.FocusedBlockId)
		if block != nil {
			if b := block.Model().GetText(); b != nil && b.Style == model.BlockContentText_Code {
				return cb.pasteRawText(ctx, req, []string{req.TextSlot}, groupId)
			}
		}
	}

	// A paste that is nothing but a TeX formula has no surrounding text for the
	// Markdown parser to format, so hand it over verbatim instead of letting the
	// parser read the sub- and superscript underscores as an emphasis pair. Mixed
	// content still goes through the parser untouched: telling maths apart from
	// markup inside a sentence needs the parser's own context, which a pre-pass
	// over the raw text does not have. See GO-7330.
	if formula, ok := standaloneFormula(req.TextSlot); ok {
		return cb.pasteRawText(ctx, req, []string{formula}, groupId)
	}

	mdText := whitespace.WhitespaceNormalizeString(req.TextSlot)
	blocks, _, err := anymark.MarkdownToBlocks([]byte(mdText), "", []string{})
	if err != nil {
		// in case we've failed to parse the text as a valid markdown,
		// split it into text paragraphs with the same logic like in anymark and paste it as a plain text
		paragraphs := splitStringIntoParagraphs(req.TextSlot, anymark.TextBlockLengthSoftLimit)
		return cb.pasteRawText(ctx, req, paragraphs, groupId)
	}
	req.AnySlot = append(req.AnySlot, blocks...)

	return cb.pasteAny(ctx, req, groupId)
}

// charsOutsideFormula are the characters that disqualify a span from the fast
// path, each for its own reason.
//
// A line break is excluded so that the number of blocks produced cannot change.
// The parser starts a new block at a line break, so a multi-line paste can yield
// several blocks where the raw path always yields one, and downstream paste
// modes behave differently on one block than on several. Without a line break
// the parser has no split point and no block-level construct can start, because
// the text begins with a dollar rather than a list, heading, quote or fence
// marker — so both routes produce exactly one paragraph.
//
// Brackets, backticks and angle brackets are excluded because they are the
// inline Markdown that still means something on a single line: links and images
// need brackets, code spans need a backtick, autolinks and raw HTML need angle
// brackets. Their presence says real markup may be at stake, which is more than
// a formula should be carrying. Emphasis characters are deliberately not
// excluded — they are what the parser eats and the formula needs kept. A pipe is
// allowed too: a table needs a delimiter row on a second line, which the
// line-break rule has already ruled out, so a lone pipe cannot mean anything.
const charsOutsideFormula = "\n\r[]`<>"

// standaloneFormula reports whether the whole text is a single TeX math span,
// and returns it trimmed. It qualifies when $…$ or $$…$$ delimiters bound the
// entire text, no further dollar sits between them, the content holds at least
// one control sequence (a backslash followed by a letter) and none of
// charsOutsideFormula.
//
// The delimiters bounding the whole text is what makes the answer safe to act
// on: there is no other content that Markdown could have formatted, so nothing
// can be lost by not parsing it. A formula inside a sentence deliberately does
// not qualify — separating maths from markup there needs the parser's context.
func standaloneFormula(text string) (string, bool) {
	text = strings.TrimSpace(text)
	for _, delim := range []string{"$$", "$"} {
		if len(text) <= 2*len(delim) || !strings.HasPrefix(text, delim) || !strings.HasSuffix(text, delim) {
			continue
		}
		content := text[len(delim) : len(text)-len(delim)]
		if strings.Contains(content, "$") || strings.ContainsAny(content, charsOutsideFormula) {
			continue
		}
		if hasControlSequence(content) {
			return text, true
		}
	}
	return "", false
}

// hasControlSequence reports whether text contains a TeX control sequence, a
// backslash followed by an ASCII letter, such as \alpha or \mathbf.
func hasControlSequence(text string) bool {
	for i := 0; i+1 < len(text); i++ {
		if text[i] != '\\' {
			continue
		}
		if c := text[i+1]; c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

func (cb *clipboard) pasteRawText(ctx session.Context, req *pb.RpcBlockPasteRequest, textArr []string, groupId string) ([]string, []pb.RpcBlockUploadRequest, int32, bool, error) {
	req.AnySlot = make([]*model.Block, 0, len(textArr))
	for _, text := range textArr {
		if text != "" {
			req.AnySlot = append(req.AnySlot, &model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Text: text},
				},
			})
		}
	}
	return cb.pasteAny(ctx, req, groupId)
}

// some types of blocks need a special duplication mechanism
type duplicatable interface {
	Duplicate(s *state.State) (newId string, visitedIds []string, blocks []simple.Block, err error)
}

func (cb *clipboard) pasteAny(
	ctx session.Context, req *pb.RpcBlockPasteRequest, groupID string,
) (
	blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error,
) {
	s := cb.NewStateCtx(ctx).SetGroupId(groupID)

	destState := state.NewDoc(clipboardRootId, nil).(*state.State)

	// AnySlot is assembled by clients, importers and integrations, so it may be structurally
	// invalid. Sanitize it before anything downstream dereferences it.
	req.AnySlot = slice.Filter(req.AnySlot, func(b *model.Block) bool {
		return b != nil
	})

	for _, b := range req.AnySlot {
		if b.Id == "" {
			b.Id = bson.NewObjectId().Hex()
		}
		if b.Id == template.TitleBlockId || b.Id == template.DescriptionBlockId {
			if b.Fields != nil {
				delete(b.Fields.Fields, text.DetailsKeyFieldName)
			}
		}
		sanitizeMarks(b)
		if d, ok := b.Content.(*model.BlockContentOfDataview); ok {
			if err = cb.addRelationLinksToDataview(d.Dataview); err != nil {
				return
			}
		}
		if f, ok := b.Content.(*model.BlockContentOfFile); ok {
			cb.processFileBlock(f)
		}
	}
	srcState := cb.blocksToState(req.AnySlot)
	visited := map[string]struct{}{}

	src := srcState.Blocks()
	srcBlocks := make([]simple.Block, 0, len(src))
	for _, b := range src {
		srcBlocks = append(srcBlocks, simple.New(b))
	}

	oldToNew := map[string]string{}
	// Handle blocks that have custom duplication code. For example, simple tables
	// have to have special ID for cells
	for _, b := range srcBlocks {
		if d, ok := b.(duplicatable); ok {
			id, visitedIds, blocks, err2 := d.Duplicate(srcState)
			if err2 != nil {
				err = fmt.Errorf("custom duplicate: %w", err2)
				return
			}

			oldToNew[b.Model().Id] = id
			for _, b := range blocks {
				destState.Add(b)
			}
			for _, id := range visitedIds {
				visited[id] = struct{}{}
			}
		}
	}

	// Collect and generate necessary IDs. Ignore ids of blocks that have been duplicated by custom code
	for _, b := range srcBlocks {
		if _, ok := visited[b.Model().Id]; ok {
			continue
		}
		oldToNew[b.Model().Id] = bson.NewObjectId().Hex()
	}

	// Remap IDs
	for _, b := range srcBlocks {
		if _, ok := visited[b.Model().Id]; ok {
			continue
		}
		b.Model().Id = oldToNew[b.Model().Id]
		for i, id := range b.Model().ChildrenIds {
			b.Model().ChildrenIds[i] = oldToNew[id]
		}
		destState.Add(b)
	}
	destState.SetRootId(oldToNew[clipboardRootId])
	destState.BlocksInit(destState)
	state.CleanupLayouts(destState)
	if err = destState.Normalize(false); err != nil {
		return
	}

	var missingRelationKeys []domain.RelationKey
	// collect missing relation keys to add it to state
	for _, b := range s.Blocks() {
		if r := b.GetRelation(); r != nil {
			missingRelationKeys = append(missingRelationKeys, domain.RelationKey(r.Key))
		}
	}

	// TODO: GO-4284 remove
	if len(missingRelationKeys) > 0 {
		if err = cb.AddRelationLinksToState(s, missingRelationKeys...); err != nil {
			return
		}
	}

	ctrl := &pasteCtrl{s: s, ps: destState}
	if err = ctrl.Exec(req); err != nil {
		return
	}
	caretPosition = ctrl.caretPos
	uploadArr = ctrl.uploadArr
	blockIds = ctrl.blockIds

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, cb.Apply(s)
}

func (cb *clipboard) blocksToState(blocks []*model.Block) (cbs *state.State) {
	cbs = state.NewDoc(clipboardRootId, nil).(*state.State)
	cbs.SetDetails(cb.Details())
	cbs.Add(simple.New(&model.Block{Id: clipboardRootId}))

	var inChildrens, rootIds []string
	for _, b := range blocks {
		inChildrens = append(inChildrens, b.ChildrenIds...)
	}
	for _, b := range blocks {
		if slice.FindPos(inChildrens, b.Id) == -1 {
			rootIds = append(rootIds, b.Id)
		}
		cbs.Add(simple.New(b))
	}
	cbs.Pick(cbs.RootId()).Model().ChildrenIds = rootIds
	cbs.BlocksInit(cbs)
	state.CleanupLayouts(cbs)
	cbs.Normalize(false)
	return
}

func (cb *clipboard) stateToBlocks(s *state.State) []*model.Block {
	blocks := s.Blocks()
	result := blocks[:0]
	for _, b := range blocks {
		if b.Id != clipboardRootId {
			result = append(result, b)
		}
	}
	return result
}

func (cb *clipboard) pasteFiles(ctx session.Context, req *pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	if req.FocusedBlockId == template.TitleBlockId || req.FocusedBlockId == template.DescriptionBlockId {
		req.FocusedBlockId = ""
	}
	s := cb.NewStateCtx(ctx)
	for _, fs := range req.FileSlot {
		b := simple.New(&model.Block{
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					Name:  fs.Name,
					Style: model.BlockContentFile_Auto,
				},
			},
		})
		s.Add(b)

		if err = cb.file.UploadState(ctx, s, b.Model().Id, file.FileSource{
			Bytes:               fs.Data,
			Path:                fs.LocalPath,
			Name:                fs.Name,
			Origin:              objectorigin.Clipboard(),
			ImageKind:           model.ImageKind_Basic,
			CreatedInContext:    cb.Id(),
			CreatedInContextRef: b.Model().Id,
		}, false); err != nil {
			return
		}
		blockIds = append(blockIds, b.Model().Id)
	}

	if err = s.InsertTo(req.FocusedBlockId, cb.getFileBlockPosition(req), blockIds...); err != nil {
		return
	}
	return blockIds, cb.Apply(s)
}

func (cb *clipboard) getFileBlockPosition(req *pb.RpcBlockPasteRequest) model.BlockPosition {
	b := cb.Pick(req.FocusedBlockId)
	if b == nil {
		return model.Block_Bottom
	}
	if txt := b.Model().GetText(); txt != nil && txt.Text == "" {
		return model.Block_Replace
	}
	return model.Block_Bottom
}

// sanitizeMarks makes every text mark of the block applicable to the block's text.
//
// A mark whose range merely overruns the text still says what the user meant, and the paste
// machinery already clips such a range when it pastes into an existing block, so we repair it
// here the same way and keep the mark with its param. Only a mark that cannot be repaired is
// removed: one with no range at all, one whose range is reversed, and one that lies entirely
// outside the text. Those describe no span of this text, so there is nothing to preserve,
// while storing one makes the block impossible to edit afterwards - the mark handling code
// dereferences Range freely.
//
// A bad mark is dropped rather than failing the whole request: a paste carries the user's
// text, an unrepairable mark comes from the software assembling the slot rather than from the
// user, and rejecting the paste would leave the user unable to paste their content at all.
func sanitizeMarks(b *model.Block) {
	txt := b.GetText()
	if txt == nil || txt.Marks == nil || len(txt.Marks.Marks) == 0 {
		return
	}
	// offsets are counted in UTF-16 code units, the unit used by the mark code and the clients
	textLen := int32(textutil.UTF16RuneCountString(txt.Text))
	validMarks := txt.Marks.Marks[:0]
	for _, m := range txt.Marks.Marks {
		if m == nil {
			log.Warnf("paste: drop mark of block %s: mark is not set", b.Id)
			continue
		}
		if err := repairMarkRange(m.Range, textLen); err != nil {
			log.Warnf("paste: drop %s mark of block %s: %v", m.Type.String(), b.Id, err)
			continue
		}
		validMarks = append(validMarks, m)
	}
	txt.Marks.Marks = validMarks
}

// repairMarkRange clips r to a text of textLen UTF-16 code units, or reports why r cannot be
// applied to that text at all.
//
// The bounds follow what the text API itself produces: SetMarkForAllText marks a text of
// length n as [0, n], so To may equal textLen, and on an empty block that is [0, 0], so a
// zero-width range at the very end is legal too. Interior zero-width ranges are legal for the
// same reason, which is why a reversed range is From strictly greater than To.
func repairMarkRange(r *model.Range, textLen int32) error {
	if r == nil {
		return fmt.Errorf("range is not set")
	}
	if r.From > r.To {
		return fmt.Errorf("range.from %d is greater than range.to %d", r.From, r.To)
	}
	// a negative To implies a negative From, since From is not greater than To
	if r.To < 0 {
		return fmt.Errorf("range %d-%d is entirely before the text", r.From, r.To)
	}
	if r.From > textLen {
		return fmt.Errorf("range %d-%d is past the end of text of length %d", r.From, r.To, textLen)
	}
	if r.From < 0 {
		r.From = 0
	}
	if r.To > textLen {
		r.To = textLen
	}
	return nil
}

func (cb *clipboard) addRelationLinksToDataview(d *model.BlockContentDataview) (err error) {
	if d == nil {
		return nil
	}
	relationKeys := make(map[string]struct{})
	if len(d.RelationLinks) != 0 || len(d.Views) == 0 {
		return
	}
	for _, v := range d.Views {
		for _, r := range v.Relations {
			if _, found := relationKeys[r.Key]; !found {
				relationKeys[r.Key] = struct{}{}
			}
		}
	}
	if len(relationKeys) == 0 {
		return
	}

	relationKeysList := make([]domain.RelationKey, 0, len(relationKeys))
	for k := range relationKeys {
		relationKeysList = append(relationKeysList, domain.RelationKey(k))
	}
	relations, err := cb.objectStore.FetchRelationByKeys(relationKeysList...)
	if err != nil {
		return fmt.Errorf("failed to fetch relation keys of dataview: %w", err)
	}
	links := make([]*model.RelationLink, 0, len(relations))
	for _, r := range relations {
		links = append(links, r.RelationLink())
	}

	d.RelationLinks = links
	return
}

func (cb *clipboard) newHTMLConverter(s *state.State) *html.HTML {
	return html.NewHTMLConverter(s, cb.fileObjectService)
}

func (cb *clipboard) processFileBlock(f *model.BlockContentOfFile) {
	if f.File == nil || f.File.TargetObjectId == "" {
		return
	}
	fileId, err := cb.fileObjectService.GetFileIdFromObject(f.File.TargetObjectId)
	if err != nil {
		log.Errorf("failed to get fileId: %v", err)
		return
	}

	if cb.SpaceID() == fileId.SpaceId {
		return
	}

	objectId, err := cb.fileObjectService.CreateFromImport(
		domain.FullFileId{SpaceId: cb.SpaceID(), FileId: fileId.FileId},
		objectorigin.ObjectOrigin{Origin: model.ObjectOrigin_clipboard},
		nil,
	)
	if err != nil {
		log.Errorf("failed to create file object: %v", err)
		return
	}

	f.File.TargetObjectId = objectId
}

func renderText(s *state.State, ignoreStyle bool) string {
	texts := make([]string, 0)
	texts, _ = renderBlock(s, texts, s.RootId(), -1, 0, ignoreStyle)

	if len(texts) > 0 {
		return strings.Join(texts, "\n")
	}

	return ""
}

func renderBlock(s *state.State, texts []string, id string, level int, numberedCount int, ignoreStyle bool) ([]string, int) {
	block := s.Pick(id).Model()
	texts, numberedCount = extractTextWithStyleAndTabs(block, texts, level, numberedCount, ignoreStyle)
	childrenIds := s.Pick(id).Model().ChildrenIds
	texts = renderChildren(s, texts, childrenIds, level, 0, ignoreStyle)
	return texts, numberedCount
}

func renderChildren(s *state.State, texts []string, childrenIds []string, level int, numberedCount int, ignoreStyle bool) []string {
	var oldNumberedCount int
	for _, id := range childrenIds {
		oldNumberedCount = numberedCount
		texts, numberedCount = renderBlock(s, texts, id, level+1, numberedCount, ignoreStyle)
		if oldNumberedCount == numberedCount {
			numberedCount = 0
		}
	}
	return texts
}

func extractTextWithStyleAndTabs(block *model.Block, texts []string, level int, numberedCount int, ignoreStyle bool) ([]string, int) {
	if blockText := block.GetText(); blockText != nil {
		if ignoreStyle {
			texts = append(texts, blockText.Text)
		} else {
			switch blockText.Style {
			case model.BlockContentText_Title:
				texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "# ", blockText.Text))
			case model.BlockContentText_Header1, model.BlockContentText_ToggleHeader1:
				texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "## ", blockText.Text))
			case model.BlockContentText_Header2, model.BlockContentText_ToggleHeader2:
				texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "### ", blockText.Text))
			case model.BlockContentText_Header3, model.BlockContentText_ToggleHeader3:
				texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "#### ", blockText.Text))
			case model.BlockContentText_Header4:
				texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "##### ", blockText.Text))
			case model.BlockContentText_Quote:
				texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "> ", blockText.Text))
			case model.BlockContentText_Code:
				texts = append(texts, fmt.Sprintf("%s%s%s%s", strings.Repeat("\t", level), "```", blockText.Text, "```"))
			case model.BlockContentText_Checkbox:
				texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "- [ ] ", blockText.Text))
			case model.BlockContentText_Marked:
				texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "- ", blockText.Text))
			case model.BlockContentText_Numbered:
				numberedCount++
				texts = append(texts, fmt.Sprintf("%s%d%s%s", strings.Repeat("\t", level), numberedCount, ". ", blockText.Text))
			case model.BlockContentText_Callout:
				texts = append(texts, fmt.Sprintf("%s%s%s%s", strings.Repeat("\t", level), blockText.IconEmoji, " ", blockText.Text))
			default:
				texts = append(texts, fmt.Sprintf("%s%s", strings.Repeat("\t", level), blockText.Text))
			}
		}
	}
	return texts, numberedCount
}

// splitStringIntoParagraphs splits text into pararagraphs
// - when text has a double line break, it is considered as a paragraph separator
// - when text has a single line break, it is considered as a soft line break, not a paragraph separator
// - when text has a single line break and the current block is longer than the soft limit, it is considered as a paragraph separator
// - func consider line with whitespaces as a paragraph separator (e.g. "\n   \n")
func splitStringIntoParagraphs(s string, lineBreakSoftLimit int) []string {
	var blocks []string
	var currentBlock strings.Builder

	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.TrimSpace(line) == "" { // This is a simple proxy for a double line break.
			if currentBlock.Len() > 0 {
				blocks = append(blocks, currentBlock.String())
				currentBlock.Reset()
			}
			continue
		}

		// Add line to current block with space handling for the soft limit.
		if lineBreakSoftLimit > 0 && currentBlock.Len()+len(line) > lineBreakSoftLimit && currentBlock.Len() > 0 {
			// Append the current block and start a new one
			blocks = append(blocks, currentBlock.String())
			currentBlock.Reset()
		}

		if currentBlock.Len() > 0 {
			currentBlock.WriteString("\n")
		}
		currentBlock.WriteString(line)
	}

	// Don't forget to add the last block if it exists.
	if currentBlock.Len() > 0 {
		blocks = append(blocks, currentBlock.String())
	}

	return blocks
}
