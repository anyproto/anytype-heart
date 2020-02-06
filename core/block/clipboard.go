package block

import (
	"fmt"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"strings"
)

func (p *commonSmart) Paste(req pb.RpcBlockPasteRequest) error {
	p.m.Lock()
	defer p.m.Unlock()

	if len(req.AnySlot) > 0 {
		return p.pasteAny(req)
	} else if len(req.HtmlSlot) > 0 {
		return p.pasteHtml(req)
	} else if len(req.TextSlot) > 0 {
		return p.pasteText(req)
	} else {
		return nil
	}
}

func (p *commonSmart) Copy(req pb.RpcBlockCopyRequest) (html string, err error) {
	return converter.BlocksToHtml(req.Blocks), nil
}

func (p *commonSmart) pasteHtml(req pb.RpcBlockPasteRequest) error {
	mdToBlocksConverter := anymark.New()
	_, blocks := mdToBlocksConverter.HTMLToBlocks([]byte(req.HtmlSlot))

	req.AnySlot = blocks
	return p.pasteAny(req)
}

func (p *commonSmart) pasteText(req pb.RpcBlockPasteRequest) error {
	if len(req.TextSlot) == 0 {
		return nil
	}

	textArr := strings.Split(req.TextSlot, "\n")

	block := p.versions[req.FocusedBlockId].Model()
	switch block.Content.(type) {
	case *model.BlockContentOfText:
		if block.GetText().Style == model.BlockContentText_Code {
			textArr = []string{req.TextSlot}
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

	err := p.pasteAny(req)
	fmt.Println("ERROR pasteAny:", err)
	return err

}

func (p *commonSmart) pasteAny(req pb.RpcBlockPasteRequest) error {

	var (
		targetId string
	)

	s := p.newState()

	cIds := p.versions[p.GetId()].Model().ChildrenIds

	if len(req.SelectedBlockIds) > 0 {
		targetId = req.SelectedBlockIds[len(req.SelectedBlockIds)-1]

		// selected text -> remove it and split the block
	} else if len(req.FocusedBlockId) > 0 {

		// split block
		_, err := p.rangeSplit(s, req.FocusedBlockId, req.SelectedTextRange.From, req.SelectedTextRange.To)
		if err != nil {
			return err
		}
		targetId = req.FocusedBlockId

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

		// if cursor at (0,0) – paste top
		if req.SelectedTextRange.From == 0 {
			targetId = getPrevBlockId(req.FocusedBlockId)
		}

	} else {
		if len(cIds) > 0 {
			targetId = cIds[len(cIds)-1]
		}
	}

	err := p.pasteBlocks(s, req, targetId)
	if err != nil {
		return err
	}

	// selected blocks -> remove it
	if len(req.SelectedBlockIds) > 0 {
		if err := p.unlink(s, req.SelectedBlockIds...); err != nil {
			return err
		}
	}

	return p.applyAndSendEvent(s)
}
