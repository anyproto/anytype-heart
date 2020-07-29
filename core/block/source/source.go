package source

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var log = logging.Logger("anytype-mw-source")

type ChangeReceiver interface {
	StateAppend(func(d state.Doc) (s *state.State, err error)) error
	StateRebuild(d state.Doc) (err error)
}

type Source interface {
	Id() string
	Anytype() anytype.Service
	Type() pb.SmartBlockType
	ReadDoc(receiver ChangeReceiver) (doc state.Doc, err error)
	ReadDetails(receiver ChangeReceiver) (doc state.Doc, err error)
	PushChange(st *state.State, changes []*pb.ChangeContent, fileChangedHashes []string) (id string, err error)
	Close() (err error)
}

func NewSource(a anytype.Service, id string) (s Source, err error) {
	sb, err := a.GetBlock(id)
	if err != nil {
		err = fmt.Errorf("anytype.GetBlock error: %v", err)
		return
	}
	s = &source{
		id:    id,
		a:     a,
		sb:    sb,
		logId: a.Device(),
	}
	return
}

type source struct {
	id, logId      string
	a              anytype.Service
	sb             core.SmartBlock
	tree           *change.Tree
	lastSnapshotId string
	logHeads       map[string]string
	receiver       ChangeReceiver
	unsubscribe    func()
	detailsOnly    bool
	closed         chan struct{}
	mu             sync.Mutex
}

func (s *source) Id() string {
	return s.id
}

func (s *source) Anytype() anytype.Service {
	return s.a
}

func (s *source) Type() pb.SmartBlockType {
	return anytype.SmartBlockTypeToProto(s.sb.Type())
}

func (s *source) ReadDetails(receiver ChangeReceiver) (doc state.Doc, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detailsOnly = true
	return s.readDoc(receiver)
}

func (s *source) ReadDoc(receiver ChangeReceiver) (doc state.Doc, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readDoc(receiver)
}

func (s *source) readDoc(receiver ChangeReceiver) (doc state.Doc, err error) {
	var ch chan core.SmartblockRecordWithLogID
	if receiver != nil {
		s.receiver = receiver
		ch = make(chan core.SmartblockRecordWithLogID)
		if s.unsubscribe, err = s.sb.SubscribeForRecords(ch); err != nil {
			return
		}
		defer func() {
			if err != nil {
				s.unsubscribe()
				s.unsubscribe = nil
			}
		}()
	}
	if s.detailsOnly {
		s.tree, s.logHeads, err = change.BuildDetailsTree(s.sb)
	} else {
		s.tree, s.logHeads, err = change.BuildTree(s.sb)
	}
	if err == change.ErrEmpty {
		err = nil
		s.tree = new(change.Tree)
		doc = state.NewDoc(s.id, nil)
	} else if err != nil {
		return nil, err
	} else if doc, err = s.buildState(); err != nil {
		return
	}
	if s.unsubscribe != nil {
		s.closed = make(chan struct{})
		go s.changeListener(ch)
	}
	return
}

func (s *source) buildState() (doc state.Doc, err error) {
	root := s.tree.Root()
	if root == nil || root.GetSnapshot() == nil {
		return nil, fmt.Errorf("root missing or not a snapshot")
	}
	s.lastSnapshotId = root.Id
	doc = state.NewDocFromSnapshot(s.id, root.GetSnapshot()).(*state.State)
	doc.(*state.State).SetChangeId(root.Id)
	st, err := change.BuildStateSimpleCRDT(doc.(*state.State), s.tree)
	if err != nil {
		return
	}
	if _, _, err = state.ApplyStateFast(st); err != nil {
		return
	}
	return
}

func (s *source) PushChange(st *state.State, changes []*pb.ChangeContent, fileChangedHashes []string) (id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c = &pb.Change{
		PreviousIds:        s.tree.Heads(),
		LastSnapshotId:     s.lastSnapshotId,
		PreviousDetailsIds: s.tree.DetailsHeads(),
		Timestamp:          time.Now().Unix(),
	}
	if s.needSnapshot() || len(changes) == 0 {
		c.Snapshot = &pb.ChangeSnapshot{
			LogHeads: s.logHeads,
			Data: &model.SmartBlockSnapshotBase{
				Blocks:  st.Blocks(),
				Details: st.Details(),
			},
			FileKeys: s.getFileKeysByHashes(st.GetAllFileHashes()),
		}
	}
	c.Content = changes
	c.FileKeys = s.getFileKeysByHashes(fileChangedHashes)

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
	return rand.Intn(500) == 42
}

func (s *source) changeListener(records chan core.SmartblockRecordWithLogID) {
	defer close(s.closed)
	for r := range records {
		if err := s.newChange(r); err != nil {
			log.Warnf("can't handle change: %v; %v", r.ID, err)
		}
	}
}

func (s *source) newChange(record core.SmartblockRecordWithLogID) (err error) {
	if record.LogID == s.a.Device() {
		// ignore self logs
		return
	}
	log.Infof("changes: received log record: %v", record.ID)
	ch, err := change.NewChangeFromRecord(record)
	if err != nil {
		return
	}
	if s.detailsOnly && !ch.HasDetails() {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.logHeads[record.LogID] = record.ID

	var heads []string
	if s.detailsOnly {
		heads = s.tree.DetailsHeads()
	} else {
		heads = s.tree.Heads()
	}
	if len(heads) == 0 {
		log.Warnf("unexpected empty heads while receive new change")
		return
	}

	switch s.tree.Add(ch) {
	case change.Nothing:
		// existing or not complete
		log.Debugf("add change to tree %v: nothing to do", ch.Id)
		return
	case change.Append:
		if ch.Snapshot != nil {
			s.lastSnapshotId = ch.Id
		}
		return s.receiver.StateAppend(func(d state.Doc) (*state.State, error) {
			return change.BuildStateSimpleCRDT(d.(*state.State), s.tree)
		})
	case change.Rebuild:
		if ch.Snapshot != nil {
			s.lastSnapshotId = ch.Id
		}
		doc, err := s.buildState()
		if err != nil {
			return err
		}
		return s.receiver.StateRebuild(doc.(*state.State))
	}
	return
}

func (s *source) getFileKeysByHashes(hashes []string) []*pb.ChangeFileKeys {
	fileKeys := make([]*pb.ChangeFileKeys, 0, len(hashes))
	for _, h := range hashes {
		fk, err := s.a.FileGetKeys(h)
		if err != nil {
			log.Warnf("can't get file key for hash: %v: %v", h, err)
			continue
		}
		fileKeys = append(fileKeys, &pb.ChangeFileKeys{
			Hash: fk.Hash,
			Keys: fk.Keys,
		})
	}
	return fileKeys
}

func (s *source) Close() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unsubscribe != nil {
		s.unsubscribe()
		<-s.closed
	}
	return nil
}
