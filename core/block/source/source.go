package source

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/cheggaaa/mb"
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
	ReadMeta(receiver ChangeReceiver) (doc state.Doc, err error)
	PushChange(params PushChangeParams) (id string, err error)
	Close() (err error)
}

var ErrUnknownDataFormat = fmt.Errorf("unknown data format: you may need to upgrade anytype in order to open this page")

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
	metaOnly       bool
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

func (s *source) ReadMeta(receiver ChangeReceiver) (doc state.Doc, err error) {
	s.metaOnly = true
	return s.readDoc(receiver, false)
}

func (s *source) ReadDoc(receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	return s.readDoc(receiver, allowEmpty)
}

func (s *source) readDoc(receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
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
	if s.metaOnly {
		s.tree, s.logHeads, err = change.BuildMetaTree(s.sb)
	} else {
		s.tree, s.logHeads, err = change.BuildTree(s.sb)
	}
	if allowEmpty && err == change.ErrEmpty {
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

type PushChangeParams struct {
	State             *state.State
	Changes           []*pb.ChangeContent
	FileChangedHashes []string
	DoSnapshot        bool
	GetAllFileHashes  func() []string
}

func (s *source) PushChange(params PushChangeParams) (id string, err error) {
	var c = &pb.Change{
		PreviousIds:     s.tree.Heads(),
		LastSnapshotId:  s.lastSnapshotId,
		PreviousMetaIds: s.tree.MetaHeads(),
		Timestamp:       time.Now().Unix(),
	}
	if params.DoSnapshot || s.needSnapshot() || len(params.Changes) == 0 {
		c.Snapshot = &pb.ChangeSnapshot{
			LogHeads: s.logHeads,
			Data: &model.SmartBlockSnapshotBase{
				Blocks:         params.State.Blocks(),
				Details:        params.State.Details(),
				ExtraRelations: params.State.ExtraRelations(),
				ObjectTypes:    params.State.ObjectTypes(),
			},
			FileKeys: s.getFileKeysByHashes(params.GetAllFileHashes()),
		}
	}
	c.Content = params.Changes
	c.FileKeys = s.getFileKeysByHashes(params.FileChangedHashes)

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

func (s *source) changeListener(recordsCh chan core.SmartblockRecordWithLogID) {
	batch := mb.New(0)
	defer batch.Close()
	go func() {
		defer close(s.closed)
		var records []core.SmartblockRecordWithLogID
		for {
			msgs := batch.Wait()
			if len(msgs) == 0 {
				return
			}
			records = records[:0]
			for _, msg := range msgs {
				records = append(records, msg.(core.SmartblockRecordWithLogID))
			}
			if err := s.applyRecords(records); err != nil {
				log.Errorf("can't handle records: %v; records: %v", err, records)
			}
			// wait 100 millisecond for better batching
			time.Sleep(100 * time.Millisecond)
		}
	}()
	for r := range recordsCh {
		batch.Add(r)
	}
}

func (s *source) applyRecords(records []core.SmartblockRecordWithLogID) (err error) {
	s.receiver.Lock()
	defer s.receiver.Unlock()
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
		if s.metaOnly && !ch.HasMeta() {
			continue
		}
		changes = append(changes, ch)
		s.logHeads[record.LogID] = record.ID
	}
	log.With("thread", s.id).Infof("received %d records; changes count: %d", len(records), len(changes))
	if len(changes) == 0 {
		return
	}
	switch s.tree.Add(changes...) {
	case change.Nothing:
		// existing or not complete
		return
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
	if s.unsubscribe != nil {
		s.unsubscribe()
		<-s.closed
	}
	return nil
}
