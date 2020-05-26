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
		rootId:  rootId,
		blocks:  blocks,
		details: snapshot.Data.Details,
	}
}

func (s *State) SetChangeId(id string) {
	s.changeId = id
}

func (s *State) ChangeId() string {
	return s.changeId
}

func (s *State) AddChanges(changes ...*pb.ChangeContent) {
	s.changes = append(s.changes, changes...)
}

func (s *State) ApplyChange(change *pb.Change) (err error) {
	// TODO:
	return
}

func (s *State) Merge(st *State) *State {
	result := s.NewState()
	// TODO:
	return result
}

func (s *State) GetChanges() []*pb.ChangeContent {
	res := s.changes
	if s.parent != nil {
		res = append(res, s.parent.GetChanges()...)
	}
	return res
}
