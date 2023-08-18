package source

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/golang/snappy"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	defaultDataType  = "1/s"
	poolSize         = 4096
	snappyLowerLimit = 64
)

var (
	log = logging.Logger("anytype-mw-source")

	bytesPool = sync.Pool{New: func() any { return make([]byte, poolSize) }}

	ErrObjectNotFound = errors.New("object not found")
	ErrReadOnly       = errors.New("object is read only")
)

func MarshallChange(c *pb.Change) (res []byte, dataType string, err error) {
	data := bytesPool.Get().([]byte)[:0]
	defer bytesPool.Put(data)

	data = slices.Grow(data, c.Size())
	n, err := c.MarshalTo(data)
	if err != nil {
		return
	}
	data = data[:n]

	if n > snappyLowerLimit {
		res = snappy.Encode(nil, data)
		log.Debugf("change is shrunk by snappy from %d bytes to %d bytes. Space saving: %.2f%%",
			len(data), len(res), 100*(1-float32(len(res))/float32(len(data))))
		dataType = defaultDataType
	} else {
		res = data
	}

	return
}

func UnmarshallChange(c *objecttree.Change, data []byte) (res any, err error) {
	ch := &pb.Change{}
	switch c.DataType {
	case defaultDataType:
		buf := bytesPool.Get().([]byte)[:0]
		defer bytesPool.Put(buf)

		n, dErr := snappy.DecodedLen(data)
		buf = slices.Grow(buf, n)
		var decoded []byte
		decoded, err = snappy.Decode(buf, data)
		if err == nil && dErr == nil {
			data = decoded
		}
	}
	if err = proto.Unmarshal(data, ch); err == nil {
		res = ch
	}
	return
}

type ChangeReceiver interface {
	StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error)) error
	StateRebuild(d state.Doc) (err error)
	sync.Locker
}

type Source interface {
	Id() string
	Type() model.SmartBlockType
	Heads() []string
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

type IDsLister interface {
	ListIds() ([]string, error)
}

type SourceWithType interface {
	Source
	IDsLister
}

var ErrUnknownDataFormat = fmt.Errorf("unknown data format: you may need to upgrade anytype in order to open this page")

type sourceDeps struct {
	sbt smartblock.SmartBlockType
	ot  objecttree.ObjectTree

	coreService    core.Service
	accountService accountservice.Service
	spaceService   space.Service
	sbtProvider    typeprovider.SmartBlockTypeProvider
	fileService    files.Service
}

func newTreeSource(id string, deps sourceDeps) (s Source, err error) {
	return &source{
		ObjectTree:     deps.ot,
		id:             id,
		coreService:    deps.coreService,
		spaceService:   deps.spaceService,
		openedAt:       time.Now(),
		smartblockType: deps.sbt,
		accountService: deps.accountService,
		sbtProvider:    deps.sbtProvider,
		fileService:    deps.fileService,
	}, nil
}

type ObjectTreeProvider interface {
	Tree() objecttree.ObjectTree
}

type source struct {
	objecttree.ObjectTree
	id                   string
	smartblockType       smartblock.SmartBlockType
	lastSnapshotId       string
	changesSinceSnapshot int
	receiver             ChangeReceiver
	unsubscribe          func()
	metaOnly             bool
	closed               chan struct{}
	openedAt             time.Time

	coreService    core.Service
	fileService    files.Service
	accountService accountservice.Service
	spaceService   space.Service
	sbtProvider    typeprovider.SmartBlockTypeProvider
}

func (s *source) Tree() objecttree.ObjectTree {
	return s.ObjectTree
}

func (s *source) Update(ot objecttree.ObjectTree) {
	// here it should work, because we always have the most common snapshot of the changes in tree
	s.lastSnapshotId = ot.Root().Id
	prevSnapshot := s.lastSnapshotId
	// todo: check this one
	err := s.receiver.StateAppend(func(d state.Doc) (st *state.State, changes []*pb.ChangeContent, err error) {
		st, changes, sinceSnapshot, err := BuildStateFull(d.(*state.State), ot, s.coreService.PredefinedBlocks().Profile)
		if prevSnapshot != s.lastSnapshotId {
			s.changesSinceSnapshot = sinceSnapshot
		} else {
			s.changesSinceSnapshot += sinceSnapshot
		}
		return st, changes, err
	})

	if err != nil {
		log.With(zap.Error(err)).Debug("failed to append the state and send it to receiver")
	}
}

func (s *source) Rebuild(ot objecttree.ObjectTree) {
	if s.ObjectTree == nil {
		return
	}

	doc, err := s.buildState()
	if err != nil {
		log.With(zap.Error(err)).Debug("failed to build state")
		return
	}
	err = s.receiver.StateRebuild(doc.(*state.State))
	if err != nil {
		log.With(zap.Error(err)).Debug("failed to send the state to receiver")
	}
}

func (s *source) ReadOnly() bool {
	return false
}

func (s *source) Id() string {
	return s.id
}

func (s *source) Type() model.SmartBlockType {
	return model.SmartBlockType(s.smartblockType)
}

func (s *source) ReadDoc(ctx context.Context, receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	return s.readDoc(ctx, receiver, allowEmpty)
}

func (s *source) readDoc(ctx context.Context, receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	s.receiver = receiver
	setter, ok := s.ObjectTree.(synctree.ListenerSetter)
	if !ok {
		err = fmt.Errorf("should be able to set listner inside object tree")
		return
	}
	setter.SetListener(s)
	return s.buildState()
}

func (s *source) buildState() (doc state.Doc, err error) {
	st, _, changesAppliedSinceSnapshot, err := BuildState(nil, s.ObjectTree, s.coreService.PredefinedBlocks().Profile)
	if err != nil {
		return
	}
	validationErr := st.Validate()
	if validationErr != nil {
		log.With("objectID", s.id).Errorf("not valid state: %v", validationErr)
	}
	st.BlocksInit(st)
	st.InjectDerivedDetails()
	s.changesSinceSnapshot = changesAppliedSinceSnapshot
	// TODO: check if we can leave only removeDuplicates instead of Normalize
	if err = st.Normalize(false); err != nil {
		return
	}

	// TODO: check if we can use apply fast one
	if _, _, err = state.ApplyState(st, false); err != nil {
		return
	}
	return st, nil
}

func (s *source) GetCreationInfo() (creator string, createdDate int64, err error) {
	createdDate = s.ObjectTree.UnmarshalledHeader().Timestamp
	creator = s.coreService.PredefinedBlocks().Profile
	return
}

type PushChangeParams struct {
	State             *state.State
	Changes           []*pb.ChangeContent
	FileChangedHashes []string
	Time              time.Time // used to derive the lastModifiedDate; Default is time.Now()
	DoSnapshot        bool
}

func (s *source) PushChange(params PushChangeParams) (id string, err error) {
	if params.Time.IsZero() {
		params.Time = time.Now()
	}

	c := s.buildChange(params)
	data, dataType, err := MarshallChange(c)
	if err != nil {
		return
	}

	content := objecttree.SignableChangeContent{
		Data:        data,
		Key:         s.accountService.Account().SignKey,
		IsSnapshot:  c.Snapshot != nil,
		IsEncrypted: true,
	}

	if dataType != "" {
		content.DataType = dataType
	}

	addResult, err := s.ObjectTree.AddContent(context.Background(), content)
	if err != nil {
		return
	}
	id = addResult.Heads[0]

	if c.Snapshot != nil {
		s.lastSnapshotId = id
		s.changesSinceSnapshot = 0
		log.Infof("%s: pushed snapshot", s.id)
	} else {
		s.changesSinceSnapshot++
		log.Debugf("%s: pushed %d changes", s.id, len(c.Content))
	}
	return
}

func (s *source) buildChange(params PushChangeParams) (c *pb.Change) {
	c = &pb.Change{
		Timestamp: params.Time.Unix(),
		Version:   params.State.MigrationVersion(),
	}
	if params.DoSnapshot || s.needSnapshot() || len(params.Changes) == 0 {
		c.Snapshot = &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks:        params.State.BlocksToSave(),
				Details:       params.State.Details(),
				ObjectTypes:   params.State.ObjectTypes(),
				Collections:   params.State.Store(),
				RelationLinks: params.State.PickRelationLinks(),
			},
			FileKeys: s.getFileHashesForSnapshot(params.FileChangedHashes),
		}
	}
	c.Content = params.Changes
	c.FileKeys = s.getFileKeysByHashes(params.FileChangedHashes)
	return c
}

func (s *source) ListIds() (ids []string, err error) {
	spc, err := s.spaceService.AccountSpace(context.Background())
	if err != nil {
		return
	}
	ids = slice.Filter(spc.StoredIds(), func(id string) bool {
		t, err := s.sbtProvider.Type(id)
		if err != nil {
			return false
		}
		return t == s.smartblockType
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
	if s.ObjectTree.Heads()[0] == s.ObjectTree.Id() {
		return true
	}
	return snapshotChance(s.changesSinceSnapshot)
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
	err := s.ObjectTree.IterateRoot(UnmarshallChange, func(c *objecttree.Change) (isContinue bool) {
		model, ok := c.Model.(*pb.Change)
		if !ok {
			return false
		}
		if model.Snapshot != nil && len(model.Snapshot.FileKeys) > 0 {
			processFileKeys(model.Snapshot.FileKeys)
		}
		if len(model.FileKeys) > 0 {
			processFileKeys(model.FileKeys)
		}
		return true
	})
	if err != nil {
		log.With(zap.Error(err)).Debug("failed to iterate through file keys")
	}
	return fileKeys
}

func (s *source) getFileKeysByHashes(hashes []string) []*pb.ChangeFileKeys {
	fileKeys := make([]*pb.ChangeFileKeys, 0, len(hashes))
	for _, h := range hashes {
		fk, err := s.fileService.FileGetKeys(h)
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

func (s *source) Heads() []string {
	if s.ObjectTree == nil {
		return nil
	}
	heads := s.ObjectTree.Heads()
	headsCopy := make([]string, 0, len(heads))
	headsCopy = append(headsCopy, heads...)
	return headsCopy
}

func (s *source) Close() (err error) {
	if s.unsubscribe != nil {
		s.unsubscribe()
		<-s.closed
	}
	return s.ObjectTree.Close()
}

func BuildState(initState *state.State, ot objecttree.ReadableObjectTree, profileId string) (st *state.State, appliedContent []*pb.ChangeContent, changesAppliedSinceSnapshot int, err error) {
	var (
		startId    string
		lastChange *objecttree.Change
		count      int
	)
	// if the state has no first change
	if initState == nil {
		startId = ot.Root().Id
	} else {
		st = initState
		startId = st.ChangeId()
	}

	var lastMigrationVersion uint32
	err = ot.IterateFrom(startId, UnmarshallChange,
		func(change *objecttree.Change) bool {
			count++
			lastChange = change
			// that means that we are starting from tree root
			if change.Id == ot.Id() {
				st = state.NewDoc(ot.Id(), nil).(*state.State)
				st.SetChangeId(change.Id)
				return true
			}

			model := change.Model.(*pb.Change)
			if model.Version > lastMigrationVersion {
				lastMigrationVersion = model.Version
			}
			if startId == change.Id {
				if st == nil {
					changesAppliedSinceSnapshot = 0
					st = state.NewDocFromSnapshot(ot.Id(), model.Snapshot, state.WithChangeId(startId)).(*state.State)
					return true
				} else {
					st = st.NewState()
				}
				return true
			}
			if model.Snapshot != nil {
				changesAppliedSinceSnapshot = 0
			} else {
				changesAppliedSinceSnapshot++
			}
			appliedContent = append(appliedContent, model.Content...)
			st.SetChangeId(change.Id)
			st.ApplyChangeIgnoreErr(model.Content...)
			st.AddFileKeys(model.FileKeys...)

			return true
		})
	if err != nil {
		return
	}
	_, _, err = state.ApplyStateFastOne(st)
	if err != nil {
		return
	}

	if lastChange != nil && !st.IsTheHeaderChange() {
		// todo: why do we don't need to set last modified for the header change?
		st.SetLastModified(lastChange.Timestamp, profileId)
	}
	st.SetMigrationVersion(lastMigrationVersion)
	return
}

// BuildStateFull is deprecated, used in tests only, use BuildState instead
func BuildStateFull(initState *state.State, ot objecttree.ReadableObjectTree, profileId string) (st *state.State, appliedContent []*pb.ChangeContent, changesAppliedSinceSnapshot int, err error) {
	var (
		startId    string
		lastChange *objecttree.Change
		count      int
	)
	// if the state has no first change
	if initState == nil {
		startId = ot.Root().Id
	} else {
		st = initState
		startId = st.ChangeId()
	}

	var lastMigrationVersion uint32
	err = ot.IterateFrom(startId, UnmarshallChange, func(change *objecttree.Change) bool {
		count++
		lastChange = change
		// that means that we are starting from tree root
		if change.Id == ot.Id() {
			st = state.NewDoc(ot.Id(), nil).(*state.State)
			st.SetChangeId(change.Id)
			return true
		}

		model := change.Model.(*pb.Change)
		if model.Version > lastMigrationVersion {
			lastMigrationVersion = model.Version
		}
		if startId == change.Id {
			if st == nil {
				changesAppliedSinceSnapshot = 0
				st = state.NewDocFromSnapshot(ot.Id(), model.Snapshot, state.WithChangeId(startId)).(*state.State)
				return true
			} else {
				st = st.NewState()
			}
			return true
		}
		if model.Snapshot != nil {
			changesAppliedSinceSnapshot = 0
		} else {
			changesAppliedSinceSnapshot++
		}
		ns := st.NewState()
		appliedContent = append(appliedContent, model.Content...)
		ns.SetChangeId(change.Id)
		ns.ApplyChangeIgnoreErr(model.Content...)
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
	if lastChange != nil && !st.IsTheHeaderChange() {
		st.SetLastModified(lastChange.Timestamp, profileId)
	}
	st.SetMigrationVersion(lastMigrationVersion)
	return
}
