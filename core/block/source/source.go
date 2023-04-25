package source

import (
	"context"
	"errors"
	"fmt"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/any-sync/commonspace/object/tree/synctree"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"
	"math/rand"
	"sync"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/status"
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
	StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error)) error
	StateRebuild(d state.Doc) (err error)
	sync.Locker
}

type Source interface {
	Id() string
	Anytype() core.Service
	Type() model.SmartBlockType
	Virtual() bool
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

type sourceDeps struct {
	anytype        core.Service
	statusService  status.Service
	accountService accountservice.Service
	sbt            smartblock.SmartBlockType
	ot             objecttree.ObjectTree
}

func newTreeSource(id string, deps sourceDeps) (s Source, err error) {
	return &source{
		ObjectTree:     deps.ot,
		id:             id,
		a:              deps.anytype,
		ss:             deps.statusService,
		logId:          deps.anytype.Device(),
		openedAt:       time.Now(),
		smartblockType: deps.sbt,
		acc:            deps.accountService,
	}, nil
}

type ObjectTreeProvider interface {
	Tree() objecttree.ObjectTree
}

type source struct {
	objecttree.ObjectTree
	id, logId      string
	tid            thread.ID
	smartblockType smartblock.SmartBlockType
	a              core.Service
	ss             status.Service
	lastSnapshotId string
	receiver       ChangeReceiver
	unsubscribe    func()
	metaOnly       bool
	closed         chan struct{}
	openedAt       time.Time
	acc            accountservice.Service
}

func (s *source) Tree() objecttree.ObjectTree {
	return s.ObjectTree
}

func (s *source) Update(ot objecttree.ObjectTree) {
	// here it should work, because we always have the most common snapshot of the changes in tree
	s.lastSnapshotId = ot.Root().Id
	err := s.receiver.StateAppend(func(d state.Doc) (st *state.State, changes []*pb.ChangeContent, err error) {
		return BuildState(d.(*state.State), ot, s.Anytype().PredefinedBlocks().Profile)
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
	st, _, err := BuildState(nil, s.ObjectTree, s.Anytype().PredefinedBlocks().Profile)
	if err != nil {
		return
	}
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
	return st, nil
}

func (s *source) GetCreationInfo() (creator string, createdDate int64, err error) {
	if s.Anytype() == nil {
		return "", 0, fmt.Errorf("anytype is nil")
	}

	createdDate = s.ObjectTree.UnmarshalledHeader().Timestamp
	// TODO: add creator in profile
	return
}

type PushChangeParams struct {
	State             *state.State
	Changes           []*pb.ChangeContent
	FileChangedHashes []string
	DoSnapshot        bool
}

func (s *source) PushChange(params PushChangeParams) (id string, err error) {
	c := &pb.Change{
		Timestamp: time.Now().Unix(),
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
	data, err := c.Marshal()
	if err != nil {
		return
	}
	addResult, err := s.ObjectTree.AddContent(context.Background(), objecttree.SignableChangeContent{
		Data:        data,
		Key:         s.acc.Account().SignKey,
		Identity:    s.acc.Account().Identity,
		IsSnapshot:  c.Snapshot != nil,
		IsEncrypted: true,
	})
	if err != nil {
		return
	}
	id = addResult.Heads[0]

	if c.Snapshot != nil {
		s.lastSnapshotId = id
		log.Infof("%s: pushed snapshot", s.id)
	} else {
		log.Debugf("%s: pushed %d changes", s.id, len(c.Content))
	}
	return
}

func (s *source) ListIds() (ids []string, err error) {
	spc, err := s.a.SpaceService().AccountSpace(context.Background())
	if err != nil {
		return
	}
	ids = slice.Filter(spc.StoredIds(), func(id string) bool {
		if s.Anytype().PredefinedBlocks().IsAccount(id) {
			return false
		}
		t, err := smartblock.SmartBlockTypeFromID(id)
		if err != nil {
			return false
		}
		return t == s.smartblockType
	})
	// exclude account thread id
	return ids, nil
}

func (s *source) needSnapshot() bool {
	if s.ObjectTree.Heads()[0] == s.ObjectTree.Id() {
		return true
	}
	return rand.Intn(500) == 42
}

func (s *source) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return s.getFileHashesForSnapshot(nil)
}

func (s *source) iterate(startId string, iterFunc objecttree.ChangeIterateFunc) (err error) {
	unmarshall := func(decrypted []byte) (res any, err error) {
		ch := &pb.Change{}
		err = proto.Unmarshal(decrypted, ch)
		res = ch
		return
	}
	return s.ObjectTree.IterateFrom(startId, unmarshall, iterFunc)
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
	err := s.iterate(s.ObjectTree.Root().Id, func(c *objecttree.Change) (isContinue bool) {
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

func BuildState(initState *state.State, ot objecttree.ReadableObjectTree, profileId string) (st *state.State, appliedContent []*pb.ChangeContent, err error) {
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
				st = state.NewDoc(ot.Id(), nil).(*state.State)
				st.SetChangeId(change.Id)
				return true
			}

			model := change.Model.(*pb.Change)
			if startId == change.Id {
				if st == nil {
					st = state.NewDocFromSnapshot(ot.Id(), model.Snapshot).(*state.State)
					st.SetChangeId(startId)
					return true
				} else {
					st = st.NewState()
				}
				return true
			}
			ns := st.NewState()
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
		st.SetLastModified(lastChange.Timestamp, profileId)
	}
	return
}
