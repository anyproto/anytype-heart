package source

import (
	"fmt"
	"math/rand"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var log = logging.Logger("anytype-mw-source")

type Source interface {
	Id() string
	Anytype() anytype.Service
	Meta() meta.Service
	Type() pb.SmartBlockType
	ReadDoc() (doc state.Doc, err error)
	PushChange(st *state.State, changes ...*pb.ChangeContent) (id string, err error)
	Close() (err error)
}

func NewSource(a anytype.Service, m meta.Service, id string) (s Source, err error) {
	sb, err := a.GetBlock(id)
	if err != nil {
		err = fmt.Errorf("anytype.GetBlock(%v) error: %v", id, err)
		return
	}
	s = &source{
		id:    id,
		a:     a,
		sb:    sb,
		meta:  m,
		logId: a.Device(),
	}
	return
}

type source struct {
	id, logId      string
	a              anytype.Service
	sb             core.SmartBlock
	meta           meta.Service
	tree           *change.Tree
	lastSnapshotId string
	logHeads       map[string]string
}

func (s *source) Id() string {
	return s.id
}

func (s *source) Anytype() anytype.Service {
	return s.a
}

func (s *source) Meta() meta.Service {
	return s.meta
}

func (s *source) Type() pb.SmartBlockType {
	return anytype.SmartBlockTypeToProto(s.sb.Type())
}

func (s *source) ReadDoc() (doc state.Doc, err error) {
	s.tree, s.logHeads, err = change.BuildTree(s.sb)
	if err == change.ErrEmpty {
		s.tree = new(change.Tree)
		return state.NewDoc(s.id, nil), nil
	} else if err != nil {
		return nil, err
	}
	root := s.tree.Root()
	if root == nil || root.GetSnapshot() == nil {
		return nil, fmt.Errorf("root missing or not a snapshot")
	}
	s.lastSnapshotId = root.Id
	doc = state.NewDocFromSnapshot(s.id, root.GetSnapshot()).(*state.State)
	doc.(*state.State).SetChangeId(root.Id)
	st, err := change.BuildState(doc.(*state.State), s.tree)
	if err != nil {
		return
	}
	if _, _, err = state.ApplyState(st); err != nil {
		return
	}
	return
}

func (s *source) PushChange(st *state.State, changes ...*pb.ChangeContent) (id string, err error) {
	var c = &pb.Change{
		PreviousIds:    s.tree.Heads(),
		LastSnapshotId: s.lastSnapshotId,
	}
	if s.needSnapshot() || len(changes) == 0 {
		c.Snapshot = &pb.ChangeSnapshot{
			LogHeads: s.logHeads,
			Data: &model.SmartBlockSnapshotBase{
				Blocks:  st.Blocks(),
				Details: st.Details(),
			},
		}
	}
	c.Content = changes

	if id, err = s.sb.PushRecord(c); err != nil {
		return
	}
	ch := &change.Change{Id: id, Change: c}
	s.tree.Add(ch)
	s.logHeads[s.logId] = id
	if c.Snapshot != nil {
		s.lastSnapshotId = id
		log.Infof("%s: pushed snapshot", s.id)
	} else {
		log.Debugf("%s: pushed %d changes", s.id, len(ch.Content))
	}
	return
}

func (s *source) needSnapshot() bool {
	if s.tree.Len() == 0 {
		// starting tree with snapshot
		return true
	}
	// TODO: think about a more smart way
	return rand.Intn(100) == 42
}

func (s *source) Close() (err error) {
	return nil
}
