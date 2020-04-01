package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var (
	maxChildrenThreshold = 10
)

func (s *State) normalize() {
	// remove invalid children
	for _, b := range s.blocks {
		s.normalizeChildren(b)
	}
	// remove empty layouts
	for _, b := range s.blocks {
		if layout := b.Model().GetLayout(); layout != nil {
			if len(b.Model().ChildrenIds) == 0 {
				s.Remove(b.Model().Id)
			}
			// load parent for checking
			s.GetParentOf(b.Model().Id)
		}
	}
	// normalize rows
	for _, b := range s.blocks {
		if layout := b.Model().GetLayout(); layout != nil {
			s.normalizeLayoutRow(b)
		}
	}
	s.normalizeTree()
	return
}

func (s *State) normalizeChildren(b simple.Block) {
	m := b.Model()
	for _, cid := range m.ChildrenIds {
		if !s.Exists(cid) {
			m.ChildrenIds = slice.Remove(m.ChildrenIds, cid)
			s.normalizeChildren(b)
			return
		}
	}
}

func (s *State) normalizeLayoutRow(b simple.Block) {
	if b.Model().GetLayout().Style != model.BlockContentLayout_Row {
		return
	}
	// remove empty row
	if len(b.Model().ChildrenIds) == 0 {
		s.Remove(b.Model().Id)
		return
	}
	// one column - remove row
	if len(b.Model().ChildrenIds) == 1 {
		var (
			contentIds   []string
			removeColumn bool
		)
		column := s.Get(b.Model().ChildrenIds[0])
		if layout := column.Model().GetLayout(); layout != nil && layout.Style == model.BlockContentLayout_Column {
			contentIds = column.Model().ChildrenIds
			removeColumn = true
		} else {
			contentIds = append(contentIds, column.Model().Id)
		}
		if parent := s.GetParentOf(b.Model().Id); parent != nil {
			rowPos := slice.FindPos(parent.Model().ChildrenIds, b.Model().Id)
			if rowPos != -1 {
				parent.Model().ChildrenIds = slice.Remove(parent.Model().ChildrenIds, b.Model().Id)
				for _, id := range contentIds {
					parent.Model().ChildrenIds = slice.Insert(parent.Model().ChildrenIds, id, rowPos)
					rowPos++
				}
				if removeColumn {
					s.Remove(column.Model().Id)
				}
				s.Remove(b.Model().Id)
			}
		}
		return
	}

	// reset columns width when count of row children was changed
	orig := s.PickOrigin(b.Model().Id)
	if orig != nil && len(orig.Model().ChildrenIds) != len(b.Model().ChildrenIds) {
		for _, chId := range b.Model().ChildrenIds {
			fields := s.Get(chId).Model().Fields
			if fields != nil && fields.Fields != nil && fields.Fields["width"] != nil {
				fields.Fields["width"] = pbtypes.Float64(0)
			}
		}
	}
}

func (s *State) validateBlock(b simple.Block) (err error) {
	id := b.Model().Id
	if id == s.RootId() {
		return
	}
	var parentIds = []string{id}
	for {
		parent := s.PickParentOf(id)
		if parent == nil {
			break
		}
		if parent.Model().Id == s.RootId() {
			return nil
		}
		if slice.FindPos(parentIds, parent.Model().Id) != -1 {
			return fmt.Errorf("cycle reference: %v", append(parentIds, parent.Model().Id))
		}
		id = parent.Model().Id
		parentIds = append(parentIds, id)
	}
	return fmt.Errorf("block '%s' has not the page in parents", id)
}

func divId(id int32) string {
	return fmt.Sprintf("div-%d", id)
}

func isDivLayout(m *model.Block) bool {
	if layout := m.GetLayout(); layout != nil && layout.Style == model.BlockContentLayout_Div {
		return true
	}
	return false
}

func (s *State) normalizeTree() {
	var seq int32
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if isDivLayout(b.Model()) {
			id := b.Model().Id
			if strings.HasPrefix(id, "div-") {
				res, _ := strconv.Atoi(id[len("div-"):])
				if int32(res) > seq {
					seq = int32(res)
				}
			}
		}
		return true
	})
	s.normalizeTreeBranch(s.RootId(), &seq)
}

func (s *State) normalizeTreeBranch(id string, seq *int32) {
	if id == "" {
		return
	}
	parentB := s.Pick(id)
	if parentB == nil {
		return
	}
	parent := parentB.Model()
	if len(parent.ChildrenIds) > maxChildrenThreshold {
		nextId := s.wrapChildrenToDiv(id, seq)
		s.normalizeTreeBranch(nextId, seq)
		return
	}
	for _, chId := range parent.ChildrenIds {
		s.normalizeTreeBranch(chId, seq)
	}
}

func (s *State) wrapChildrenToDiv(id string, seq *int32) (nextId string) {
	parent := s.Get(id).Model()
	var (
		moveIds   []string
		targetId  string
		targetPos model.BlockPosition
	)

	if isDivLayout(parent) { // div overflow - move outer and rescan parent
		moveIds = parent.ChildrenIds[maxChildrenThreshold:]
		targetId = parent.Id
		targetPos = model.Block_Bottom
		if pp := s.GetParentOf(parent.Id); pp != nil {
			nextId = pp.Model().Id
			for _, mId := range moveIds {
				s.Unlink(mId)
			}
			if err := s.InsertTo(targetId, targetPos, moveIds...); err != nil {
				log.Warnf("normalize: wrapChildrenToDiv: insertTo error: %v", err)
			}
		}
		return
	}

	for len(parent.ChildrenIds) > maxChildrenThreshold {
		*seq++
		moveIds = make([]string, maxChildrenThreshold)
		copy(moveIds, parent.ChildrenIds[:maxChildrenThreshold])
		divId := fmt.Sprintf("div-%d", *seq)
		parent.ChildrenIds = append([]string{divId}, parent.ChildrenIds[maxChildrenThreshold:]...)
		div := simple.New(&model.Block{
			Id: divId,
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Div},
			},
			ChildrenIds: moveIds,
		})
		s.Add(div)
	}
	return
}
