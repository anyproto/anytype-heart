package block

import "C"
import (
	"errors"
	"fmt"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"strings"
)
var (
	ErrAllSlotsEmpty = errors.New("All slots are empty")
)

func (p *commonSmart) Paste(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	p.m.Lock()
	defer p.m.Unlock()

	if len(req.AnySlot) > 0 {
		return p.pasteAny(req)
	} else if len(req.HtmlSlot) > 0 {
		blockIds, err = p.pasteHtml(req)

		if err != nil {
			return p.pasteText(req)
		} else {
			return blockIds, err
		}

	} else if len(req.TextSlot) > 0 {
		return p.pasteText(req)
	} else {
		return nil, ErrAllSlotsEmpty
	}
}

func (p *commonSmart) Copy(req pb.RpcBlockCopyRequest, images map[string][]byte) (html string, err error) {
	conv := converter.New()
	return conv.Convert(req.Blocks, images), nil
}

func (p *commonSmart) Cut(req pb.RpcBlockCutRequest, images map[string][]byte) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	s := p.newState()
	textSlot = ""
	for _, b := range req.Blocks {
		switch text := b.Content.(type) {
		case *model.BlockContentOfText:
			textSlot += text.Text.Text + "\n"
		}
	}
	conv := converter.New()
	htmlSlot = conv.Convert(req.Blocks, images)
	anySlot = req.Blocks

	var ids []string

	for _, b := range req.Blocks {
	 	ids = append(ids, b.Id)
	}
	if len(ids) > 0 {
		if err := p.unlink(s, ids...); err != nil {
			return textSlot, htmlSlot, anySlot, err
		}
	}

	return textSlot, htmlSlot, anySlot, p.applyAndSendEvent(s)
}

func (p *commonSmart) pasteHtml(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	mdToBlocksConverter := anymark.New()
	_, blocks := mdToBlocksConverter.HTMLToBlocks([]byte(req.HtmlSlot))

	req.AnySlot = blocks
	return p.pasteAny(req)
}

func (p *commonSmart) pasteText(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	if len(req.TextSlot) == 0 {
		return blockIds, nil
	}

	textArr := strings.Split(req.TextSlot, "\n")

	if len(req.FocusedBlockId) > 0 {
		block := p.versions[req.FocusedBlockId].Model()
		switch block.Content.(type) {
		case *model.BlockContentOfText:
			if block.GetText().Style == model.BlockContentText_Code {
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

	blockIds, err = p.pasteAny(req)
	fmt.Println("ERROR pasteAny:", err)
	return blockIds, err

}

func (p *commonSmart) pasteAny(req pb.RpcBlockPasteRequest) (blockIds []string, err error) {

	var (
		targetId string
	)

	s := p.newState()

	cIds := p.versions[p.GetId()].Model().ChildrenIds

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
		cIds = p.versions[p.GetId()].Model().ChildrenIds
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
	if len(req.FocusedBlockId) > 0 && p.versions[req.FocusedBlockId] != nil {
		if contentTitle, ok := p.versions[req.FocusedBlockId].Model().Content.(*model.BlockContentOfText); ok &&
			len(req.AnySlot) > 0 {
			if contentPaste, ok := req.AnySlot[0].Content.(*model.BlockContentOfText); ok {
				if contentTitle.Text.Style == model.BlockContentText_Title {

					contentPaste.Text.Text = strings.Replace(contentPaste.Text.Text, "\n", " ", -1)
					contentPaste.Text.Marks = &model.BlockContentTextMarks{}
					err = p.rangeTextPaste(s, req.FocusedBlockId, req.SelectedTextRange.From, req.SelectedTextRange.To, contentPaste.Text.Text, contentPaste.Text.Marks.Marks)
					if err != nil {
						return blockIds, err
					}

					titlePasted = true

					if len(req.AnySlot) == 1 {
						return blockIds, p.applyAndSendEvent(s)
					} else {
						req.AnySlot = req.AnySlot[1:]

						var getNextBlockId = func(id string) string {
							var out string
							var isNext = false
							cIds = p.versions[p.GetId()].Model().ChildrenIds
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

						fmt.Println("NEXT:", getNextBlockId(req.FocusedBlockId))
						req.SelectedTextRange.From = 0
						req.SelectedTextRange.To = 0
						blockIds, err = p.pasteBlocks(s, req, req.FocusedBlockId)
						if err != nil {
							return blockIds, err
						}

						return blockIds, p.applyAndSendEvent(s)
					}
				}
			}
		}
	}

	// ---- SPECIAL CASE: paste text without new blocks creation ----
	// If there is 1 block and it is a text =>
	// if there is a focused block => Do not create new blocks
	// If selectedBlocks => ignore it, it is an error
	if  content, ok := req.AnySlot[0].Content.(*model.BlockContentOfText); ok &&
		len(req.AnySlot) == 1 &&
		len(req.FocusedBlockId) > 0 && p.versions[req.FocusedBlockId] != nil &&
		!titlePasted {

		if req.SelectedTextRange == nil {
			req.SelectedTextRange = &model.Range{From:0, To:0}
		}

		if content.Text.Marks == nil {
			content.Text.Marks = &model.BlockContentTextMarks{Marks:[]*model.BlockContentTextMark{}}
		}

		err = p.rangeTextPaste(s,
			req.FocusedBlockId,
			req.SelectedTextRange.From,
			req.SelectedTextRange.To,
			content.Text.Text,
			content.Text.Marks.Marks)
		if err != nil {
			return blockIds, err
		}

		return blockIds, p.applyAndSendEvent(s)
	}

	if len(req.SelectedBlockIds) > 0 {
		targetId = req.SelectedBlockIds[len(req.SelectedBlockIds)-1]

		// selected text -> remove it and split the block
	} else if len(req.FocusedBlockId) > 0 && len(req.AnySlot) > 1 {

		if req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 {
			blockIds, err = p.pasteBlocks(s, req, req.FocusedBlockId)
			if err != nil {
				return blockIds, err
			} else {
				return blockIds, p.applyAndSendEvent(s)
			}
		}

		// split block
		_, err := p.rangeSplit(s, req.FocusedBlockId, req.SelectedTextRange.From, req.SelectedTextRange.To)
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

	blockIds, err = p.pasteBlocks(s, req, targetId)
	if err != nil {
		return blockIds, err
	}

	// selected blocks -> remove it
	if len(req.SelectedBlockIds) > 0 {
		if err := p.unlink(s, req.SelectedBlockIds...); err != nil {
			return blockIds, err
		}
	}

	return blockIds, p.applyAndSendEvent(s)
}
