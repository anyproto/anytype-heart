package block

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (s *commonSmart) newState() *state {
	return &state{
		sb:     s,
		blocks: make(map[string]simple.Block),
	}
}

type state struct {
	sb       *commonSmart
	blocks   map[string]simple.Block
	toRemove []string
}

func (s *state) create(b *model.Block) (new simple.Block, err error) {
	if b == nil {
		return nil, fmt.Errorf("can't create nil block")
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

func (s *state) remove(id string) {
	if findPosInSlice(s.toRemove, id) == -1 {
		s.toRemove = append(s.toRemove, id)
	}
}

func (s *state) findParentOf(id string) simple.Block {
	b := s.sb.findParentOf(id, s.blocks, s.sb.versions)
	if b == nil {
		return nil
	}
	if b == s.blocks[b.Model().Id] {
		return b
	}
	copy := b.Copy()
	s.blocks[b.Model().Id] = copy
	return copy
}

func (s *state) apply() (msgs []*pb.EventMessage, err error) {
	st := time.Now()
	var toSave []*model.Block
	for id, b := range s.blocks {
		if findPosInSlice(s.toRemove, id) != -1 {
			continue
		}
		orig, ok := s.sb.versions[id]
		if ! ok {
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfBlockAdd{
					BlockAdd: &pb.EventBlockAdd{
						Blocks: []*model.Block{b.Model()},
					},
				},
			})
			if !b.Virtual() {
				toSave = append(toSave, s.sb.toSave(b.Model(), s.blocks, s.sb.versions))
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
		}
		if err := s.sb.validateBlock(b, s.blocks, s.sb.versions); err != nil {
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
	if _, err = s.sb.block.AddVersions(toSave); err != nil {
		return
	}
	for id, b := range s.blocks {
		s.sb.versions[id] = b
	}
	for _, id := range s.toRemove {
		delete(s.sb.versions, id)
	}
	fmt.Printf("middle: state apply: %d for save; %d for remove; for a %v\n", len(toSave), len(s.toRemove), time.Since(st))
	return
}
