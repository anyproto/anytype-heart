package state

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var (
	maxChildrenThreshold = 40
	divSize              = maxChildrenThreshold / 2
)

func (s *State) Normalize(withLayouts bool) (err error) {
	s.removeDuplicates()
	return s.normalize(withLayouts)
}

func (s *State) normalize(withLayouts bool) (err error) {
	// remove invalid children
	for _, b := range s.blocks {
		s.normalizeChildren(b)
	}
	// remove empty layouts
	for _, b := range s.blocks {
		if layout := b.Model().GetLayout(); layout != nil {
			if len(b.Model().ChildrenIds) == 0 {
				s.Unlink(b.Model().Id)
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
	if withLayouts {
		return s.normalizeTree()
	}
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
	// remove empty layout
	if len(b.Model().ChildrenIds) == 0 {
		s.Unlink(b.Model().Id)
		return
	}
	if b.Model().GetLayout().Style != model.BlockContentLayout_Row {
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
		s.InsertTo(b.Model().Id, model.Block_Replace, contentIds...)
		if removeColumn {
			s.Unlink(column.Model().Id)
		}
		s.Unlink(b.Model().Id)
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

func isDivLayout(m *model.Block) bool {
	if layout := m.GetLayout(); layout != nil && layout.Style == model.BlockContentLayout_Div {
		return true
	}
	return false
}

func (s *State) normalizeTree() (err error) {
	var seq int32
	err = s.Iterate(func(b simple.Block) (isContinue bool) {
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
	if err != nil {
		return
	}
	s.checkDividedLists(s.RootId())
	s.normalizeTreeBranch(s.RootId(), &seq)
	return nil
}

func (s *State) checkDividedLists(id string) {
	pb := s.Pick(id)
	if pb == nil {
		return
	}
	parent := pb.Model()
	if isDivLayout(parent) {
		if nextDiv := s.pickNextDiv(parent.Id); nextDiv != nil {
			nextDivM := nextDiv.Model()
			if len(parent.ChildrenIds) > 0 && len(nextDivM.ChildrenIds) > 0 {
				if !s.canDivide(parent.ChildrenIds[len(parent.ChildrenIds)-1]) && !s.canDivide(nextDivM.ChildrenIds[0]) {
					parent = s.Get(id).Model()
					parent.ChildrenIds = append(parent.ChildrenIds, nextDivM.ChildrenIds...)
					s.Unlink(nextDivM.Id)
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
		if nextId := s.wrapChildrenToDiv(id, seq); nextId != "" {
			s.normalizeTreeBranch(nextId, seq)
			return
		}
	}
	for _, chId := range parent.ChildrenIds {
		s.normalizeTreeBranch(chId, seq)
	}
}

func (s *State) dividedLen(ids []string) int {
	l := len(ids)
	for l > 0 {
		if s.canDivide(ids[l-1]) {
			return l
		}
		l--
	}
	return 0
}

func (s *State) wrapChildrenToDiv(id string, seq *int32) (nextId string) {
	parent := s.Get(id).Model()
	overflow := maxChildrenThreshold - len(parent.ChildrenIds)
	if isDivLayout(parent) {
		changes := false
		nextDiv := s.getNextDiv(id)
		if nextDiv == nil || len(nextDiv.Model().ChildrenIds)+overflow > maxChildrenThreshold {
			nextDiv = s.newDiv(seq)
			s.Add(nextDiv)
			s.InsertTo(parent.Id, model.Block_Bottom, nextDiv.Model().Id)
			changes = true
		}
		for s.divBalance(parent, nextDiv.Model()) {
			parent = nextDiv.Model()
			nextDiv = s.getNextDiv(parent.Id)
			overflow = maxChildrenThreshold - len(parent.ChildrenIds)
			if nextDiv == nil || len(nextDiv.Model().ChildrenIds)+overflow > maxChildrenThreshold {
				nextDiv = s.newDiv(seq)
				s.Add(nextDiv)
				s.InsertTo(parent.Id, model.Block_Bottom, nextDiv.Model().Id)
				changes = true
			}
		}
		if changes {
			return s.PickParentOf(parent.Id).Model().Id
		}
		return ""
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
	div := sum / 2
	if sum > maxChildrenThreshold*2 {
		div = maxChildrenThreshold / 2
	}
	for div < sum && !s.canDivide(d1.ChildrenIds[div]) {
		div++
	}
	d2.ChildrenIds = make([]string, len(d1.ChildrenIds[div:]))
	copy(d2.ChildrenIds, d1.ChildrenIds[div:])
	d1.ChildrenIds = d1.ChildrenIds[:div]
	return s.dividedLen(d2.ChildrenIds) > maxChildrenThreshold
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
	if b := s.pickNextDiv(id); b != nil {
		return s.Get(b.Model().Id)
	}
	return nil
}

func (s *State) pickNextDiv(id string) simple.Block {
	parent := s.PickParentOf(id)
	if parent != nil {
		pm := parent.Model()
		pos := slice.FindPos(pm.ChildrenIds, id)
		if pos != -1 && pos < len(pm.ChildrenIds)-1 {
			b := s.Pick(pm.ChildrenIds[pos+1])
			if isDivLayout(b.Model()) {
				return b
			}
		}
	}
	return nil
}

func (s *State) removeDuplicates() {
	childrenIds := make(map[string]struct{})
	handledBlocks := make(map[string]struct{})
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if _, ok := handledBlocks[b.Model().Id]; ok {
			return true
		}
		var delIdx []int
		for i, cid := range b.Model().ChildrenIds {
			if _, ok := childrenIds[cid]; ok {
				delIdx = append(delIdx, i)
			} else {
				childrenIds[cid] = struct{}{}
			}
		}
		if len(delIdx) > 0 {
			b = s.Get(b.Model().Id)
			chIds := b.Model().ChildrenIds
			for i, idx := range delIdx {
				idx = idx - i
				chIds = append(chIds[:idx], chIds[idx+1:]...)
			}
			b.Model().ChildrenIds = chIds
		}
		handledBlocks[b.Model().Id] = struct{}{}
		return true
	})
}

func CleanupLayouts(s *State) (removedCount int) {
	var divIds []string
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if layout := b.Model().GetLayout(); layout != nil && layout.Style == model.BlockContentLayout_Div {
			divIds = append(divIds, b.Model().Id)
		}
		return true
	})
	for _, divId := range divIds {
		divChildrens := s.Pick(divId).Model().ChildrenIds
		for _, dCh := range divChildrens {
			s.Unlink(dCh)
		}
		s.InsertTo(divId, model.Block_Replace, divChildrens...)
	}
	return len(divIds)
}
