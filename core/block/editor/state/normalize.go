package state

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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
