package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (p *commonSmart) Move(req pb.RpcBlockListMoveRequest) (err error) {
	p.m.Lock()
	defer p.m.Unlock()

	s := p.newState()

	if err = p.cut(s, req.BlockIds...); err != nil {
		return
	}

	if err = p.insertTo(s, req.DropTargetId, req.Position, req.BlockIds...); err != nil {
		return
	}
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) cut(s *state, ids ...string) (err error) {
	for _, id := range ids {
		if b := s.get(id); b != nil {
			s.removeFromChilds(id)
		} else {
			err = fmt.Errorf("block '%s' not found", id)
			return
		}
	}
	return
}

func (p *commonSmart) moveFromSide(s *state, target simple.Block, pos model.BlockPosition, ids ...string) (err error) {
	row := s.findParentOf(target.Model().Id)
	if row == nil {
		return fmt.Errorf("target block has not parent")
	}
	if row.Model().GetLayout() == nil || row.Model().GetLayout().Style != model.BlockContentLayout_Row {
		if row, err = p.wrapToRow(s, row, target); err != nil {
			return
		}
		target = s.get(row.Model().ChildrenIds[0])
		log.Debug("middle: creating row:", row.Model().Id)
	}
	column, err := s.create(&model.Block{
		ChildrenIds: ids,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	})
	if err != nil {
		return
	}

	targetPos := findPosInSlice(row.Model().ChildrenIds, target.Model().Id)
	if targetPos == -1 {
		return fmt.Errorf("target[%s] is not a child of row[%s]", target.Model().Id, row.Model().Id)
	}

	columnPos := targetPos
	if pos == model.Block_Right {
		columnPos += 1
	}
	row.Model().ChildrenIds = insertToSlice(row.Model().ChildrenIds, column.Model().Id, columnPos)
	return
}

func (p *commonSmart) wrapToRow(s *state, parent, b simple.Block) (row simple.Block, err error) {
	column, err := s.create(&model.Block{
		ChildrenIds: []string{b.Model().Id},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	})
	if err != nil {
		return
	}
	if row, err = s.create(&model.Block{
		ChildrenIds: []string{column.Model().Id},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		},
	}); err != nil {
		return
	}
	pos := findPosInSlice(parent.Model().ChildrenIds, b.Model().Id)
	if pos == -1 {
		return nil, fmt.Errorf("creating row: can't find child[%s] in given parent[%s]", b.Model().Id, parent.Model().Id)
	}
	parent.Model().ChildrenIds[pos] = row.Model().Id
	return
}
