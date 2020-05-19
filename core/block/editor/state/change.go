package state

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

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
