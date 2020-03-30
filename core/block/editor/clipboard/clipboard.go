package clipboard

import (
	"context"
	"errors"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	logging "github.com/ipfs/go-log"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/converter"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	ErrAllSlotsEmpty = errors.New("All slots are empty")
	ErrOutOfRange    = errors.New("out of range")
	log = logging.Logger("anytype-clipboard")
)

type Clipboard interface {
	Cut(req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Paste(req pb.RpcBlockPasteRequest) (blockIds []string, err error)
	Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (html string, err error)
	Export(req pb.RpcBlockExportRequest, images map[string][]byte) (path string, err error)
}

func NewClipboard(sb smartblock.SmartBlock) Clipboard {
	return &clipboard{sb}
}

type clipboard struct {
	smartblock.SmartBlock
}

func (cb *clipboard) Paste(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	if len(req.AnySlot) > 0 {
		return cb.pasteAny(req)
	} else if len(req.HtmlSlot) > 0 {
		blockIds, err = cb.pasteHtml(req)

		if err != nil {
			return cb.pasteText(req)
		} else {
			return blockIds, err
		}

	} else if len(req.TextSlot) > 0 {
		return cb.pasteText(req)
	} else {
		return nil, ErrAllSlotsEmpty
	}
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
		s.Unlink(id)
	}

	if err != nil {
		return textSlot, htmlSlot, anySlot, err
	}

	conv := converter.New()
	htmlSlot = conv.Convert(req.Blocks, images)
	anySlot = req.Blocks

	return textSlot, htmlSlot, anySlot, cb.Apply(s)

}

func (cb *clipboard) blocksTreeToMap(blocksMapIn map[string]*model.Block, ids []string) (blocksMapOut map[string]*model.Block) {
	blocksMapOut = blocksMapIn
	blocks := cb.Blocks()

	for i, id := range ids {
		blocksMapOut[id] = blocks[i]
		if len(blocks[i].ChildrenIds) > 0 {
			blocksMapOut = cb.blocksTreeToMap(blocksMapOut, blocks[i].ChildrenIds)
		}
	}
	return blocksMapOut
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
	fmt.Println("filepath.Join(dir, fileName)", filepath.Join(dir, fileName), dir, fileName)
	fmt.Println(html)

	return filePath, nil
}

func (cb *clipboard) pasteHtml(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	mdToBlocksConverter := anymark.New()
	_, blocks := mdToBlocksConverter.HTMLToBlocks([]byte(req.HtmlSlot))
	fmt.Println("HERE1", req.HtmlSlot, blocks)
	req.AnySlot = blocks
	return cb.pasteAny(req)
}

func (cb *clipboard) pasteText(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	if len(req.TextSlot) == 0 {
		return blockIds, nil
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

	fmt.Println("BLOCKS text:", req.AnySlot)

	blockIds, err = cb.pasteAny(req)
	log.Error("ERROR pasteAny:", err)
	return blockIds, err

}

func (cb *clipboard) filterFromLayouts (anySlot []*model.Block) (anySlotFiltered []*model.Block) {
	for _, b := range anySlot {
		if b.GetLayout() == nil {
			anySlotFiltered = append(anySlotFiltered, b)
		}
	}

	return anySlotFiltered
}

func (cb *clipboard) pasteAny(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	s := cb.NewState()
	targetId := req.FocusedBlockId

	req.AnySlot = cb.filterFromLayouts(req.AnySlot)
	isMultipleBlocksToPaste := len(req.AnySlot) > 1
	firstPasteBlockText := req.AnySlot[0].GetText()


	if req.SelectedTextRange == nil {
		req.SelectedTextRange = &model.Range{From: 0, To: 0}
	}

	if firstPasteBlockText.Marks == nil {
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

	focusedBlock :=  cb.Pick(targetId)
	focusedBlockText, ok := focusedBlock.(text.Block)

	isEmptyPage := !ok && len(req.SelectedBlockIds) == 0
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

				for _, childId := range b.ChildrenIds {
					childBlock := s.Get(childId)
					s.Add(childBlock)

					if err = s.InsertTo(b.Id, model.Block_Bottom, childId); err != nil {
						return blockIds, err
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

	pasteSingleTextInFocusedTitle := isFocusedTitle && !isMultipleBlocksToPaste
	pasteMultipleBlocksInFocusedTitle := isFocusedTitle && isMultipleBlocksToPaste
	pasteSingleTextInFocusedText := isFocusedText && !isFocusedTitle && !isMultipleBlocksToPaste
	pasteMultipleBlocksInFocusedText := isFocusedText && isMultipleBlocksToPaste
	pasteMultipleBlocksOnSelectedBlocks := isSelectedBlocks

	switch true {
	case pasteSingleTextInFocusedTitle:
		firstPasteBlockText.Text = strings.Replace(firstPasteBlockText.Text, "\n", " ", -1)
		firstPasteBlockText.Marks = &model.BlockContentTextMarks{}

		err = focusedBlockText.RangeTextPaste(req.SelectedTextRange.From, req.SelectedTextRange.To, firstPasteBlockText.Text, firstPasteBlockText.Marks.Marks)
		if err != nil {
			return blockIds, err
		}
		break

	case pasteSingleTextInFocusedText:
		err = focusedBlockText.RangeTextPaste(req.SelectedTextRange.From, req.SelectedTextRange.To, firstPasteBlockText.Text, firstPasteBlockText.Marks.Marks)
		if err != nil {
			return blockIds, err
		}
		break

	case pasteMultipleBlocksInFocusedText:
		if isPasteTop {
			fmt.Println("@isPasteTop")
			blockIds, _, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Top, true)
			if err != nil {
				return blockIds, err
			}

			if len(focusedContent.Text.Text) == 0 {
				s.Unlink(focusedBlock.Model().Id)
			}

		} else if isPasteBottom {
			fmt.Println("@isPasteBottom")
			blockIds, _, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Bottom, false)
			if err != nil {
				return blockIds, err
			}

		} else if isPasteInstead {
			fmt.Println("@isPasteInstead")
			blockIds, _, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Top, false)
			if err != nil {
				return blockIds, err
			}

			if !pasteMultipleBlocksInFocusedTitle {
				s.Unlink(targetId)
			}

		} else if isPasteWithSplit {
			fmt.Println("@isPasteWithSplit")
			newBlocks, txt, err := focusedBlockText.RangeSplit(req.SelectedTextRange.From, req.SelectedTextRange.To)
			if err != nil {
				return blockIds, err
			}

			blockIds, targetId, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Top, true)
			if err != nil {
				return blockIds, err
			}

			if len(newBlocks) > 0 {
				newBlock := newBlocks[0]
				s.Add(newBlock)
				err = s.InsertTo(targetId, model.Block_Bottom, newBlock.Model().Id)
				if err != nil {
					return blockIds, err
				}
				blockIds = append(blockIds, newBlock.Model().Id)
			}

			if len(txt) == 0 && !pasteMultipleBlocksInFocusedTitle {
				s.Unlink(targetId)
			}
		}
		break

	case pasteMultipleBlocksOnSelectedBlocks:
		blockIds, _, err = cb.insertBlocks(s, targetId, req.AnySlot, model.Block_Bottom, false)
		if err != nil {
			return blockIds, err
		}
		for _, selectedBlockId := range req.SelectedBlockIds {
			s.Unlink(selectedBlockId)
		}

		break
	}

	return blockIds, cb.Apply(s)
}

func (cb *clipboard) insertBlocks(s *state.State, targetId string, blocks []*model.Block, pos model.BlockPosition, isReversed bool) (blockIds []string, targetIdOut string, err error) {
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
			return blockIds, targetId, nil
		}

		targetId = newBlock.Model().Id

		for _, childId := range blocks[i].ChildrenIds {
			childBlock := s.Get(childId)
			s.Add(childBlock)

			if err = s.InsertTo(blocks[i].Id, model.Block_Bottom, childId); err != nil {
				return blockIds, targetId, err
			}
		}

	}

	return blockIds, targetId, nil
}
