package block

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (p *commonSmart) Paste(req pb.RpcBlockPasteRequest) error {

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

func (p *commonSmart) pasteHtml(req pb.RpcBlockPasteRequest) error {
	return nil
}

func (p *commonSmart) pasteText(req pb.RpcBlockPasteRequest) error {
	return nil
}

func (p *commonSmart) pasteAny(req pb.RpcBlockPasteRequest) error {
	var (
		targetId string
	)

	s := p.newState()
	blockIds := req.AnySlot

	// selected blocks -> remove it
	if len(req.SelectedBlockIds) > 0 {
		if err := p.unlink(s, req.SelectedBlockIds...); err != nil {
			return err
		}

		// selected text -> remove it and split the block
	} else if len(req.FocusedBlockId) > 0 && req.SelectedTextRange.From > 0 {
		// TODO: remove text in range

		// split block
		if _, err := p.Split(req.FocusedBlockId, req.SelectedTextRange.From); err != nil {
			return err
		}

		targetId = req.FocusedBlockId

	} else if len(req.FocusedBlockId) > 0 &&
		// TODO: or (req.SelectedTextRange.From == len(blockText) && req.SelectedTextRange.To == len(blockText))
		(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0) {

	} else {
		cIds := p.versions[p.GetId()].Model().ChildrenIds
		targetId = cIds[len(cIds)-1]
	}

	targetId = req.FocusedBlockId

	for i := 0; i < len(blockIds); i++ {
		id, err := p.Duplicate(pb.RpcBlockDuplicateRequest{
			ContextId: req.ContextId,
			TargetId:  targetId,
			BlockId:   blockIds[i],
			Position:  model.Block_Bottom,
		})

		if err != nil {
			return err
		}

		targetId = id
	}

	return p.applyAndSendEvent(s)
}
