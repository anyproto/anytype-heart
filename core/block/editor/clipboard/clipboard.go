package clipboard

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/converter/html"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/util/anyblocks"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var (
	ErrAllSlotsEmpty        = errors.New("all slots are empty")
	ErrTitlePasteRestricted = errors.New("paste to title restricted")
	ErrOutOfRange           = errors.New("out of range")
	log                     = logging.Logger("anytype-clipboard")
)

type Clipboard interface {
	Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Paste(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)
	Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
}

func NewClipboard(sb smartblock.SmartBlock, file file.File) Clipboard {
	return &clipboard{SmartBlock: sb, file: file}
}

type clipboard struct {
	smartblock.SmartBlock
	file file.File
}

func (cb *clipboard) Paste(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	caretPosition = -1
	if len(req.FileSlot) > 0 {
		blockIds, err = cb.pasteFiles(ctx, req)
		return
	} else if len(req.AnySlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteAny(ctx, req)
	} else if len(req.HtmlSlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteHtml(ctx, req)

		if err != nil {
			blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteText(ctx, req)
		}

	} else if len(req.TextSlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteText(ctx, req)

	} else {
		return nil, nil, caretPosition, isSameBlockCaret, ErrAllSlotsEmpty
	}

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
}

func (cb *clipboard) Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	anySlot = req.Blocks
	textSlot = ""
	htmlSlot = ""

	if len(req.Blocks) == 0 {
		return textSlot, htmlSlot, anySlot, nil
	}

	s := cb.blocksToState(req.Blocks)

	var texts []string
	for _, b := range req.Blocks {
		if text := b.GetText(); text != nil {
			texts = append(texts, text.Text)
		}
	}

	if len(texts) > 0 {
		textSlot = strings.Join(texts, "\n")
	}

	firstBlock := s.Get(req.Blocks[0].Id)

	// scenario: rangeCopy
	if firstBlockText, isText := firstBlock.(text.Block); isText &&
		req.SelectedTextRange != nil &&
		!(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0) &&
		len(req.Blocks) == 1 {
		cutBlock, _, err := firstBlockText.RangeCut(req.SelectedTextRange.From, req.SelectedTextRange.To)
		if err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("error while cut: %s", err)
		}

		if cutBlock.GetText() != nil && cutBlock.GetText().Marks != nil {
			for i, m := range cutBlock.GetText().Marks.Marks {
				cutBlock.GetText().Marks.Marks[i].Range.From = m.Range.From - req.SelectedTextRange.From
				cutBlock.GetText().Marks.Marks[i].Range.To = m.Range.To - req.SelectedTextRange.From
			}
		}

		cutBlock.GetText().Style = model.BlockContentText_Paragraph
		textSlot = cutBlock.GetText().Text
		s.Set(simple.New(cutBlock))
		htmlSlot = html.NewHTMLConverter(cb.Anytype(), s).Convert()
		textSlot = cutBlock.GetText().Text
		anySlot = cb.stateToBlocks(s)
		return textSlot, htmlSlot, anySlot, nil
	}

	// scenario: ordinary copy
	htmlSlot = html.NewHTMLConverter(cb.Anytype(), s).Convert()
	anySlot = cb.stateToBlocks(s)
	return textSlot, htmlSlot, anySlot, nil
}

func (cb *clipboard) Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	s := cb.NewStateCtx(ctx)

	textSlot = ""

	if len(req.Blocks) == 0 || req.Blocks[0].Id == "" {
		return textSlot, htmlSlot, anySlot, errors.New("nothing to cut")
	}

	if len(req.Blocks) == 1 && req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 && req.Blocks[0].GetText() != nil {
		req.SelectedTextRange.To = int32(utf8.RuneCountInString(req.Blocks[0].GetText().Text))
	}

	firstBlock := s.Get(req.Blocks[0].Id)

	// scenario: rangeCut
	if firstBlockText, isText := firstBlock.(text.Block); isText &&
		req.SelectedTextRange != nil &&
		!(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0) &&
		len(req.Blocks) == 1 {

		cutBlock, initialBlock, err := firstBlockText.RangeCut(req.SelectedTextRange.From, req.SelectedTextRange.To)

		if err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("error while cut: %s", err)
		}

		firstBlock.(text.Block).SetText(initialBlock.GetText().Text, initialBlock.GetText().Marks)

		if cutBlock.GetText() != nil && cutBlock.GetText().Marks != nil {
			for i, m := range cutBlock.GetText().Marks.Marks {
				cutBlock.GetText().Marks.Marks[i].Range.From = m.Range.From - req.SelectedTextRange.From
				cutBlock.GetText().Marks.Marks[i].Range.To = m.Range.To - req.SelectedTextRange.From
			}
		}

		textSlot = cutBlock.GetText().Text
		anySlot = []*model.Block{cutBlock}
		cbs := cb.blocksToState(req.Blocks)
		cbs.Set(simple.New(cutBlock))
		htmlSlot = html.NewHTMLConverter(cb.Anytype(), cbs).Convert()

		return textSlot, htmlSlot, anySlot, cb.Apply(s)
	}

	// scenario: cutBlocks
	var ids []string
	for _, b := range req.Blocks {
		if text := b.GetText(); text != nil {
			textSlot += text.Text + "\n"
		}

		ids = append(ids, b.Id)
	}

	htmlSlot = html.NewHTMLConverter(cb.Anytype(), cb.blocksToState(req.Blocks)).Convert()
	anySlot = req.Blocks

	for i, _ := range req.Blocks {
		ok := s.Unlink(req.Blocks[i].Id)
		if !ok {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("can't remove block with id: %s", req.Blocks[i].Id)
		}
	}

	return textSlot, htmlSlot, anySlot, cb.Apply(s)
}

func (cb *clipboard) getImages(blocks map[string]*model.Block) (images map[string][]byte, err error) {
	for _, b := range blocks {
		if file := b.GetFile(); file != nil {
			if file.Type == model.BlockContentFile_Image {
				fh, err := cb.Anytype().FileByHash(context.TODO(), file.Hash)
				if err != nil {
					return images, err
				}

				reader, err := fh.Reader()
				if err != nil {
					return images, err
				}

				reader.Read(images[file.Hash])
			}
		}
	}

	return images, nil
}

func (cb *clipboard) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	s := cb.blocksToState(req.Blocks)
	htmlData := html.NewHTMLConverter(cb.Anytype(), s).Export()

	dir := os.TempDir()
	fileName := "export-" + cb.Id() + ".html"
	filePath := filepath.Join(dir, fileName)
	err = ioutil.WriteFile(filePath, []byte(htmlData), 0644)

	if err != nil {
		return "", err
	}
	log.Debug("Export output. filepath:", filepath.Join(dir, fileName))

	return filePath, nil
}

func (cb *clipboard) pasteHtml(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	mdToBlocksConverter := anymark.New()
	err, blocks, _ := mdToBlocksConverter.HTMLToBlocks([]byte(req.HtmlSlot))

	if err != nil {
		return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
	}

	req.AnySlot = blocks
	return cb.pasteAny(ctx, req)
}

func (cb *clipboard) pasteText(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	if utf8.RuneCountInString(req.TextSlot) == 0 {
		return blockIds, uploadArr, caretPosition, isSameBlockCaret, nil
	}

	textArr := strings.Split(req.TextSlot, "\n")

	if len(req.FocusedBlockId) > 0 {
		block := cb.Pick(req.FocusedBlockId)
		if block != nil {
			if b := block.Model().GetText(); b != nil && b.Style == model.BlockContentText_Code {
				textArr = []string{req.TextSlot}
			}
		}
	}

	req.AnySlot = []*model.Block{}
	for i := 0; i < len(textArr); i++ {
		req.AnySlot = append(req.AnySlot, &model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: textArr[i]},
			},
		})
	}

	return cb.pasteAny(ctx, req)

}

func (cb *clipboard) filterFromLayouts(anySlot []*model.Block) (anySlotFiltered []*model.Block) {
	for _, b := range anySlot {
		if b.GetLayout() == nil {
			anySlotFiltered = append(anySlotFiltered, b)
		}
	}

	return anySlotFiltered
}

func (cb *clipboard) replaceIds(anySlot []*model.Block) (anySlotreplacedIds []*model.Block) {
	var oldToNew map[string]string
	oldToNew = make(map[string]string)

	for i, _ := range anySlot {
		var oldId = make([]byte, len(anySlot[i].Id))

		newId := bson.NewObjectId().Hex()

		copy(oldId, anySlot[i].Id)
		oldToNew[string(oldId)] = newId
		anySlot[i].Id = newId
	}

	for i, _ := range anySlot {
		cIds := []string{}
		for _, cId := range anySlot[i].ChildrenIds {
			if len(oldToNew[cId]) > 0 {
				cIds = append(cIds, oldToNew[cId])
			}
		}
		anySlot[i].ChildrenIds = cIds
	}

	return anySlot
}

func (cb *clipboard) pasteAny(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	s := cb.NewStateCtx(ctx)
	targetId := req.FocusedBlockId
	isSameBlockCaret = false

	req.AnySlot = cb.replaceIds(req.AnySlot)
	req.AnySlot = cb.filterFromLayouts(req.AnySlot)

	isMultipleBlocksToPaste := len(req.AnySlot) > 1
	firstPasteBlockText := &model.BlockContentText{}
	firstPasteBlockText = nil

	req.AnySlot = uri.ProcessAllURI(req.AnySlot)

	caretPosition = -1

	if len(req.AnySlot) > 0 {
		firstPasteBlockText = req.AnySlot[0].GetText()
	}

	if req.SelectedTextRange == nil {
		req.SelectedTextRange = &model.Range{From: 0, To: 0}
	}

	if firstPasteBlockText != nil && firstPasteBlockText.Marks == nil {
		firstPasteBlockText.Marks = &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{}}
	}

	isSelectedBlocks := len(req.SelectedBlockIds) > 0
	if isSelectedBlocks {
		targetId = req.SelectedBlockIds[len(req.SelectedBlockIds)-1]
	}

	var focusedContent *model.BlockContentOfText

	isFocusedText := false
	isFocusedTitle := false

	isPasteTop := false
	isPasteBottom := false
	isPasteInstead := false
	isPasteWithSplit := false
	isPasteToCodeBlock := false

	focusedBlock := s.Get(targetId)
	focusedBlockText, ok := focusedBlock.(text.Block)
	if ok {
		isPasteToCodeBlock = focusedBlock.Model().GetText() != nil && focusedBlock.Model().GetText().Style == model.BlockContentText_Code
	}

	cIds := cb.Pick(cb.Id()).Model().ChildrenIds

	isEmptyPage := len(cIds) == 0
	if isEmptyPage {
		root := cb.Pick(cb.Id())
		if root != nil && root.Model() != nil && len(root.Model().ChildrenIds) > 0 {
			targetId = root.Model().ChildrenIds[0]
		} else {
			root := cb.Pick(cb.Id())
			children := []string{}
			for _, b := range req.AnySlot {
				newBlock := simple.New(b)
				s.Add(newBlock)
				children = append(children, newBlock.Model().Id)
				root.Model().ChildrenIds = children
				s.Set(root)

				targetId = newBlock.Model().Id
				focusedBlock = cb.Pick(targetId)
				focusedBlockText, ok = focusedBlock.(text.Block)

				for _, childId := range b.ChildrenIds {
					childBlock := s.Get(childId)
					s.Add(childBlock)

					if err = s.InsertTo(b.Id, model.Block_Bottom, childId); err != nil {
						return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
					}
				}
			}
		}
	}

	if ok && focusedBlock != nil && focusedBlockText != nil && !isSelectedBlocks {
		focusedContent, isFocusedText = focusedBlock.Model().Content.(*model.BlockContentOfText)
		//isFocusedTitle = isFocusedText && focusedContent.Text.Style == model.BlockContentText_Title

		isPasteTop = req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 && utf8.RuneCountInString(focusedContent.Text.Text) != 0
		isPasteBottom = req.SelectedTextRange.From == int32(utf8.RuneCountInString(focusedContent.Text.Text)) && req.SelectedTextRange.To == int32(utf8.RuneCountInString(focusedContent.Text.Text)) && req.SelectedTextRange.To != 0
		isPasteInstead = req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == int32(utf8.RuneCountInString(focusedContent.Text.Text))
		isPasteWithSplit = !isPasteInstead && !isPasteBottom && !isPasteTop
	}

	pasteToTheEnd := targetId == "" && len(req.SelectedBlockIds) == 0 && len(cIds) > 0
	pasteSingleTextInFocusedText := focusedBlockText != nil && isFocusedText && !isFocusedTitle && !isMultipleBlocksToPaste && firstPasteBlockText != nil
	pasteMultipleBlocksInFocusedText := isFocusedText && (isMultipleBlocksToPaste || firstPasteBlockText == nil)
	pasteMultipleBlocksOnSelectedBlocks := isSelectedBlocks

	switch true {

	case isPasteToCodeBlock:
		combinedCodeBlock := anyblocks.CombineCodeBlocks(req.AnySlot)
		caretPosition, err = focusedBlockText.RangeTextPaste(req.SelectedTextRange.From, req.SelectedTextRange.To, combinedCodeBlock, req.IsPartOfBlock)

	case pasteToTheEnd:
		targetId = cb.Pick(cIds[len(cIds)-1]).Model().Id
		blockIds, uploadArr, targetId, err = cb.insertBlocks(s, isPasteToCodeBlock, targetId, req.AnySlot, model.Block_Bottom, false)
		if err != nil {
			return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
		}
		break

	case pasteSingleTextInFocusedText:
		caretPosition, err = focusedBlockText.RangeTextPaste(req.SelectedTextRange.From, req.SelectedTextRange.To, req.AnySlot[0], req.IsPartOfBlock)
		if err != nil {
			return nil, nil, -1, isSameBlockCaret, err
		}
		break

	case pasteMultipleBlocksInFocusedText:
		if isPasteTop {
			isSameBlockCaret = true
			blockIds, uploadArr, targetId, err = cb.insertBlocks(s, isPasteToCodeBlock, targetId, req.AnySlot, model.Block_Top, true)
			if err != nil {
				return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
			}

			if utf8.RuneCountInString(focusedContent.Text.Text) == 0 {
				s.Unlink(focusedBlock.Model().Id)
			}

		} else if isPasteBottom {
			blockIds, uploadArr, targetId, err = cb.insertBlocks(s, isPasteToCodeBlock, targetId, req.AnySlot, model.Block_Bottom, false)
			if err != nil {
				return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
			}

		} else if isPasteInstead {
			blockIds, uploadArr, targetId, err = cb.insertBlocks(s, isPasteToCodeBlock, req.FocusedBlockId, req.AnySlot, model.Block_Bottom, false)
			if err != nil {
				return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
			}
			s.Unlink(req.FocusedBlockId)

			break

		} else if isPasteWithSplit {
			isSameBlockCaret = true
			newBlock, err := focusedBlockText.RangeSplit(req.SelectedTextRange.From, req.SelectedTextRange.To, true)
			if err != nil {
				return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
			}

			// insert new blocks
			pos := model.Block_Top
			isReversed := true
			blockIds, uploadArr, targetId, err = cb.insertBlocks(s, isPasteToCodeBlock, targetId, req.AnySlot, pos, isReversed)
			if err != nil {
				return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
			}

			if utf8.RuneCountInString(newBlock.Model().GetText().Text) > 0 {
				s.Add(newBlock)
				err = s.InsertTo(targetId, model.Block_Top, newBlock.Model().Id)

				if err != nil {
					return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
				}
				blockIds = append(blockIds, newBlock.Model().Id)
			}

			if utf8.RuneCountInString(focusedBlock.Model().GetText().Text) == 0 {
				s.Unlink(focusedBlock.Model().Id)
			}
		}
		break

	case pasteMultipleBlocksOnSelectedBlocks:
		blockIds, uploadArr, targetId, err = cb.insertBlocks(s, isPasteToCodeBlock, targetId, req.AnySlot, model.Block_Bottom, false)
		if err != nil {
			return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
		}
		for _, selectedBlockId := range req.SelectedBlockIds {
			s.Unlink(selectedBlockId)
		}

		break
	}

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, cb.Apply(s)
}

func (cb *clipboard) insertBlocks(s *state.State, isPasteToCodeBlock bool, targetId string, blocks []*model.Block, pos model.BlockPosition, isReversed bool) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, targetIdOut string, err error) {
	idToIsChild := make(map[string]bool)
	/*	if isPasteToCodeBlock {
		blocks = blocks.AllBlocksToCode(blocks)
	}*/

	for _, b := range blocks {
		for _, cId := range b.ChildrenIds {
			idToIsChild[cId] = true
		}
	}

	var newBlocks []simple.Block
	for i, _ := range blocks {
		index := i
		if isReversed {
			index = len(blocks) - i - 1
		}
		newBlock := simple.New(blocks[index])
		newBlocks = append(newBlocks, newBlock)
		s.Add(newBlock)
	}

	for i, _ := range blocks {
		index := i
		if isReversed {
			index = len(blocks) - i - 1
		}
		newBlock := newBlocks[i]

		blockIds = append(blockIds, newBlock.Model().Id)

		if idToIsChild[blocks[index].Id] != true {
			if targetId == template.TitleBlockId {
				targetId = template.HeaderLayoutId
				pos = model.Block_Bottom
			}
			err = s.InsertTo(targetId, pos, newBlock.Model().Id)

			if err != nil {
				return blockIds, uploadArr, targetId, err
			}

			targetId = newBlock.Model().Id
		}

		if f := newBlock.Model().GetFile(); f != nil {
			if f.State != model.BlockContentFile_Done {
				uploadArr = append(uploadArr,
					pb.RpcBlockUploadRequest{
						BlockId: newBlock.Model().Id,
						Url:     f.Name,
					})
			}
		}
	}

	return blockIds, uploadArr, targetId, nil
}

func (cb *clipboard) blocksToState(blocks []*model.Block) (cbs *state.State) {
	cbs = state.NewDoc("cbRoot", nil).(*state.State)
	cbs.SetDetails(cb.Details())
	cbs.Add(simple.New(&model.Block{Id: "cbRoot"}))

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
	cbs.BlocksInit()
	cbs.Normalize(false)
	return
}

func (cb *clipboard) stateToBlocks(s *state.State) []*model.Block {
	blocks := s.Blocks()
	result := blocks[:0]
	for _, b := range blocks {
		if b.Id != "cbRoot" {
			result = append(result, b)
		}
	}
	return result
}

func (cb *clipboard) pasteFiles(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	s := cb.NewStateCtx(ctx)
	for _, fs := range req.FileSlot {
		b := simple.New(&model.Block{
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					Name: fs.Name,
				},
			},
		})
		s.Add(b)
		if err = cb.file.UploadState(s, b.Model().Id, file.FileSource{
			Bytes: fs.Data,
			Path:  fs.LocalPath,
			Name:  fs.Name,
		}, true); err != nil {
			return
		}
		blockIds = append(blockIds, b.Model().Id)
	}
	if err = s.InsertTo(req.FocusedBlockId, model.Block_Bottom, blockIds...); err != nil {
		return
	}
	return blockIds, cb.Apply(s)
}
