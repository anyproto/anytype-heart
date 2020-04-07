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
	maxChildrenThreshold = 50
	divSize              = maxChildrenThreshold / 2
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

func isDivLayout(m *model.Block) bool {
	if layout := m.GetLayout(); layout != nil && layout.Style == model.BlockContentLayout_Div {
		return true
	}
	return false
}

func (s *State) normalizeTree() {
	s.checkDividedLists(s.RootId())
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

func (s *State) checkDividedLists(id string) {
	pb := s.Pick(id)
	if pb == nil {
		return
	}
	parent := pb.Model()
	if isDivLayout(parent) {
		if nextDiv := s.getNextDiv(parent.Id); nextDiv != nil {
			nextDivM := nextDiv.Model()
			if len(parent.ChildrenIds) > 0 && len(nextDivM.ChildrenIds) > 0 {
				if !s.canDivide(parent.ChildrenIds[len(parent.ChildrenIds)-1]) && !s.canDivide(nextDivM.ChildrenIds[0]) {
					parent = s.Get(id).Model()
					parent.ChildrenIds = append(parent.ChildrenIds, nextDivM.ChildrenIds...)
					s.Remove(nextDivM.Id)
					s.checkDividedLists(id)
					return
				}
			}
		}
	}
	for _, chId := range parent.ChildrenIds {
		s.checkDividedLists(chId)
	}
}

func (s *State) normalizeTreeBranch(id string, seq *int32) {
	parentB := s.Pick(id)
	if parentB == nil {
		return
	}
	parent := parentB.Model()
	if s.dividedLen(parent.ChildrenIds) > maxChildrenThreshold {
		nextId := s.wrapChildrenToDiv(id, seq)
		s.normalizeTreeBranch(nextId, seq)
		return
	}
	for _, chId := range parent.ChildrenIds {
		s.normalizeTreeBranch(chId, seq)
	}
}

func (s *State) dividedLen(ids []string) (l int) {
	var m int
	for _, id := range ids {
		if s.canDivide(id) {
			l++
		} else {
			if l > m {
				m = l
			}
			l = 0
		}
	}
	if l > m {
		return l
	}
	return m
}

func (s *State) wrapChildrenToDiv(id string, seq *int32) (nextId string) {
	parent := s.Get(id).Model()
	overflow := maxChildrenThreshold - len(parent.ChildrenIds)
	if isDivLayout(parent) {
		nextDiv := s.getNextDiv(id)
		if nextDiv == nil || len(nextDiv.Model().ChildrenIds)+overflow > maxChildrenThreshold {
			nextDiv = s.newDiv(seq)
			s.Add(nextDiv)
			s.InsertTo(parent.Id, model.Block_Bottom, nextDiv.Model().Id)
		}
		for s.divBalance(parent, nextDiv.Model()) {
			parent = nextDiv.Model()
			nextDiv = s.getNextDiv(parent.Id)
			overflow = maxChildrenThreshold - len(parent.ChildrenIds)
			if nextDiv == nil || len(nextDiv.Model().ChildrenIds)+overflow > maxChildrenThreshold {
				nextDiv = s.newDiv(seq)
				s.Add(nextDiv)
				s.InsertTo(parent.Id, model.Block_Bottom, nextDiv.Model().Id)
			}
		}
		return s.PickParentOf(parent.Id).Model().Id
	}

	div := s.newDiv(seq)
	div.Model().ChildrenIds = parent.ChildrenIds
	s.Add(div)
	parent.ChildrenIds = []string{div.Model().Id}
	return parent.Id
}

func (s *State) divBalance(d1, d2 *model.Block) (overflow bool) {
	d1.ChildrenIds = append(d1.ChildrenIds, d2.ChildrenIds...)
	sum := len(d1.ChildrenIds)
	var overflowIds []string
	if sum > maxChildrenThreshold*2 {
		overflow = true
		overflowIds = d1.ChildrenIds[maxChildrenThreshold:]
		d1.ChildrenIds = d1.ChildrenIds[:maxChildrenThreshold]
		sum = maxChildrenThreshold
	}
	div := sum / 2
	for div < sum && !s.canDivide(d1.ChildrenIds[div]) {
		div++
	}
	d2.ChildrenIds = make([]string, sum-div)
	copy(d2.ChildrenIds, d1.ChildrenIds[div:])
	d1.ChildrenIds = d1.ChildrenIds[:div]
	d2.ChildrenIds = append(d2.ChildrenIds, overflowIds...)
	return
}

func (s *State) canDivide(id string) bool {
	if b := s.Pick(id); b != nil {
		if tb := b.Model().GetText(); tb != nil && tb.Style == model.BlockContentText_Numbered {
			return false
		}
	}
	return true
}

func (s *State) newDiv(seq *int32) simple.Block {
	*seq++
	divId := fmt.Sprintf("div-%d", *seq)
	return simple.New(&model.Block{
		Id: divId,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Div},
		},
	})
}

func (s *State) getNextDiv(id string) simple.Block {
	parent := s.PickParentOf(id)
	if parent != nil {
		pm := parent.Model()
		pos := slice.FindPos(pm.ChildrenIds, id)
		if pos != -1 && pos < len(pm.ChildrenIds)-1 {
			if isDivLayout(s.Pick(pm.ChildrenIds[pos+1]).Model()) {
				return s.Get(pm.ChildrenIds[pos+1])
			}
		}
	}
	return nil
}
