package source

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"math/rand"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/threads"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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
	Anytype() core.Service
	Type() pb.SmartBlockType
	Virtual() bool
	ReadOnly() bool
	ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error)
	ReadMeta(receiver ChangeReceiver) (doc state.Doc, err error)
	PushChange(params PushChangeParams) (id string, err error)
	FindFirstChange(ctx context.Context) (c *change.Change, err error)
	Close() (err error)
}

var ErrUnknownDataFormat = fmt.Errorf("unknown data format: you may need to upgrade anytype in order to open this page")

func NewSource(a core.Service, ss status.Service, id string) (s Source, err error) {
	st, err := smartblock.SmartBlockTypeFromID(id)

	if id == addr.AnytypeProfileId {
		return NewAnytypeProfile(a, id), nil
	}

	if st == smartblock.SmartBlockTypeFile {
		return NewFiles(a, id), nil
	}

	if st == smartblock.SmartBlockTypeBundledObjectType {
		return NewBundledObjectType(a, id), nil
	}

	if st == smartblock.SmartBlockTypeBundledRelation {
		return NewBundledRelation(a, id), nil
	}

	if st == smartblock.SmartBlockTypeIndexedRelation {
		return NewIndexedRelation(a, id), nil
	}

	return newSource(a, ss, id)
}

func newSource(a core.Service, ss status.Service, id string) (s Source, err error) {
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
		id:       id,
		tid:      tid,
		a:        a,
		ss:       ss,
		sb:       sb,
		logId:    a.Device(),
		openedAt: time.Now(),
	}
	return
}

type source struct {
	id, logId      string
	tid            thread.ID
	a              core.Service
	ss             status.Service
	sb             core.SmartBlock
	tree           *change.Tree
	lastSnapshotId string
	logHeads       map[string]*change.Change
	receiver       ChangeReceiver
	unsubscribe    func()
	metaOnly       bool
	closed         chan struct{}
	openedAt       time.Time
}

func (s *source) ReadOnly() bool {
	return false
}

func (s *source) Id() string {
	return s.id
}

func (s *source) Anytype() core.Service {
	return s.a
}

func (s *source) Type() pb.SmartBlockType {
	return smartblock.SmartBlockTypeToProto(s.sb.Type())
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

	if s.sb.Type() != smartblock.SmartBlockTypeArchive && !s.Virtual() {
		if verr := st.Validate(); verr != nil {
			log.With("thread", s.id).With("sbType", s.sb.Type()).Errorf("not valid state: %v", verr)
		}
	}
	if err = st.Normalize(false); err != nil {
		return
	}

	// set local-only details
	st.SetDetailAndBundledRelation(bundle.RelationKeyLastOpenedDate, pbtypes.Int64(s.openedAt.Unix()))
	InjectLocalDetails(s, st)

	if s.Type() != pb.SmartBlockType_Archive && !s.Virtual() {
		// we do not need details for archive or breadcrumbs
		st.InjectDerivedDetails()
		err = InjectCreationInfo(s, st)
		if err != nil {
			log.With("thread", s.id).Errorf("injectCreationInfo failed: %s", err.Error())
		}
	}

	if _, _, err = state.ApplyState(st, false); err != nil {
		return
	}

	return
}

func InjectCreationInfo(s Source, st *state.State) (err error) {
	if s.Anytype() == nil {
		return fmt.Errorf("anytype is nil")
	}

	defer func() {
		if !pbtypes.HasRelation(st.ExtraRelations(), bundle.RelationKeyCreator.String()) {
			st.SetExtraRelation(bundle.MustGetRelation(bundle.RelationKeyCreator))
		}
		if !pbtypes.HasRelation(st.ExtraRelations(), bundle.RelationKeyCreatedDate.String()) {
			st.SetExtraRelation(bundle.MustGetRelation(bundle.RelationKeyCreatedDate))
		}
	}()

	if pbtypes.HasField(st.Details(), bundle.RelationKeyCreator.String()) {
		return nil
	}

	var (
		createdDate = time.Now().Unix()
		createdBy   = s.Anytype().Account()
	)
	// protect from the big documents with a large trees
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	fc, err := s.FindFirstChange(ctx)
	if err == change.ErrEmpty {
		err = nil
		log.Debugf("InjectCreationInfo set for the empty object")
	} else if err != nil {
		return fmt.Errorf("failed to find first change to derive creation info")
	} else {
		createdDate = fc.Timestamp
		createdBy = fc.Account
	}

	st.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Float64(float64(createdDate)))
	if profileId, e := threads.ProfileThreadIDFromAccountAddress(createdBy); e == nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(profileId.String()))
	}
	return
}

func InjectLocalDetails(s Source, st *state.State) {
	if details, err := s.Anytype().ObjectStore().GetDetails(s.Id()); err == nil {
		if details != nil && details.Details != nil {
			for key, v := range details.Details.Fields {
				if slice.FindPos(bundle.LocalRelationsKeys, key) != -1 {
					st.SetDetail(key, v)
					if !pbtypes.HasRelation(st.ExtraRelations(), key) {
						st.SetExtraRelation(bundle.MustGetRelation(bundle.RelationKey(key)))
					}
				}
			}
		}
	}
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
			LogHeads: s.logHeadIDs(),
			Data: &model.SmartBlockSnapshotBase{
				Blocks:         params.State.Blocks(),
				Details:        params.State.ObjectScopedDetails(),
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
	s.logHeads[s.logId] = ch
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
	batch := mb.New(0)
	defer batch.Close()
	go func() {
		defer close(s.closed)
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
		if s.metaOnly && !ch.HasMeta() {
			continue
		}
		changes = append(changes, ch)
		s.logHeads[record.LogID] = ch
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

func (s *source) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	if s.tree.RootId() == "" {
		return nil, change.ErrEmpty
	}
	c = s.tree.Get(s.tree.RootId())
	for c.LastSnapshotId != "" {
		var rec *core.SmartblockRecordEnvelope
		if rec, err = s.sb.GetRecord(ctx, c.LastSnapshotId); err != nil {
			return
		}
		if c, err = change.NewChangeFromRecord(*rec); err != nil {
			return
		}
	}
	return
}

func (s *source) logHeadIDs() map[string]string {
	var hs = make(map[string]string)
	for id, ch := range s.logHeads {
		if ch != nil {
			hs[id] = ch.Id
		}
	}
	return hs
}

func (s *source) timeline() []status.LogTime {
	var timeline = make([]status.LogTime, 0, len(s.logHeads))
	for _, ch := range s.logHeads {
		if ch != nil && len(ch.Account) > 0 && len(ch.Device) > 0 {
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
