package state

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var log = logging.Logger("anytype-mw-state")

type Doc interface {
	RootId() string
	NewState() *State
	NewStateCtx(ctx *Context) *State
	Blocks() []*model.Block
	Pick(id string) (b simple.Block)
	Append(targetId string, id string) (ok bool)
}

func NewDoc(rootId string, blocks map[string]simple.Block) Doc {
	if blocks == nil {
		blocks = make(map[string]simple.Block)
	}
	return &State{
		rootId: rootId,
		blocks: blocks,
	}
}

type State struct {
	ctx      *Context
	parent   *State
	blocks   map[string]simple.Block
	rootId   string
	toRemove []string
	newIds   []string
}

func (s *State) RootId() string {
	return s.rootId
}

func (s *State) NewState() *State {
	return &State{parent: s, blocks: make(map[string]simple.Block), rootId: s.rootId}
}

func (s *State) NewStateCtx(ctx *Context) *State {
	return &State{parent: s, blocks: make(map[string]simple.Block), rootId: s.rootId, ctx: ctx}
}

func (s *State) Context() *Context {
	return s.ctx
}

func (s *State) Add(b simple.Block) (ok bool) {
	id := b.Model().Id
	if s.Pick(id) == nil {
		s.blocks[id] = b
		if s.parent != nil {
			s.newIds = append(s.newIds, id)
		}
		return true
	}
	return false
}

func (s *State) Set(b simple.Block) {
	if !s.Exists(b.Model().Id) {
		s.Add(b)
	} else {
		s.blocks[b.Model().Id] = b
	}
}

func (s *State) Get(id string) (b simple.Block) {
	if slice.FindPos(s.toRemove, id) != -1 {
		return nil
	}
	if b = s.blocks[id]; b != nil {
		return
	}
	if s.parent != nil {
		if b = s.parent.Get(id); b != nil {
			b = b.Copy()
			s.blocks[id] = b
			return
		}
	}
	return
}

func (s *State) Pick(id string) (b simple.Block) {
	if slice.FindPos(s.toRemove, id) != -1 {
		return nil
	}
	if b = s.blocks[id]; b != nil {
		return
	}
	if s.parent != nil {
		return s.parent.Pick(id)
	}
	return
}

func (s *State) PickOrigin(id string) (b simple.Block) {
	if s.parent != nil {
		return s.parent.Pick(id)
	}
	return
}

func (s *State) Remove(id string) (ok bool) {
	if slice.FindPos(s.toRemove, id) != -1 {
		return false
	}
	if s.Pick(id) != nil {
		s.Unlink(id)
		if _, ok = s.blocks[id]; ok {
			delete(s.blocks, id)
		}
		s.toRemove = append(s.toRemove, id)
		if slice.FindPos(s.newIds, id) != -1 {
			s.newIds = slice.Remove(s.newIds, id)
		}
		return true
	}
	return
}

func (s *State) Unlink(id string) (ok bool) {
	if parent := s.GetParentOf(id); parent != nil {
		parentM := parent.Model()
		parentM.ChildrenIds = slice.Remove(parentM.ChildrenIds, id)
		return true
	}
	return
}

func (s *State) Append(targetId string, id string) (ok bool) {
	parent := s.Get(targetId).Model()
	parent.ChildrenIds = append(parent.ChildrenIds, id)
	return true
}

func (s *State) GetParentOf(id string) (res simple.Block) {
	if parent := s.PickParentOf(id); parent != nil {
		return s.Get(parent.Model().Id)
	}
	return
}

func (s *State) PickParentOf(id string) (res simple.Block) {
	s.Iterate(func(b simple.Block) bool {
		if slice.FindPos(b.Model().ChildrenIds, id) != -1 {
			res = b
			return false
		}
		return true
	})
	return
}

func (s *State) Iterate(f func(b simple.Block) (isContinue bool)) (err error) {
	var iter func(id string) (isContinue bool, err error)
	var parentIds []string
	iter = func(id string) (isContinue bool, err error) {
		if slice.FindPos(parentIds, id) != -1 {
			return false, fmt.Errorf("cycle reference: %v %s", parentIds, id)
		}
		parentIds = append(parentIds, id)
		parentSize := len(parentIds)
		b := s.Pick(id)
		if b != nil {
			if isContinue = f(b); !isContinue {
				return
			}
			for _, cid := range b.Model().ChildrenIds {
				if isContinue, err = iter(cid); !isContinue || err != nil {
					return
				}
				parentIds = parentIds[:parentSize]
			}
		}
		return true, nil
	}
	_, err = iter(s.RootId())
	return
}

func (s *State) Exists(id string) (ok bool) {
	return s.Pick(id) != nil
}

func ApplyState(s *State) (msgs []*pb.EventMessage, action history.Action, err error) {
	return s.apply()
}

func (s *State) apply() (msgs []*pb.EventMessage, action history.Action, err error) {
	st := time.Now()
	if err = s.normalize(); err != nil {
		return
	}

	var toSave []*model.Block
	var newBlocks []*model.Block
	for id, b := range s.blocks {
		if slice.FindPos(s.toRemove, id) != -1 {
			continue
		}
		orig := s.PickOrigin(id)
		if orig == nil {
			newBlocks = append(newBlocks, b.Model())
			toSave = append(toSave, b.Model())
			action.Add = append(action.Add, b.Copy())
			continue
		}

		diff, err := orig.Diff(b)
		if err != nil {
			return nil, history.Action{}, err
		}
		if len(diff) > 0 {
			toSave = append(toSave, b.Model())
			msgs = append(msgs, diff...)
			if file := orig.Model().GetFile(); file != nil {
				if file.State == model.BlockContentFile_Uploading {
					file.State = model.BlockContentFile_Empty
				}
			}
			action.Change = append(action.Change, history.Change{
				Before: orig.Copy(),
				After:  b.Copy(),
			})
		}
	}
	if len(s.toRemove) > 0 {
		msgs = append(msgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockDelete{
				BlockDelete: &pb.EventBlockDelete{BlockIds: s.toRemove},
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
	for _, b := range s.blocks {
		if s.parent != nil {
			s.parent.blocks[b.Model().Id] = b
		}
	}
	for _, id := range s.toRemove {
		if old := s.PickOrigin(id); old != nil {
			action.Remove = append(action.Remove, old.Copy())
		}
		if s.parent != nil {
			delete(s.parent.blocks, id)
		}
	}
	log.Infof("middle: state apply: %d for save; %d for remove; %d copied; for a %v", len(toSave), len(s.toRemove), len(s.blocks), time.Since(st))
	return
}

func (s *State) Blocks() []*model.Block {
	if s.Pick(s.RootId()) == nil {
		return nil
	}
	return s.fillSlice(s.RootId(), make([]*model.Block, 0, len(s.blocks)))
}

func (s *State) fillSlice(id string, blocks []*model.Block) []*model.Block {
	blocks = append(blocks, s.blocks[id].Copy().Model())
	for _, chId := range s.blocks[id].Model().ChildrenIds {
		blocks = s.fillSlice(chId, blocks)
	}
	return blocks
}

func (s *State) String() (res string) {
	buf := bytes.NewBuffer(nil)
	s.writeString(buf, 0, s.RootId())
	return buf.String()
}

func (s *State) writeString(buf *bytes.Buffer, l int, id string) {
	b := s.Pick(id)
	buf.WriteString(strings.Repeat("\t", l))
	if b == nil {
		buf.WriteString(id)
		buf.WriteString(" MISSING")
	} else {
		buf.WriteString(b.String())
	}
	buf.WriteString("\n")
	if b != nil {
		for _, cid := range b.Model().ChildrenIds {
			s.writeString(buf, l+1, cid)
		}
	}
}
