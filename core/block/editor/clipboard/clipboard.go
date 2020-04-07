package clipboard

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/converter"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	ErrAllSlotsEmpty        = errors.New("all slots are empty")
	ErrTitlePasteRestricted = errors.New("paste to title restricted")
	ErrOutOfRange           = errors.New("out of range")
	log                     = logging.Logger("anytype-clipboard")
)

type Clipboard interface {
	Cut(req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Paste(req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, err error)
	Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (html string, err error)
	Export(req pb.RpcBlockExportRequest, images map[string][]byte) (path string, err error)
}

func NewClipboard(sb smartblock.SmartBlock) Clipboard {
	return &clipboard{sb}
}

type clipboard struct {
	smartblock.SmartBlock
}

func (cb *clipboard) Paste(req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, err error) {
	caretPosition = -1

	if len(req.AnySlot) > 0 {
		blockIds, uploadArr, caretPosition, err = cb.pasteAny(req)

	} else if len(req.HtmlSlot) > 0 {
		blockIds, uploadArr, caretPosition, err = cb.pasteHtml(req)

		if err != nil {
			blockIds, uploadArr, caretPosition, err = cb.pasteText(req)
		}

	} else if len(req.TextSlot) > 0 {
		blockIds, uploadArr, caretPosition, err = cb.pasteText(req)

	} else {
		return nil, nil, caretPosition, ErrAllSlotsEmpty
	}

	return blockIds, uploadArr, caretPosition, err
}

func (cb *clipboard) Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (html string, err error) {

	blocksMap := make(map[string]*model.Block)
	for _, b := range req.Blocks {
		blocksMap[b.Id] = b
	}

	if err != nil {
		return "", err
	}

	conv := converter.New()
	return conv.Convert(req.Blocks, images), nil
}

func (cb *clipboard) Cut(req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	s := cb.NewState()

	blocksMap := make(map[string]*model.Block)
	textSlot = ""
	var ids []string

	for _, b := range req.Blocks {
		blocksMap[b.Id] = b

		if text := b.GetText(); text != nil {
			textSlot += text.Text + "\n"
		}

		ids = append(ids, b.Id)
	}

	for _, id := range ids {
		s.Remove(id)
	}

	if err != nil {
		return textSlot, htmlSlot, anySlot, err
	}

	conv := converter.New()
	htmlSlot = conv.Convert(req.Blocks, images)
	anySlot = req.Blocks

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

func (cb *clipboard) Export(req pb.RpcBlockExportRequest, images map[string][]byte) (path string, err error) {

	blocks := req.Blocks
	conv := converter.New()
	html := conv.Export(blocks, images)

	dir := os.TempDir()
	fileName := "export-" + cb.Id() + ".html"
	filePath := filepath.Join(dir, fileName)
	err = ioutil.WriteFile(filePath, []byte(html), 0644)

	if err != nil {
		return "", err
	}
	log.Debug("Export output. filepath:", filepath.Join(dir, fileName))

	return filePath, nil
}

func (cb *clipboard) pasteHtml(req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, err error) {
	mdToBlocksConverter := anymark.New()
	_, blocks := mdToBlocksConverter.HTMLToBlocks([]byte(req.HtmlSlot))
	req.AnySlot = blocks
	return cb.pasteAny(req)
}

func (cb *clipboard) pasteText(req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, err error) {
	if len(req.TextSlot) == 0 {
		return blockIds, uploadArr, caretPosition, nil
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

	return cb.pasteAny(req)

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
	for _, b := range anySlot {
		b.Id = uuid.New().String()
		anySlotreplacedIds = append(anySlotreplacedIds, b)
	}

	return anySlotreplacedIds
}

func (cb *clipboard) pasteAny(req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, err error) {
	s := cb.NewState()
	targetId := req.FocusedBlockId
	req.AnySlot = cb.replaceIds(req.AnySlot)
	req.AnySlot = cb.filterFromLayouts(req.AnySlot)
	isMultipleBlocksToPaste := len(req.AnySlot) > 1
	firstPasteBlockText := &model.BlockContentText{}
	firstPasteBlockText = nil

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

	focusedBlock := s.Get(targetId)
	focusedBlockText, ok := focusedBlock.(text.Block)
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
						return blockIds, uploadArr, caretPosition, err
					}
				}

			}
		}
	}

	if ok && focusedBlock != nil && focusedBlockText != nil && !isSelectedBlocks {
		focusedContent, isFocusedText = focusedBlock.Model().Content.(*model.BlockContentOfText)
		isFocusedTitle = isFocusedText && focusedContent.Text.Style == model.BlockContentText_Title

		isPasteTop = req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 && len(focusedContent.Text.Text) != 0
		isPasteBottom = req.SelectedTextRange.From == int32(len(focusedContent.Text.Text)) && req.SelectedTextRange.To == int32(len(focusedContent.Text.Text)) && req.SelectedTextRange.To != 0
		isPasteInstead = req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == int32(len(focusedContent.Text.Text))
		isPasteWithSplit = !isPasteInstead && !isPasteBottom && !isPasteTop
	}

	if isFocusedTitle {
		return blockIds, uploadArr, caretPosition, ErrTitlePasteRestricted
	}

	if req.CopyTextRange == nil {
		req.CopyTextRange = &model.Range{From: 0, To: 0}
	}

	if req.CopyTextRange.To == 0 {
		if firstPasteBlockText != nil && firstPasteBlockText.Text != "" {
			req.CopyTextRange.To = int32(len([]rune(firstPasteBlockText.Text)))

		}
	}

	pasteToTheEnd := targetId == "" && len(req.SelectedBlockIds) == 0 && len(cIds) > 0
	pasteSingleTextInFocusedText := focusedBlockText != nil && isFocusedText && !isFocusedTitle && !isMultipleBlocksToPaste && firstPasteBlockText != nil
	pasteMultipleBlocksInFocusedText := isFocusedText && (isMultipleBlocksToPaste || firstPasteBlockText == nil)
	pasteMultipleBlocksOnSelectedBlocks := isSelectedBlocks

	switch true {

	case pasteToTheEnd:
		targetId = cb.Pick(cIds[len(cIds)-1]).Model().Id
		blockIds, uploadArr, targetId, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Bottom, false)
		if err != nil {
			return blockIds, uploadArr, caretPosition, err
		}

		break

	case pasteSingleTextInFocusedText:
		caretPosition, err = focusedBlockText.RangeTextPaste(req.CopyTextRange.From, req.CopyTextRange.To, req.SelectedTextRange.From, req.SelectedTextRange.To, req.AnySlot[0].GetText())
		if err != nil {
			return nil, nil, -1, nil
		}
		break

	case pasteMultipleBlocksInFocusedText:
		if isPasteTop {

			blockIds, uploadArr, targetId, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Top, true)
			if err != nil {
				return blockIds, uploadArr, caretPosition, err
			}

			if len(focusedContent.Text.Text) == 0 {
				s.Remove(focusedBlock.Model().Id)
			}

		} else if isPasteBottom {
			blockIds, uploadArr, targetId, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Bottom, false)
			if err != nil {
				return blockIds, uploadArr, caretPosition, err
			}

		} else if isPasteInstead {
			blockIds, uploadArr, targetId, err = cb.insertBlocks(s, req.FocusedBlockId, req.AnySlot, model.Block_Bottom, false)
			if err != nil {
				return blockIds, uploadArr, caretPosition, err
			}
			s.Remove(req.FocusedBlockId)

			break

		} else if isPasteWithSplit {
			newBlock, err := focusedBlockText.RangeSplit(req.SelectedTextRange.From, req.SelectedTextRange.To)
			if err != nil {
				return blockIds, uploadArr, caretPosition, err
			}

			// insert new blocks
			pos := model.Block_Top
			isReversed := true
			blockIds, uploadArr, targetId, err = cb.insertBlocks(s, targetId, req.AnySlot, pos, isReversed)
			if err != nil {
				return blockIds, uploadArr, caretPosition, err
			}

			if len(newBlock.Model().GetText().Text) > 0 {
				s.Add(newBlock)
				err = s.InsertTo(targetId, model.Block_Top, newBlock.Model().Id)
				if err != nil {
					return blockIds, uploadArr, caretPosition, err
				}
				blockIds = append(blockIds, newBlock.Model().Id)
			}

			if len(focusedBlock.Model().GetText().Text) == 0 {
				s.Remove(focusedBlock.Model().Id)
			}
		}
		break

	case pasteMultipleBlocksOnSelectedBlocks:
		blockIds, uploadArr, targetId, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Bottom, false)
		if err != nil {
			return blockIds, uploadArr, caretPosition, err
		}
		for _, selectedBlockId := range req.SelectedBlockIds {
			s.Remove(selectedBlockId)
		}

		break
	}

	return blockIds, uploadArr, caretPosition, cb.Apply(s)
}

func (cb *clipboard) insertBlocks(s *state.State, targetId string, blocks []*model.Block, pos model.BlockPosition, isReversed bool) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, targetIdOut string, err error) {
	for i, _ := range blocks {
		index := i
		if isReversed {
			index = len(blocks) - i - 1
		}
		newBlock := simple.New(blocks[index])
		s.Add(newBlock)
		blockIds = append(blockIds, newBlock.Model().Id)
		err = s.InsertTo(targetId, pos, newBlock.Model().Id)
		if err != nil {
			return blockIds, uploadArr, targetId, err
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

		targetId = newBlock.Model().Id

		for _, childId := range blocks[i].ChildrenIds {
			childBlock := s.Get(childId)
			s.Add(childBlock)

			if err = s.InsertTo(blocks[i].Id, model.Block_Bottom, childId); err != nil {
				return blockIds, uploadArr, targetId, err
			}
		}

	}

	return blockIds, uploadArr, targetId, nil
}
