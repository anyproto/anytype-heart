package block

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
	"github.com/mohae/deepcopy"
)

func (s *commonSmart) newState() *state {
	return &state{
		sb:     s,
		blocks: make(map[string]simple.Block),
	}
}

type state struct {
	sb             *commonSmart
	blocks         map[string]simple.Block
	toRemove       []string
	newSmartBlocks bool
}

func (s *state) create(b *model.Block) (new simple.Block, err error) {
	if b == nil {
		return nil, fmt.Errorf("can't create nil block")
	}
	if isSmartBlock(b) {
		if err = s.sb.createSmartBlock(b); err != nil {
			return nil, fmt.Errorf("can't create smartblock: %v", err)
		}
		b = s.createLink(b)
		s.newSmartBlocks = true
	}
	nb, err := s.sb.block.NewBlock(*b)
	if err != nil {
		return
	}
	new = simple.New(b)
	new.Model().Id = nb.GetId()
	s.blocks[new.Model().Id] = new
	return
}

func (s *state) createLink(target *model.Block) (m *model.Block) {
	style := model.BlockContentLink_Page
	if target.GetDataview() != nil {
		style = model.BlockContentLink_Dataview
	}
	return &model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: target.Id,
				Style:         style,
				Fields:        deepcopy.Copy(target.Fields).(*types.Struct),
			},
		},
	}
}

func (s *state) get(id string) simple.Block {
	if b, ok := s.blocks[id]; ok {
		return b
	}
	if b, ok := s.sb.versions[id]; ok {
		copy := b.Copy()
		s.blocks[id] = copy
		return copy
	}
	return nil
}

func (s *state) exists(id string) bool {
	if findPosInSlice(s.toRemove, id) != -1 {
		return false
	}
	if _, ok := s.blocks[id]; ok {
		return true
	}
	if _, ok := s.sb.versions[id]; ok {
		return true
	}
	return false
}

func (s *state) getText(id string) (text.Block, error) {
	if b := s.get(id); b != nil {
		tb, ok := b.(text.Block)
		if ! ok {
			return nil, ErrUnexpectedBlockType
		}
		return tb, nil
	}
	return nil, ErrBlockNotFound
}

func (s *state) getIcon(id string) (base.IconBlock, error) {
	if b := s.get(id); b != nil {
		tb, ok := b.(base.IconBlock)
		if ! ok {
			return nil, ErrUnexpectedBlockType
		}
		return tb, nil
	}
	return nil, ErrBlockNotFound
}

func (s *state) getFile(id string) (file.Block, error) {
	if b := s.get(id); b != nil {
		tb, ok := b.(file.Block)
		if ! ok {
			return nil, ErrUnexpectedBlockType
		}
		return tb, nil
	}
	return nil, ErrBlockNotFound
}

func (s *state) remove(id string) {
	if _, ok := s.blocks[id]; ok {
		delete(s.blocks, id)
	}
	if findPosInSlice(s.toRemove, id) == -1 {
		s.toRemove = append(s.toRemove, id)
	}
}

func (s *state) findParentOf(id string) simple.Block {
	b := s.sb.findParentOf(id, s.blocks, s.sb.versions)
	if b == nil {
		return nil
	}
	return s.get(b.Model().Id)
}

func (s *state) apply(action *history.Action) (msgs []*pb.EventMessage, err error) {
	st := time.Now()
	s.normalize()
	var toSave []*model.Block
	var newBlocks []*model.Block
	for id, b := range s.blocks {
		if findPosInSlice(s.toRemove, id) != -1 {
			continue
		}
		orig, ok := s.sb.versions[id]
		if ! ok {
			newBlocks = append(newBlocks, b.Model())
			if !b.Virtual() {
				toSave = append(toSave, s.sb.toSave(b.Model(), s.blocks, s.sb.versions))
			}
			if action != nil {
				action.Add = append(action.Add, *b.Model())
			}
			continue
		}

		diff, err := orig.Diff(b)
		if err != nil {
			return nil, err
		}
		if len(diff) > 0 {
			if !b.Virtual() {
				toSave = append(toSave, s.sb.toSave(b.Model(), s.blocks, s.sb.versions))
			}
			msgs = append(msgs, diff...)
			if action != nil {
				action.Change = append(action.Change, *b.Model())
			}
		}
		if err := s.validateBlock(b); err != nil {
			return nil, err
		}
	}
	for _, removeId := range s.toRemove {
		msgs = append(msgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockDelete{
				BlockDelete: &pb.EventBlockDelete{BlockId: removeId},
			},
		})
	}
	if len(newBlocks) > 0 {
		msgs = append(msgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockAdd{
				BlockAdd: &pb.EventBlockAdd{
					Blocks: newBlocks,
				},
			},
		})
	}
	if len(toSave) > 0 {
		if _, err = s.sb.block.AddVersions(toSave); err != nil {
			return
		}
		if s.newSmartBlocks {
			s.sb.block.Flush()
		}
	}
	for _, b := range s.blocks {
		s.sb.setBlock(b)
	}
	for _, id := range s.toRemove {
		if old := s.sb.deleteBlock(id); old != nil && action != nil {
			action.Remove = append(action.Remove, *old.Model())
		}
	}
	fmt.Printf("middle: state apply: %d for save; %d for remove; %d copied; for a %v\n", len(toSave), len(s.toRemove), len(s.blocks), time.Since(st))
	return
}

func (s *state) normalize() {
	// remove invalid children
	for _, b := range s.blocks {
		s.normalizeChildren(b)
	}
	// remove empty layouts
	for _, b := range s.blocks {
		if layout := b.Model().GetLayout(); layout != nil {
			if len(b.Model().ChildrenIds) == 0 {
				s.removeFromChilds(b.Model().Id)
				s.remove(b.Model().Id)
				fmt.Println("normalize: remove empty layout:", b.Model().Id)
			}
			// pick parent for checking
			s.findParentOf(b.Model().Id)
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

func (s *state) normalizeChildren(b simple.Block) {
	m := b.Model()
	for _, cid := range m.ChildrenIds {
		if !s.exists(cid) {
			fmt.Println("normalize: remove missed children:", cid)
			m.ChildrenIds = removeFromSlice(m.ChildrenIds, cid)
			s.normalizeChildren(b)
			return
		}
	}
}

func (s *state) normalizeLayoutRow(b simple.Block) {
	if b.Model().GetLayout().Style != model.BlockContentLayout_Row {
		return
	}
	// remove empty row
	if len(b.Model().ChildrenIds) == 0 {
		s.removeFromChilds(b.Model().Id)
		s.remove(b.Model().Id)
		fmt.Println("normalize: remove empty row:", b.Model().Id)
		return
	}
	// one column - remove row
	if len(b.Model().ChildrenIds) == 1 {
		var (
			contentIds   []string
			removeColumn bool
		)
		column := s.get(b.Model().ChildrenIds[0])
		if layout := column.Model().GetLayout(); layout != nil && layout.Style == model.BlockContentLayout_Column {
			contentIds = column.Model().ChildrenIds
			removeColumn = true
		} else {
			contentIds = append(contentIds, column.Model().Id)
		}
		if parent := s.findParentOf(b.Model().Id); parent != nil {
			rowPos := findPosInSlice(parent.Model().ChildrenIds, b.Model().Id)
			if rowPos != -1 {
				parent.Model().ChildrenIds = removeFromSlice(parent.Model().ChildrenIds, b.Model().Id)
				for _, id := range contentIds {
					parent.Model().ChildrenIds = insertToSlice(parent.Model().ChildrenIds, id, rowPos)
					rowPos++
				}
				if removeColumn {
					s.remove(column.Model().Id)
				}
				s.remove(b.Model().Id)
				fmt.Println("normalize: remove one column row:", b.Model().Id)
			}
		}
		return
	}

	// reset columns width when count of row children was changed
	orig := s.sb.versions[b.Model().Id]
	if orig != nil && len(orig.Model().ChildrenIds) != len(b.Model().ChildrenIds) {
		for _, chId := range b.Model().ChildrenIds {
			fields := s.get(chId).Model().Fields
			if fields != nil && fields.Fields != nil && fields.Fields["width"] != nil {
				fields.Fields["width"] = testFloatValue(0)
			}
		}
	}
}

func (s *state) removeFromChilds(id string) (ok bool) {
	if parent := s.findParentOf(id); parent != nil {
		if pos := findPosInSlice(parent.Model().ChildrenIds, id); pos != -1 {
			parent.Model().ChildrenIds = removeFromSlice(parent.Model().ChildrenIds, id)
			return true
		}
	}
	return
}

func (s *state) validateBlock(b simple.Block) (err error) {
	id := b.Model().Id
	if id == s.sb.GetId() {
		return
	}
	var parentIds = []string{id}
	for {
		parent := s.sb.findParentOf(id, s.blocks, s.sb.versions)
		if parent == nil {
			break
		}
		if parent.Model().Id == s.sb.GetId() {
			return nil
		}
		if findPosInSlice(parentIds, parent.Model().Id) != -1 {
			return fmt.Errorf("cycle reference: %v", append(parentIds, parent.Model().Id))
		}
		id = parent.Model().Id
		parentIds = append(parentIds, id)
	}
	return fmt.Errorf("block '%s' has not the page in parents", id)
}
