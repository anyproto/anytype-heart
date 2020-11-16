package source

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/cheggaaa/mb"
	"github.com/textileio/go-threads/core/thread"
)

var log = logging.Logger("anytype-mw-source")

type ChangeReceiver interface {
	StateAppend(func(d state.Doc) (s *state.State, err error)) error
	StateRebuild(d state.Doc) (err error)
	sync.Locker
}

type Source interface {
	Id() string
	Anytype() anytype.Service
	Type() pb.SmartBlockType
	Virtual() bool
	ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error)
	ReadDetails(receiver ChangeReceiver) (doc state.Doc, err error)
	PushChange(st *state.State, changes []*pb.ChangeContent, fileChangedHashes []string, doSnapshot bool) (id string, err error)
	Close() (err error)
}

var ErrUnknownDataFormat = fmt.Errorf("unknown data format: you may need to upgrade anytype in order to open this page")

func NewSource(a anytype.Service, ss status.Service, id string) (s Source, err error) {
	sb, err := a.GetBlock(id)
	if err != nil {
		err = fmt.Errorf("anytype.GetBlock error: %w", err)
		return
	}

	tid, err := thread.Decode(id)
	if err != nil {
		err = fmt.Errorf("can't restore thread ID: %w", err)
		return
	}

	s = &source{
		id:    id,
		tid:   tid,
		a:     a,
		ss:    ss,
		sb:    sb,
		logId: a.Device(),
	}
	return
}

type source struct {
	id, logId      string
	tid            thread.ID
	a              anytype.Service
	ss             status.Service
	sb             core.SmartBlock
	tree           *change.Tree
	lastSnapshotId string
	logHeads       map[string]string
	receiver       ChangeReceiver
	unsubscribe    func()
	detailsOnly    bool
	closed         chan struct{}
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

func (s *source) Virtual() bool {
	return false
}

func (s *source) ReadDetails(receiver ChangeReceiver) (doc state.Doc, err error) {
	s.detailsOnly = true
	return s.readDoc(receiver, false)
}

func (s *source) ReadDoc(receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	return s.readDoc(receiver, allowEmpty)
}

func (s *source) readDoc(receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	var ch chan core.SmartblockRecordEnvelope
	if receiver != nil {
		s.receiver = receiver
		ch = make(chan core.SmartblockRecordEnvelope)
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
	if allowEmpty && err == change.ErrEmpty {
		err = nil
		s.tree = new(change.Tree)
		doc = state.NewDoc(s.id, nil)
	} else if err != nil {
		return
	} else if doc, err = s.buildState(); err != nil {
		return
	}

	if s.ss != nil {
		// update timeline with recent information about heads
		s.ss.UpdateTimeline(s.tid, s.timeline())
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
	if verr := st.Validate(); verr != nil {
		log.With("thread", s.id).Errorf("not valid state: %v", verr)
	}
	if err = st.Normalize(false); err != nil {
		return
	}
	if _, _, err = state.ApplyState(st, false); err != nil {
		return
	}
	return
}

func (s *source) PushChange(st *state.State, changes []*pb.ChangeContent, fileChangedHashes []string, doSnapshot bool) (id string, err error) {
	var c = &pb.Change{
		PreviousIds:        s.tree.Heads(),
		LastSnapshotId:     s.lastSnapshotId,
		PreviousDetailsIds: s.tree.DetailsHeads(),
		Timestamp:          time.Now().Unix(),
	}
	if doSnapshot || s.needSnapshot() || len(changes) == 0 {
		c.Snapshot = &pb.ChangeSnapshot{
			LogHeads: s.logHeads,
			Data: &model.SmartBlockSnapshotBase{
				Blocks:  st.BlocksToSave(),
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

func (s *source) changeListener(recordsCh chan core.SmartblockRecordEnvelope) {
	defer close(s.closed)
	batch := mb.New(0)
	defer batch.Close()
	go func() {
		var records []core.SmartblockRecordEnvelope
		for {
			msgs := batch.Wait()
			if len(msgs) == 0 {
				return
			}
			records = records[:0]
			for _, msg := range msgs {
				records = append(records, msg.(core.SmartblockRecordEnvelope))
			}

			s.receiver.Lock()
			if err := s.applyRecords(records); err != nil {
				log.Errorf("can't handle records: %v; records: %v", err, records)
			} else if s.ss != nil {
				// notify about probably updated timeline
				if tl := s.timeline(); len(tl) > 0 {
					s.ss.UpdateTimeline(s.tid, tl)
				}
			}
			s.receiver.Unlock()

			// wait 100 millisecond for better batching
			time.Sleep(100 * time.Millisecond)
		}
	}()
	for r := range recordsCh {
		batch.Add(r)
	}
}

func (s *source) applyRecords(records []core.SmartblockRecordEnvelope) error {
	var changes = make([]*change.Change, 0, len(records))
	for _, record := range records {
		if record.LogID == s.a.Device() {
			// ignore self logs
			continue
		}
		ch, e := change.NewChangeFromRecord(record)
		if e != nil {
			return e
		}
		if s.detailsOnly && !ch.HasDetails() {
			continue
		}
		changes = append(changes, ch)
		s.logHeads[record.LogID] = record.ID
	}
	log.With("thread", s.id).Infof("received %d records; changes count: %d", len(records), len(changes))
	if len(changes) == 0 {
		return nil
	}

	switch s.tree.Add(changes...) {
	case change.Nothing:
		// existing or not complete
		return nil
	case change.Append:
		s.lastSnapshotId = s.tree.LastSnapshotId()
		return s.receiver.StateAppend(func(d state.Doc) (*state.State, error) {
			return change.BuildStateSimpleCRDT(d.(*state.State), s.tree)
		})
	case change.Rebuild:
		s.lastSnapshotId = s.tree.LastSnapshotId()
		doc, err := s.buildState()
		if err != nil {
			return err
		}
		return s.receiver.StateRebuild(doc.(*state.State))
	default:
		return fmt.Errorf("unsupported tree mode")
	}
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

func (s *source) timeline() []status.LogTime {
	var (
		heads    = s.tree.Heads()
		timeline = make([]status.LogTime, 0, len(heads))
	)
	for _, head := range heads {
		if ch := s.tree.Get(head); ch != nil && len(ch.Account) > 0 && len(ch.Device) > 0 {
			timeline = append(timeline, status.LogTime{
				AccountID: ch.Account,
				DeviceID:  ch.Device,
				LastEdit:  ch.Timestamp,
			})
		}
	}
	return timeline
}

func (s *source) Close() (err error) {
	if s.unsubscribe != nil {
		s.unsubscribe()
		<-s.closed
	}
	return nil
}
