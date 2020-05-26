package state

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func NewDocFromSnapshot(rootId string, snapshot *pb.ChangeSnapshot) Doc {
	blocks := make(map[string]simple.Block)
	for _, b := range snapshot.Data.Blocks {
		blocks[b.Id] = simple.New(b)
	}
	return &State{
		rootId: rootId,
		blocks: blocks,
	}
}

func (s *State) SetChangeId(id string) {
	s.changeId = id
}

func (s *State) ChangeId() string {
	return s.changeId
}

func (s *State) ApplyChange(change *pb.Change) (err error) {
	// TODO:
	return
}

func (s *State) Merge(st *State) *State {
	result := s.NewState()
	return result
}
