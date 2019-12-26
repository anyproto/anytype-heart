package block

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
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

func (p *commonSmart) pasteHtml(req pb.RpcBlockPasteRequest) error {
	return nil
}

func (p *commonSmart) pasteText(req pb.RpcBlockPasteRequest) error {
	/*
		Вставляем текст. Если текст был скопирован на клиенте и нажата команда вставки, то вставка будет происходить на клиенте.
		Соответственно, если команда пришла, значит текст был скопирован извне. Текстовый слот имеет самый низкий приоритет,
		он используется только если any и html слоты пустые.

		1. Если есть блок в фокусе и выделен текст, сперва удаляем текст, а затем:
			1а. Вставляем на место этого текста текст из слота
			1b. Сплитим блок и посередине вставляем массив блоков, которые были созданы из текста.
		2. Если выделение... TODO
	*/
	return nil
}

func (p *commonSmart) pasteAny(req pb.RpcBlockPasteRequest) error {
	var (
		targetId string
	)

	s := p.newState()

	/*strArrEq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}

		for i, v := range a {
			if v != b[i] {
				return false
			}
		}
		return true
	}*/

	if len(req.SelectedBlockIds) > 0 {
		targetId = req.SelectedBlockIds[len(req.SelectedBlockIds)-1]

		// selected text -> remove it and split the block
	} else if len(req.FocusedBlockId) > 0 { // && req.SelectedTextRange.From > 0 && req.SelectedTextRange.To > req.SelectedTextRange.From

		// split block

		newBlockId, err := p.rangeSplit(s, req.FocusedBlockId, req.SelectedTextRange.From, req.SelectedTextRange.To)
		if err != nil {
			return err
		}
		newBlockId = newBlockId;
		targetId = req.FocusedBlockId


		//t, err := s.getText(newBlockId);
		//if err !=nil {
		//	return err
		//}
		//if len(t.GetText()) == 0 {
		//	if err := p.unlink(s, newBlockId); err != nil {
		//		return err
		//	}
		//}

	//} else if len(req.FocusedBlockId) > 0 &&
	//	// TODO: or (req.SelectedTextRange.From == len(blockText) && req.SelectedTextRange.To == len(blockText))
	//	(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0) {

		// No focus -> check last
	} else {
		cIds := p.versions[p.GetId()].Model().ChildrenIds

		if len(cIds) > 0 {
			targetId = cIds[len(cIds)-1]
		}
	}

	err := p.pasteBlocks(s, req, targetId)

	t, err := s.getText(targetId);
	if err !=nil {
		return err
	}
	if len(t.GetText()) == 0 {
		if err := p.unlink(s, targetId); err != nil {
			return err
		}
	}

	if err != nil {
		return err
	}

	// selected blocks -> remove it
	if len(req.SelectedBlockIds) > 0 {
			if err := p.unlink(s, req.SelectedBlockIds...); err != nil {
				return err
			}
	}

	cIds := p.versions[p.GetId()].Model().ChildrenIds
	for i := 0; i < len(cIds); i++ {
		//fmt.Println(">>> Text", p.versions[cIds[i]].Model().GetText().Text)
		if len(p.versions[cIds[i]].Model().GetText().Text) == 0 {
			if err := p.unlink(s, cIds[i]); err != nil {
				return err
			}
		}
	}

	return p.applyAndSendEvent(s)
}
