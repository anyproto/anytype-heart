package source

import (
	"context"
	"errors"
	"fmt"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/logstore"
	"math/rand"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/cheggaaa/mb"
	"github.com/textileio/go-threads/core/thread"
)

var log = logging.Logger("anytype-mw-source")
var ErrObjectNotFound = errors.New("object not found")

type ChangeReceiver interface {
	StateAppend(func(d state.Doc) (s *state.State, err error)) error
	StateRebuild(d state.Doc) (err error)
	sync.Locker
}

type Source interface {
	Id() string
	Anytype() core.Service
	Type() model.SmartBlockType
	Virtual() bool
	LogHeads() map[string]string

	ReadOnly() bool
	ReadDoc(receiver ChangeReceiver, empty bool) (doc state.Doc, err error)
	ReadMeta(receiver ChangeReceiver) (doc state.Doc, err error)
	PushChange(params PushChangeParams) (id string, err error)
	FindFirstChange(ctx context.Context) (c *change.Change, err error)
	Close() (err error)
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
	case smartblock.SmartBlockTypeBundledRelation, smartblock.SmartBlockTypeIndexedRelation:
		return &bundledRelation{a: s.anytype}, nil
	case smartblock.SmartBlockTypeWorkspaceOld:
		return &threadDB{a: s.anytype}, nil
	case smartblock.SmartBlockTypeBundledTemplate:
		return s.NewStaticSource("", model.SmartBlockType_BundledTemplate, nil), nil
	default:
		if err := blockType.Valid(); err != nil {
			return nil, err
		} else {
			return &source{a: s.anytype, smartblockType: blockType}, nil
		}
	}
}

func newSource(a core.Service, ss status.Service, tid thread.ID, listenToOwnChanges bool) (s Source, err error) {
	id := tid.String()
	sb, err := a.GetBlock(id)
	if err != nil {
		if err == logstore.ErrThreadNotFound {
			return nil, ErrObjectNotFound
		}
		err = fmt.Errorf("anytype.GetBlock error: %w", err)
		return
	}

	sbt, err := smartblock.SmartBlockTypeFromID(id)
	if err != nil {
		return nil, err
	}

	s = &source{
		id:                       id,
		smartblockType:           sbt,
		tid:                      tid,
		a:                        a,
		ss:                       ss,
		sb:                       sb,
		listenToOwnDeviceChanges: listenToOwnChanges,
		logId:                    a.Device(),
		openedAt:                 time.Now(),
	}
	return
}

type source struct {
	id, logId                string
	tid                      thread.ID
	smartblockType           smartblock.SmartBlockType
	a                        core.Service
	ss                       status.Service
	sb                       core.SmartBlock
	tree                     *change.Tree
	lastSnapshotId           string
	logHeads                 map[string]*change.Change
	receiver                 ChangeReceiver
	unsubscribe              func()
	metaOnly                 bool
	listenToOwnDeviceChanges bool // false means we will ignore own(same-logID) changes in applyRecords
	closed                   chan struct{}
	openedAt                 time.Time
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
	return model.SmartBlockType(s.sb.Type())
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
	batch := mb.New(0)
	if receiver != nil {
		s.receiver = receiver
		ch = make(chan core.SmartblockRecordEnvelope)
		if s.unsubscribe, err = s.sb.SubscribeForRecords(ch); err != nil {
			return
		}
		go func() {
			defer batch.Close()
			for rec := range ch {
				batch.Add(rec)
			}
		}()
		defer func() {
			if err != nil {
				batch.Close()
				s.unsubscribe()
				s.unsubscribe = nil
			}
		}()
	}
	startTime := time.Now()
	if s.metaOnly {
		s.tree, s.logHeads, err = change.BuildMetaTree(s.sb)
	} else {
		s.tree, s.logHeads, err = change.BuildTree(s.sb)
	}
	treeBuildTime := time.Now().Sub(startTime).Milliseconds()
	// if the build time is large enough we should record it
	if treeBuildTime > 100 {
		metrics.SharedClient.RecordEvent(metrics.TreeBuild{
			TimeMs:   treeBuildTime,
			ObjectId: s.id,
		})
	}

	if allowEmpty && err == change.ErrEmpty {
		err = nil
		s.tree = new(change.Tree)
		doc = state.NewDoc(s.id, nil)
		InjectCreationInfo(s, doc.(*state.State))
		doc.(*state.State).InjectDerivedDetails()
	} else if err != nil {
		log.With("thread", s.id).Errorf("buildTree failed: %s", err.Error())
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
		go s.changeListener(batch)
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
	st.BlocksInit(st)
	storedDetails, _ := s.Anytype().ObjectStore().GetDetails(s.Id())

	// inject also derived keys, because it may be a good idea to have created date and creator cached so we don't need to traverse changes every time
	InjectLocalDetails(st, pbtypes.StructFilterKeys(storedDetails.GetDetails(), append(bundle.LocalRelationsKeys, bundle.DerivedRelationsKeys...)))
	InjectCreationInfo(s, st)
	st.InjectDerivedDetails()
	if err = st.Normalize(false); err != nil {
		return
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

	if pbtypes.HasField(st.LocalDetails(), bundle.RelationKeyCreator.String()) {
		return nil
	}

	var (
		createdDate = time.Now().Unix()
		// todo: remove this default
		createdBy = s.Anytype().Account()
	)
	// protect from the big documents with a large trees
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	start := time.Now()
	fc, err := s.FindFirstChange(ctx)
	if err == change.ErrEmpty {
		err = nil
		createdBy = s.Anytype().Account()
		log.Debugf("InjectCreationInfo set for the empty object")
	} else if err != nil {
		return fmt.Errorf("failed to find first change to derive creation info")
	} else {
		createdDate = fc.Timestamp
		createdBy = fc.Account
	}
	spent := time.Since(start).Seconds()
	if spent > 0.05 {
		log.Warnf("Calculate creation info %s: %.2fs", s.Id(), time.Since(start).Seconds())
	}

	st.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Float64(float64(createdDate)))
	if profileId, e := threads.ProfileThreadIDFromAccountAddress(createdBy); e == nil {
		st.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(profileId.String()))
	}
	return
}

func InjectLocalDetails(st *state.State, localDetails *types.Struct) {
	for key, v := range localDetails.GetFields() {
		if v == nil {
			continue
		}
		if _, isNull := v.Kind.(*types.Value_NullValue); isNull {
			continue
		}
		st.SetLocalDetail(key, v)
		if !pbtypes.HasRelation(st.ExtraRelations(), key) {
			st.SetExtraRelation(bundle.MustGetRelation(bundle.RelationKey(key)))
		}
	}
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
				ExtraRelations: params.State.ExtraRelations(),
				ObjectTypes:    params.State.ObjectTypes(),
				Collections:    params.State.Store(),
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

func (s *source) getFileHashesForSnapshot(changeHashes []string) []*pb.ChangeFileKeys {
	fileKeys := s.getFileKeysByHashes(changeHashes)
	var uniqKeys = make(map[string]struct{})
	for _, fk := range fileKeys {
		uniqKeys[fk.Hash] = struct{}{}
	}
	s.tree.Iterate(s.tree.RootId(), func(c *change.Change) (isContinue bool) {
		for _, fk := range c.FileKeys {
			if _, ok := uniqKeys[fk.Hash]; !ok {
				uniqKeys[fk.Hash] = struct{}{}
				fileKeys = append(fileKeys, fk)
			}
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
			return
		}
		if c, err = change.NewChangeFromRecord(*rec); err != nil {
			return
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
