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
	log = logging.Logger("anytype-service")
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
		log.Debug(err)
		return "", err
	}
	log.Debug("filepath.Join(dir, fileName)", filepath.Join(dir, fileName), dir, fileName)
	log.Debug(html)

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

	log.Debug("BLOCKS text:", req.AnySlot)

	blockIds, err = cb.pasteAny(req)
	log.Error("ERROR pasteAny:", err)
	return blockIds, err

}

func (cb *clipboard) pasteAny(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	s := cb.NewState()

	targetId := req.FocusedBlockId

	//b :=
	//fmt.Println("b:", b, "cb:", cb, cb.Id(), cb.RootId())
	children := cb.Blocks()

	reqFiltered := []*model.Block{}
	for i := 0; i < len(req.AnySlot); i++ {
		switch req.AnySlot[i].Content.(type) {
		case *model.BlockContentOfLayout:
			continue
		default:
			reqFiltered = append(reqFiltered, req.AnySlot[i])
		}
	}

	fmt.Println("HERE2", req.AnySlot)
	req.AnySlot = reqFiltered
	fmt.Println("HERE3", req.AnySlot)

	var getPrevBlockId = func(id string) string {
		var out string
		var prev string

		children := cb.Blocks()

		for _, c := range children {
			out = prev
			if c.Id == id {
				return out
			}
			prev = c.Id
		}
		return out
	}

	// ---- SPECIAL CASE: paste in title ----
	titlePasted := false
	b := cb.Pick(req.FocusedBlockId)
	if len(req.FocusedBlockId) > 0 && b != nil {
		if contentTitle, ok := b.Model().Content.(*model.BlockContentOfText); ok &&
			len(req.AnySlot) > 0 {
			if contentPaste, ok := req.AnySlot[0].Content.(*model.BlockContentOfText); ok {
				if contentTitle.Text.Style == model.BlockContentText_Title {

					contentPaste.Text.Text = strings.Replace(contentPaste.Text.Text, "\n", " ", -1)
					contentPaste.Text.Marks = &model.BlockContentTextMarks{}

					block := s.Get(b.Model().Id)
					if block == nil {
						return nil, smartblock.ErrSimpleBlockNotFound
					}
					tb, ok := block.(text.Block)
					if !ok {
						return nil, smartblock.ErrSimpleBlockNotFound
					}

					err = tb.RangeTextPaste(req.SelectedTextRange.From, req.SelectedTextRange.To, contentPaste.Text.Text, contentPaste.Text.Marks.Marks)
					if err != nil {
						return blockIds, err
					}

					titlePasted = true

					if len(req.AnySlot) == 1 {
						return blockIds, cb.Apply(s)
					} else {
						req.AnySlot = req.AnySlot[1:]

						var getNextBlockId = func(id string) string {
							var out string
							var isNext = false

							b := cb.Pick(cb.Id())
							cIds := b.Model().ChildrenIds

							for _, i := range cIds {
								if isNext {
									out = i
									isNext = false
								}

								if i == id {
									isNext = true
								}
							}
							return out
						}

						log.Debug("NEXT:", getNextBlockId(req.FocusedBlockId))
						req.SelectedTextRange.From = 0
						req.SelectedTextRange.To = 0

						blockIds, err = cb.pasteBlocks(req.AnySlot, s, req.FocusedBlockId)
						if err != nil {
							return blockIds, err
						}

						return blockIds, cb.Apply(s)
					}
				}
			}
		}
	}

	// ---- SPECIAL CASE: paste text without new blocks creation ----
	// If there is 1 block and it is a text =>
	// if there is a focused block => Do not create new blocks
	// If selectedBlocks => ignore it, it is an error
	b = cb.Pick(req.FocusedBlockId)
	if content, ok := req.AnySlot[0].Content.(*model.BlockContentOfText); ok &&
		len(req.AnySlot) == 1 &&
		len(req.FocusedBlockId) > 0 &&
		!titlePasted {

		if req.SelectedTextRange == nil {
			req.SelectedTextRange = &model.Range{From: 0, To: 0}
		}

		if content.Text.Marks == nil {
			content.Text.Marks = &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{}}
		}

		block := s.Get(b.Model().Id)
		if block == nil {
			return nil, smartblock.ErrSimpleBlockNotFound
		}
		tb, ok := block.(text.Block)
		if !ok {
			return nil, smartblock.ErrSimpleBlockNotFound
		}
		fmt.Println("HERE5", req.AnySlot)
		if err := tb.RangeTextPaste(req.SelectedTextRange.From, req.SelectedTextRange.To, content.Text.Text, content.Text.Marks.Marks); err != nil {
			return blockIds, err
		}

		return blockIds, cb.Apply(s)
	}

	fmt.Println(">>> target", targetId)
	if len(req.SelectedBlockIds) > 0 {
		targetId = req.SelectedBlockIds[len(req.SelectedBlockIds)-1]
		fmt.Println(">>> target2", targetId)
		// selected text -> remove it and split the block
	} else if len(req.FocusedBlockId) > 0 && len(req.AnySlot) > 1 {
		fmt.Println(">>> target3", targetId)
		fmt.Println("HERE4", req.AnySlot)
		if req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 {
			fmt.Println("HERE4", req.AnySlot)
			blockIds, err = cb.pasteBlocks(req.AnySlot, s, req.FocusedBlockId)

			fmt.Println(">>> target4", req.AnySlot, req.FocusedBlockId, blockIds, err)

			if err != nil {
				return blockIds, err
			} else {
				return blockIds, cb.Apply(s)
			}
		}

		// split block
		block := s.Get(b.Model().Id)
		fmt.Println("HERE5", req.AnySlot)
		if block == nil {
			return nil, smartblock.ErrSimpleBlockNotFound
		}

		tb, ok := block.(text.Block)
		if !ok {
			return nil, smartblock.ErrSimpleBlockNotFound
		}
		fmt.Println("HERE6", req.AnySlot)
		newBlocks, txt, err := tb.RangeSplit(req.SelectedTextRange.From, req.SelectedTextRange.To)

		if len(txt) == 0 {
			s.Unlink(b.Model().Id)
		}

		fmt.Println("HERE61", req.AnySlot)

		if len(newBlocks) == 0 {
			return blockIds, cb.Apply(s)
		}
		fmt.Println("HERE62", req.AnySlot)
		sb := simple.New(newBlocks[0].Model())
		s.Add(sb)
		fmt.Println(">>>>>>", targetId, model.Block_Bottom, sb.Model().Id)
		if err = s.InsertTo(req.FocusedBlockId, model.Block_Bottom, sb.Model().Id); err != nil { // TODO: req.FocusedBlockId -> targetId
			fmt.Println("HERE621", req.AnySlot, err)
			return blockIds, err
		}
		fmt.Println("HERE63", req.AnySlot)
		targetId = req.FocusedBlockId

		// if cursor at (0,0) – paste top
		if req.SelectedTextRange.From == 0 {
			targetId = getPrevBlockId(req.FocusedBlockId)
		}

	} else {
		if len(children) > 0 {
			targetId = children[len(children)-1].Id
		}
	}
	fmt.Println("HERE7", req.AnySlot)
	blockIds, err = cb.pasteBlocks(req.AnySlot, s, targetId)
	if err != nil {
		return blockIds, err
	}
	fmt.Println("HERE8", req.AnySlot)
	// selected blocks -> remove it
	if len(req.SelectedBlockIds) > 0 {
		for _, id := range req.SelectedBlockIds {
			s.Unlink(id)
		}
	}

	return blockIds, cb.Apply(s)
}

func (cb *clipboard) pasteBlocks(blocksToPaste []*model.Block, s *state.State, targetId string) (blockIds []string, err error) {
	//parent := s.Pick(cb.Id()).Model()
	//s := cb.NewState()
	parent := s.Get(cb.RootId()).Copy().Model()

	//fmt.Println("TARGET_ID", targetId)
	//
	emptyPage := false
	//b := simple.New(&model.Block{Id: "XXX"})
	//s.Add(b)
	//s.InsertTo(targetId, model.Block_Bottom, b.Model().Id)
	//
	//s.Append("test", b.Model().Id)
	//parent.ChildrenIds = append(parent.ChildrenIds, b.Model().Id)
	//s.Set(parent)

	//
	blockIds = []string{}
	blocks := []simple.Block{}
	//
	if len(parent.ChildrenIds) == 0 {
		emptyPage = true
	}
	//
	for i := 0; i < len(blocksToPaste); i++ {
		pasteBlock := simple.New(blocksToPaste[i])


		pasteBlockId := pasteBlock.Model().Id
		blockIds = append(blockIds, pasteBlockId)
		blocks = append(blocks, pasteBlock)

		s.Set(pasteBlock)
		if err = s.InsertTo(targetId, model.Block_Bottom, pasteBlockId); err != nil {
			return blockIds, err
		}

		// if file -> upload
		// TODO: copy of file? Discuss it
		/*		if fb, ok := pasteBlock.(file.Block); ok {
				if err = fb.Upload(cb.Anytype(), nil, "", ""); err != nil {
					return blockIds, err
				}
		}*/

		for _, childId := range pasteBlock.Model().ChildrenIds {
			childBlock := s.Get(childId)
			s.Add(childBlock)

			if err = s.InsertTo(targetId, model.Block_Bottom, childId); err != nil {
				return blockIds, err
			}
		}

		if emptyPage {
			parent.ChildrenIds = append(parent.ChildrenIds, pasteBlockId)
		} else {
			if err = s.InsertTo(targetId, model.Block_Bottom, pasteBlockId); err != nil {
				return blockIds, err
			}
			targetId = pasteBlockId
		}
	}

	return blockIds, nil
}
