package source

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/cheggaaa/mb"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/logstore"
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
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
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
	ReadMeta(ctx context.Context, receiver ChangeReceiver) (doc state.Doc, err error)
	PushChange(params PushChangeParams) (id string, err error)
	FindFirstChange(ctx context.Context) (c *change.Change, err error)
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
	changesSinceSnapshot     int
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

func (s *source) ReadMeta(ctx context.Context, receiver ChangeReceiver) (doc state.Doc, err error) {
	s.metaOnly = true
	return s.readDoc(ctx, receiver, false)
}

func (s *source) ReadDoc(ctx context.Context, receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	return s.readDoc(ctx, receiver, allowEmpty)
}

func (s *source) readDoc(ctx context.Context, receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
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
	log.With("thread", s.id).
		Debug("start building tree")
	loadCh := make(chan struct{})

	ctxP := core.ThreadLoadProgress{}
	ctx = ctxP.DeriveContext(ctx)

	var request string
	if v := ctx.Value(metrics.CtxKeyRequest); v != nil {
		request = v.(string)
	}
	sendEvent := func(v core.ThreadLoadProgress, inProgress bool) {
		logs, _ := s.sb.GetLogs()
		spent := time.Since(startTime).Seconds()
		var msg string
		if inProgress {
			msg = "tree building in progress"
		} else {
			msg = "tree building finished"
		}

		l := log.With("thread", s.id).
			With("sb_type", s.smartblockType).
			With("request", request).
			With("logs", logs).
			With("records_loaded", v.RecordsLoaded).
			With("records_missing", v.RecordsMissingLocally).
			With("spent", spent)

		if spent > 30 {
			l.Errorf(msg)
		} else if spent > 3 {
			l.Warn(msg)
		} else {
			l.Debug(msg)
		}

		event := metrics.TreeBuild{
			SbType:         uint64(s.smartblockType),
			TimeMs:         time.Since(startTime).Milliseconds(),
			ObjectId:       s.id,
			Request:        request,
			InProgress:     inProgress,
			Logs:           len(logs),
			RecordsFailed:  v.RecordsFailedToLoad,
			RecordsLoaded:  v.RecordsLoaded,
			RecordsMissing: v.RecordsMissingLocally,
		}

		metrics.SharedClient.RecordEvent(event)
	}

	go func() {
		tDuration := time.Second * 10
		var v core.ThreadLoadProgress
	forloop:
		for {
			select {
			case <-loadCh:
				break forloop
			case <-time.After(tDuration):
				v2 := ctxP.Value()
				if v2.RecordsLoaded == v.RecordsLoaded && v2.RecordsMissingLocally == v.RecordsMissingLocally && v2.RecordsFailedToLoad == v.RecordsFailedToLoad {
					// no progress, double the ticker
					tDuration = tDuration * 2
				}
				v = v2
				sendEvent(v, true)
			}
		}
	}()
	if s.metaOnly {
		s.tree, s.logHeads, err = change.BuildMetaTree(ctx, s.sb)
	} else {
		s.tree, s.logHeads, err = change.BuildTree(ctx, s.sb)
	}
	close(loadCh)
	treeBuildTime := time.Now().Sub(startTime).Milliseconds()

	// if the build time is large enough we should record it
	if treeBuildTime > 100 {
		sendEvent(ctxP.Value(), false)
	}

	if allowEmpty && err == change.ErrEmpty {
		err = nil
		s.tree = new(change.Tree)
		doc = state.NewDoc(s.id, nil)
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
	st, changesApplied, err := change.BuildStateSimpleCRDT(doc.(*state.State), s.tree)
	if err != nil {
		return
	}
	s.changesSinceSnapshot = changesApplied

	if s.sb.Type() != smartblock.SmartBlockTypeArchive && !s.Virtual() {
		if verr := st.Validate(); verr != nil {
			log.With("thread", s.id).With("sbType", s.sb.Type()).Errorf("not valid state: %v", verr)
		}
	}
	st.BlocksInit(st)
	st.InjectDerivedDetails()

	// TODO: check if we can leave only removeDuplicates instead of Normalize
	if _, err = st.Normalize(false); err != nil {
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

	createdDate = time.Now().Unix()
	createdBy := s.Anytype().Account()

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
		return "", 0, fmt.Errorf("failed to find first change to derive creation info")
	} else {
		createdDate = fc.Timestamp
		createdBy = fc.Account
	}
	spent := time.Since(start).Seconds()
	if spent > 0.05 {
		log.Warnf("Calculate creation info %s: %.2fs", s.Id(), time.Since(start).Seconds())
	}

	if profileId, e := threads.ProfileThreadIDFromAccountAddress(createdBy); e == nil {
		creator = profileId.String()
	}
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
		if s.tree.Len() > 0 {
			log.With("thread", s.id).With("len", s.tree.Len(), "lenSnap", s.changesSinceSnapshot, "changes", len(params.Changes), "doSnap", params.DoSnapshot).Warnf("do the snapshot")
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
		s.changesSinceSnapshot = 0
		log.Infof("%s: pushed snapshot", s.id)
	} else {
		s.changesSinceSnapshot++
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

func snapshotChance(changesSinceSnapshot int) bool {
	v := 2000
	if changesSinceSnapshot <= 100 {
		return false
	}

	d := changesSinceSnapshot/50 + 1

	min := (v / 2) - d
	max := (v / 2) + d

	r := rand.Intn(v)
	if r >= min && r <= max {
		return true
	}

	return false
}

func (s *source) needSnapshot() bool {
	if s.tree.Len() == 0 {
		// starting tree with snapshot
		return true
	}
	return snapshotChance(s.changesSinceSnapshot)
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
			if ch.Snapshot != nil {
				s.changesSinceSnapshot = 0
			} else {
				s.changesSinceSnapshot++
			}
			changesContent = append(changesContent, ch.Content...)
		}
		s.lastSnapshotId = s.tree.LastSnapshotId(context.TODO())
		return s.receiver.StateAppend(func(d state.Doc) (*state.State, error) {
			st, _, err := change.BuildStateSimpleCRDT(d.(*state.State), s.tree)
			return st, err
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
