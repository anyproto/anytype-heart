package clipboard

import (
	"errors"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	ErrAllSlotsEmpty = errors.New("All slots are empty")
)

type Clipboard interface {
	Cut(req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Paste(req pb.RpcBlockPasteRequest) (blockIds []string, err error)
	Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (html string, err error)
	Export(req pb.RpcBlockExportRequest, images map[string][]byte) (path string, err error)
}

/*
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

	// TODO: unlink
	/*
	if len(ids) > 0 {
		if err := p.unlink(s, ids...); err != nil {
			return textSlot, htmlSlot, anySlot, err
		}
	}*/

	if err != nil {
		return textSlot, htmlSlot, anySlot, err
	}

	conv := converter.New()
	htmlSlot = conv.Convert(req.Blocks, images)
	anySlot = req.Blocks

	// TODO: is it OK to Apply in the middle of CutTo Function?
	return textSlot, htmlSlot, anySlot,  cb.Apply(s)

}

func (cb *clipboard) blocksTreeToMap (blocksMapIn map[string]*model.Block, ids []string) (blocksMapOut map[string]*model.Block) {
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

func (cb *clipboard) getImages (blocks map[string]*model.Block) (images map[string][]byte, err error) {
	for _, b := range blocks {
		if file := b.GetFile(); file != nil {
			if file.Type == model.BlockContentFile_Image {
				fh, err := cb.Anytype().FileByHash(file.Hash)
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
	err = ioutil.WriteFile(filePath, []byte(html),0644)

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

	req.AnySlot = blocks
	return cb.pasteAny(req)
}

func (cb *clipboard) pasteText(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	if len(req.TextSlot) == 0 {
		return blockIds, nil
	}

	textArr := strings.Split(req.TextSlot, "\n")

	if len(req.FocusedBlockId) > 0 {
		block, err := cb.GetBlock(req.FocusedBlockId)
		if err == nil {
			if b := block.GetText(); b != nil && b.Style == model.BlockContentText_Code {
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

	var targetId string

	b, _ :=  cb.GetBlock(cb.Id())
	cIds := b.ChildrenIds

	reqFiltered := []*model.Block{}
	for i:=0; i < len(req.AnySlot); i++ {
		switch req.AnySlot[i].Content.(type) {
		case *model.BlockContentOfLayout: continue
		default: reqFiltered = append(reqFiltered, req.AnySlot[i])
		}
	}

	req.AnySlot = reqFiltered

	var getPrevBlockId = func(id string) string {
		var out string
		var prev string

		b, _ :=  cb.GetBlock(cb.Id())
		cIds := b.ChildrenIds

		for _, i := range cIds {
			out = prev
			if i == id {
				return out
			}
			prev = i
		}
		return out
	}

	// ---- SPECIAL CASE: paste in title ----
	titlePasted := false
	b, err = cb.GetBlock(req.FocusedBlockId)
	if len(req.FocusedBlockId) > 0 &&  err != nil {
		if contentTitle, ok := b.Content.(*model.BlockContentOfText); ok &&
			len(req.AnySlot) > 0 {
			if contentPaste, ok := req.AnySlot[0].Content.(*model.BlockContentOfText); ok {
				if contentTitle.Text.Style == model.BlockContentText_Title {

					contentPaste.Text.Text = strings.Replace(contentPaste.Text.Text, "\n", " ", -1)
					contentPaste.Text.Marks = &model.BlockContentTextMarks{}
					// TODO: rangeTextPaste
					// err = p.rangeTextPaste(s, req.FocusedBlockId, req.SelectedTextRange.From, req.SelectedTextRange.To, contentPaste.Text.Text, contentPaste.Text.Marks.Marks)
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

							b, _ :=  cb.GetBlock(cb.Id())
							cIds := b.ChildrenIds

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

						// TODO: pasteBlocks
						// blockIds, err = p.pasteBlocks(s, req, req.FocusedBlockId)
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
	b, err = cb.GetBlock(req.FocusedBlockId)
	if  content, ok := req.AnySlot[0].Content.(*model.BlockContentOfText); ok &&
		len(req.AnySlot) == 1 &&
		len(req.FocusedBlockId) > 0 && err != nil &&
		!titlePasted {

		if req.SelectedTextRange == nil {
			req.SelectedTextRange = &model.Range{From:0, To:0}
		}

		if content.Text.Marks == nil {
			content.Text.Marks = &model.BlockContentTextMarks{Marks:[]*model.BlockContentTextMark{}}
		}

		// TODO: rangeTextPaste
/*		err = p.rangeTextPaste(s,
			req.FocusedBlockId,
			req.SelectedTextRange.From,
			req.SelectedTextRange.To,
			content.Text.Text,
			content.Text.Marks.Marks)
		if err != nil {
			return blockIds, err
		}
*/
		return blockIds, cb.Apply(s)
	}

	if len(req.SelectedBlockIds) > 0 {
		targetId = req.SelectedBlockIds[len(req.SelectedBlockIds)-1]

		// selected text -> remove it and split the block
	} else if len(req.FocusedBlockId) > 0 && len(req.AnySlot) > 1 {

		if req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 {
			// TODO: pasteBlocks
			//blockIds, err = p.pasteBlocks(s, req, req.FocusedBlockId)
			if err != nil {
				return blockIds, err
			} else {
				return blockIds, cb.Apply(s)
			}
		}

		// split block
		// TODO: rangeSplit
		//_, err := p.rangeSplit(s, req.FocusedBlockId, req.SelectedTextRange.From, req.SelectedTextRange.To)
		if err != nil {
			return blockIds, err
		}
		targetId = req.FocusedBlockId

		// if cursor at (0,0) – paste top
		if req.SelectedTextRange.From == 0 {
			targetId = getPrevBlockId(req.FocusedBlockId)
		}

	} else {
		if len(cIds) > 0 {
			targetId = cIds[len(cIds)-1]
		}
	}

	// TODO: pasteBlocks
	//blockIds, err = p.pasteBlocks(s, req, targetId)
	if err != nil {
		return blockIds, err
	}

	// selected blocks -> remove it
	if len(req.SelectedBlockIds) > 0 {
		// TODO: unlink
		//if err := p.unlink(s, req.SelectedBlockIds...); err != nil {
		//	return blockIds, err
		//}
	}

	return blockIds, cb.Apply(s)
}
*/
