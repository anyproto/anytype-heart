package source

import (
	"context"
	"errors"
	"fmt"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/commonspace/object/tree/objecttree"
	"math/rand"
	"sync"
	"time"

	"github.com/cheggaaa/mb"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var log = logging.Logger("anytype-mw-source")
var (
	ErrObjectNotFound = errors.New("object not found")
	ErrReadOnly       = errors.New("object is read only")
)

type ChangeReceiver interface {
	StateAppend(func(d state.Doc) (s *state.State, err error), []*pb.ChangeContent) error
	StateRebuild(d state.Doc) (err error)
	sync.Locker
}

type Source interface {
	Id() string
	Anytype() core.Service
	Type() model.SmartBlockType
	Virtual() bool
	LogHeads() map[string]string
	GetFileKeysSnapshot() []*pb.ChangeFileKeys
	ReadOnly() bool
	ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error)
	PushChange(params PushChangeParams) (id string, err error)
	Close() (err error)
}

type CreationInfoProvider interface {
	GetCreationInfo() (creator string, createdDate int64, err error)
}

type SourceIdEndodedDetails interface {
	Id() string
	DetailsFromId() (*types.Struct, error)
}

type SourceType interface {
	ListIds() ([]string, error)
	Virtual() bool
}

type SourceWithType interface {
	Source
	SourceType
}

var ErrUnknownDataFormat = fmt.Errorf("unknown data format: you may need to upgrade anytype in order to open this page")

func (s *service) SourceTypeBySbType(blockType smartblock.SmartBlockType) (SourceType, error) {
	switch blockType {
	case smartblock.SmartBlockTypeAnytypeProfile:
		return &anytypeProfile{a: s.anytype}, nil
	case smartblock.SmartBlockTypeFile:
		return &files{a: s.anytype}, nil
	case smartblock.SmartBlockTypeBundledObjectType:
		return &bundledObjectType{a: s.anytype}, nil
	case smartblock.SmartBlockTypeBundledRelation:
		return &bundledRelation{a: s.anytype}, nil
	case smartblock.SmartBlockTypeWorkspaceOld:
		return &threadDB{a: s.anytype}, nil
	case smartblock.SmartBlockTypeBundledTemplate:
		return s.NewStaticSource("", model.SmartBlockType_BundledTemplate, nil, nil), nil
	default:
		if err := blockType.Valid(); err != nil {
			return nil, err
		} else {
			return &source{a: s.anytype, smartblockType: blockType}, nil
		}
	}
}

func newTreeSource(a core.Service, ss status.Service, id string, listenToOwnChanges bool) (s Source, err error) {
	return &source{
		id:                       id,
		a:                        a,
		ss:                       ss,
		listenToOwnDeviceChanges: listenToOwnChanges,
		logId:                    a.Device(),
		openedAt:                 time.Now(),
		smartblockType:           smartblock.SmartBlockTypePage,
	}, nil
}

type source struct {
	id, logId                string
	tid                      thread.ID
	smartblockType           smartblock.SmartBlockType
	a                        core.Service
	ss                       status.Service
	objectTree               objecttree.ObjectTree
	lastSnapshotId           string
	logHeads                 map[string]*change.Change
	receiver                 ChangeReceiver
	unsubscribe              func()
	metaOnly                 bool
	listenToOwnDeviceChanges bool // false means we will ignore own(same-logID) changes in applyRecords
	closed                   chan struct{}
	openedAt                 time.Time
}

func (s *source) Update(tree objecttree.ObjectTree) {

}

func (s *source) Rebuild(tree objecttree.ObjectTree) {

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

func (s *source) Type() model.SmartBlockType {
	return model.SmartBlockType(s.smartblockType)
}

func (s *source) Virtual() bool {
	return false
}

func (s *source) ReadDoc(ctx context.Context, receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	return s.readDoc(ctx, receiver, allowEmpty)
}

func (s *source) readDoc(ctx context.Context, receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	spc, err := s.a.SpaceService().AccountSpace(context.Background())
	if err != nil {
		return
	}

	s.objectTree, err = spc.BuildTree(context.Background(), s.id, s)
	if err != nil {
		return
	}

	return s.buildState()
	//if allowEmpty && s.objectTree.Heads()[0] == s.objectTree.Id() {
	//	err = nil
	//	doc = state.NewDoc(s.id, nil)
	//	doc.(*state.State).InjectDerivedDetails()
	//} else if doc, err = s.buildState(); err != nil {
	//
	//}
	//return
}

func (s *source) buildState() (doc state.Doc, err error) {
	st, err := change.BuildState(nil, s.objectTree)
	if err != nil {
		return
	}
	// TODO: check if we need to check this validation
	err = st.Validate()
	if err != nil {
		return
	}
	st.BlocksInit(st)
	st.InjectDerivedDetails()

	// TODO: check if we can leave only removeDuplicates instead of Normalize
	if err = st.Normalize(false); err != nil {
		return
	}

	// TODO: check if we can use apply fast one
	if _, _, err = state.ApplyState(st, false); err != nil {
		return
	}

	return
}

func (s *source) GetCreationInfo() (creator string, createdDate int64, err error) {
	if s.Anytype() == nil {
		return "", 0, fmt.Errorf("anytype is nil")
	}

	createdDate = s.objectTree.UnmarshalledHeader().Timestamp
	createdBy := s.objectTree.UnmarshalledHeader().Identity

	return
}

type PushChangeParams struct {
	State             *state.State
	Changes           []*pb.ChangeContent
	FileChangedHashes []string
	DoSnapshot        bool
}

func (s *source) PushChange(params PushChangeParams) (id string, err error) {
	if events := s.tree.GetDuplicateEvents(); events > 30 {
		params.DoSnapshot = true
		log.With("thread", s.id).Errorf("found %d duplicate events: do the snapshot", events)
		s.tree.ResetDuplicateEvents()
	}
	var c = &pb.Change{
		PreviousIds:     s.tree.Heads(),
		LastSnapshotId:  s.lastSnapshotId,
		PreviousMetaIds: s.tree.MetaHeads(),
		Timestamp:       time.Now().Unix(),
	}
	if params.DoSnapshot || s.needSnapshot() || len(params.Changes) == 0 {
		c.Snapshot = &pb.ChangeSnapshot{
			LogHeads: s.LogHeads(),
			Data: &model.SmartBlockSnapshotBase{
				Blocks:         params.State.BlocksToSave(),
				Details:        params.State.Details(),
				ExtraRelations: nil,
				ObjectTypes:    params.State.ObjectTypes(),
				Collections:    params.State.Store(),
				RelationLinks:  params.State.PickRelationLinks(),
			},
			FileKeys: s.getFileHashesForSnapshot(params.FileChangedHashes),
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

func (v *source) ListIds() ([]string, error) {
	ids, err := v.Anytype().ThreadsIds()
	if err != nil {
		return nil, err
	}
	ids = slice.Filter(ids, func(id string) bool {
		if v.Anytype().PredefinedBlocks().IsAccount(id) {
			return false
		}
		t, err := smartblock.SmartBlockTypeFromID(id)
		if err != nil {
			return false
		}
		return t == v.smartblockType
	})
	// exclude account thread id
	return ids, nil
}

func (s *source) needSnapshot() bool {
	if s.tree.Len() == 0 {
		// starting tree with snapshot
		return true
	}
	// TODO: think about a more smart way
	return rand.Intn(500) == 42
}

func (s *source) changeListener(batch *mb.MB) {
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
}

func (s *source) applyRecords(records []core.SmartblockRecordEnvelope) error {
	var changes = make([]*change.Change, 0, len(records))
	for _, record := range records {
		if record.LogID == s.a.Device() && !s.listenToOwnDeviceChanges {
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

	metrics.SharedClient.RecordEvent(metrics.ChangesetEvent{
		Diff: time.Now().Unix() - changes[0].Timestamp,
	})

	switch s.tree.Add(changes...) {
	case change.Nothing:
		// existing or not complete
		return nil
	case change.Append:
		changesContent := make([]*pb.ChangeContent, 0, len(changes))
		for _, ch := range changes {
			changesContent = append(changesContent, ch.Content...)
		}
		s.lastSnapshotId = s.tree.LastSnapshotId(context.TODO())
		return s.receiver.StateAppend(func(d state.Doc) (*state.State, error) {
			return change.BuildStateSimpleCRDT(d.(*state.State), s.tree)
		}, changesContent)
	case change.Rebuild:
		s.lastSnapshotId = s.tree.LastSnapshotId(context.TODO())
		doc, err := s.buildState()

		if err != nil {
			return err
		}
		return s.receiver.StateRebuild(doc.(*state.State))
	default:
		return fmt.Errorf("unsupported tree mode")
	}
}

func (s *source) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return s.getFileHashesForSnapshot(nil)
}

func (s *source) getFileHashesForSnapshot(changeHashes []string) []*pb.ChangeFileKeys {
	fileKeys := s.getFileKeysByHashes(changeHashes)
	var uniqKeys = make(map[string]struct{})
	for _, fk := range fileKeys {
		uniqKeys[fk.Hash] = struct{}{}
	}
	processFileKeys := func(keys []*pb.ChangeFileKeys) {
		for _, fk := range keys {
			if _, ok := uniqKeys[fk.Hash]; !ok {
				uniqKeys[fk.Hash] = struct{}{}
				fileKeys = append(fileKeys, fk)
			}
		}
	}
	s.tree.Iterate(s.tree.RootId(), func(c *change.Change) (isContinue bool) {
		if c.Snapshot != nil && len(c.Snapshot.FileKeys) > 0 {
			processFileKeys(c.Snapshot.FileKeys)
		}
		if len(c.Change.FileKeys) > 0 {
			processFileKeys(c.Change.FileKeys)
		}
		return true
	})
	return fileKeys
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
			log.With("thread", s.id).With("logid", s.logId).With("recordId", c.LastSnapshotId).Errorf("failed to load first change: %s", err.Error())
			return
		}
		if c, err = change.NewChangeFromRecord(*rec); err != nil {
			log.With("thread", s.id).
				With("logid", s.logId).
				With("change", rec.ID).Errorf("FindFirstChange: failed to unmarshal change: %s; continue", err.Error())
			err = nil
		}
	}
	return
}

func (s *source) LogHeads() map[string]string {
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
