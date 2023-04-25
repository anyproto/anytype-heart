package change

import (
	"errors"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/commonspace/object/tree/objecttree"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
)

var ErrEmpty = errors.New("first change doesn't exist")

func BuildState(initState *state.State, ot objecttree.ObjectTree) (s *state.State, appliedContent []*pb.ChangeContent, err error) {
	var (
		startId    string
		lastChange *objecttree.Change
		count      int
	)
	// if the state has no first change
	if initState == nil {
		startId = ot.Root().Id
	} else {
		s = initState
		startId = s.ChangeId()
	}

	err = ot.IterateFrom(startId,
		func(decrypted []byte) (any, error) {
			ch := &pb.Change{}
			err = proto.Unmarshal(decrypted, ch)
			if err != nil {
				return nil, err
			}
			return ch, nil
		}, func(change *objecttree.Change) bool {
			count++
			lastChange = change
			// that means that we are starting from tree root
			if change.Id == ot.Id() {
				s = state.NewDoc(ot.Id(), nil).(*state.State)
				s.SetChangeId(change.Id)
				return true
			}

			model := change.Model.(*pb.Change)
			if startId == change.Id {
				if s == nil {
					s = state.NewDocFromSnapshot(change.Id, model.Snapshot, nil).(*state.State)
					s.SetChangeId(startId)
					return true
				}
				return true
			}
			ns := s.NewState()
			appliedContent = append(appliedContent, model.Content...)
			ns.ApplyChangeIgnoreErr(model.Content...)
			ns.SetChangeId(change.Id)
			ns.AddFileKeys(model.FileKeys...)
			_, _, err = state.ApplyStateFastOne(ns)
			if err != nil {
				return false
			}
			return true
		})
	if err != nil {
		return
	}
	if lastChange != nil {
		s.SetLastModified(lastChange.Timestamp, lastChange.Identity)
	}
	return
}
