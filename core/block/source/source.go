package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/golang/snappy"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	dataTypeSnappy   = "1/s"
	poolSize         = 4096
	snappyLowerLimit = 64
	changeSizeLimit  = 10 * 1024 * 1024
)

var (
	log = logging.Logger("anytype-mw-source")

	bytesPool = sync.Pool{New: func() any { return make([]byte, poolSize) }}

	ErrObjectNotFound = errors.New("object not found")
	ErrReadOnly       = errors.New("object is read only")
	ErrBigChangeSize  = errors.New("change size is above the limit")
)

func MarshalChange(change *pb.Change) (result []byte, dataType string, err error) {
	data := bytesPool.Get().([]byte)[:0]
	defer bytesPool.Put(data)

	data = slices.Grow(data, change.Size())
	n, err := change.MarshalTo(data)
	if err != nil {
		return
	}
	data = data[:n]

	if n > snappyLowerLimit {
		result = snappy.Encode(nil, data)
		dataType = dataTypeSnappy
	} else {
		result = bytes.Clone(data)
	}

	return
}

func UnmarshalChange(treeChange *objecttree.Change, data []byte) (result any, err error) {
	change := &pb.Change{}
	if treeChange.DataType == dataTypeSnappy {
		buf := bytesPool.Get().([]byte)[:0]
		defer bytesPool.Put(buf)

		var n int
		if n, err = snappy.DecodedLen(data); err == nil {
			buf = slices.Grow(buf, n)[:n]
			var decoded []byte
			decoded, err = snappy.Decode(buf, data)
			if err == nil {
				data = decoded
			}
		}
	}
	if err = proto.Unmarshal(data, change); err == nil {
		result = change
	}
	return
}

func UnmarshalChangeWithDataType(dataType string, decrypted []byte) (res any, err error) {
	return UnmarshalChange(&objecttree.Change{DataType: dataType}, decrypted)
}

type ChangeReceiver interface {
	StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error)) error
	StateRebuild(d state.Doc) (err error)
}

type Source interface {
	Id() string
	SpaceID() string
	Type() smartblock.SmartBlockType
	Heads() []string
	GetFileKeysSnapshot() []*pb.ChangeFileKeys
	ReadOnly() bool
	ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error)
	PushChange(params PushChangeParams) (id string, err error)
	Close() (err error)
	GetCreationInfo() (creatorObjectId string, createdDate int64, err error)
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

func (s *service) newTreeSource(ctx context.Context, space Space, id string, buildOpts objecttreebuilder.BuildTreeOpts) (Source, error) {
	treeBuilder := space.TreeBuilder()
	if treeBuilder == nil {
		return nil, fmt.Errorf("space doesn't have tree builder")
	}
	ot, err := space.TreeBuilder().BuildTree(ctx, id, buildOpts)
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	sbt, _, err := typeprovider.GetTypeAndKeyFromRoot(ot.Header())
	if err != nil {
		return nil, err
	}

	return &source{
		ObjectTree:         ot,
		id:                 id,
		space:              space,
		spaceID:            space.Id(),
		smartblockType:     sbt,
		accountService:     s.accountService,
		accountKeysService: s.accountKeysService,
		sbtProvider:        s.sbtProvider,
		fileService:        s.fileService,
		objectStore:        s.objectStore,
		fileObjectMigrator: s.fileObjectMigrator,
	}, nil
}

type ObjectTreeProvider interface {
	Tree() objecttree.ObjectTree
}

type fileObjectMigrator interface {
	MigrateFiles(st *state.State, spc Space, keysChanges []*pb.ChangeFileKeys)
	MigrateFileIdsInDetails(st *state.State, spc Space)
}

type Store interface {
	GetRelationByKey(spaceId string, key string) (*model.Relation, error)
	QueryByID(ids []string) (records []database.Record, err error)
}

type source struct {
	objecttree.ObjectTree
	id                   string
	space                Space
	spaceID              string
	smartblockType       smartblock.SmartBlockType
	lastSnapshotId       string
	changesSinceSnapshot int
	receiver             ChangeReceiver
	unsubscribe          func()
	closed               chan struct{}

	fileService        files.Service
	accountService     accountService
	accountKeysService accountservice.Service
	sbtProvider        typeprovider.SmartBlockTypeProvider
	objectStore        Store
	fileObjectMigrator fileObjectMigrator
}

var _ updatelistener.UpdateListener = (*source)(nil)

func (s *source) Tree() objecttree.ObjectTree {
	return s.ObjectTree
}

func (s *source) Update(ot objecttree.ObjectTree) {
	// here it should work, because we always have the most common snapshot of the changes in tree
	s.lastSnapshotId = ot.Root().Id
	prevSnapshot := s.lastSnapshotId
	// todo: check this one
	err := s.receiver.StateAppend(func(d state.Doc) (st *state.State, changes []*pb.ChangeContent, err error) {
		// State will be applied later in smartblock.StateAppend
		st, changes, sinceSnapshot, err := BuildState(s.spaceID, d.(*state.State), ot, false)
		if err != nil {
			return
		}
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
	st := doc.(*state.State)
	err = s.receiver.StateRebuild(st)
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

func (s *source) SpaceID() string {
	return s.spaceID
}

func (s *source) Type() smartblock.SmartBlockType {
	return s.smartblockType
}

func (s *source) ReadDoc(_ context.Context, receiver ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	return s.readDoc(receiver)
}

func (s *source) readDoc(receiver ChangeReceiver) (doc state.Doc, err error) {
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
	st, _, changesAppliedSinceSnapshot, err := BuildState(s.spaceID, nil, s.ObjectTree, true)
	if err != nil {
		return
	}

	validationErr := st.Validate()
	if validationErr != nil {
		log.With("objectID", s.id).Errorf("not valid state: %v", validationErr)
	}
	st.BlocksInit(st)

	// This is temporary migration. We will move it to persistent migration later after several releases.
	// The reason is to minimize the number of glitches for users of both old and new versions of Anytype.
	// For example, if we persist this migration for Dataview block now, user will see "No query selected"
	// error in the old version of Anytype. We want to avoid this as much as possible by making this migration
	// temporary, though the applying change to this Dataview block will persist this migration, breaking backward
	// compatibility. But in many cases we expect that users update object not so often as they just view them.
	// TODO: we can skip migration for non-personal spaces
	migration := NewSubObjectsAndProfileLinksMigration(s.smartblockType, s.space, s.accountService.MyParticipantId(s.spaceID), s.objectStore)
	migration.Migrate(st)

	// we need to have required internal relations for all objects, including system
	st.AddBundledRelationLinks(bundle.RequiredInternalRelations...)
	if s.Type() == smartblock.SmartBlockTypePage || s.Type() == smartblock.SmartBlockTypeProfilePage {
		template.WithAddedFeaturedRelation(bundle.RelationKeyBacklinks)(st)
		template.WithRelations([]domain.RelationKey{bundle.RelationKeyBacklinks})(st)
	}

	s.fileObjectMigrator.MigrateFiles(st, s.space, s.GetFileKeysSnapshot())
	// Details in spaceview comes from Workspace object, so we don't need to migrate them
	if s.Type() != smartblock.SmartBlockTypeSpaceView {
		s.fileObjectMigrator.MigrateFileIdsInDetails(st, s.space)
	}

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

func (s *source) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	root := s.ObjectTree.Root()
	createdDate = root.Timestamp

	header := s.ObjectTree.UnmarshalledHeader()
	if header != nil && header.Timestamp != 0 && header.Timestamp < createdDate {
		createdDate = header.Timestamp
	}
	if root != nil && root.Identity != nil {
		creatorObjectId = domain.NewParticipantId(s.spaceID, root.Identity.Account())
	}
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
	change := s.buildChange(params)

	data, dataType, err := MarshalChange(change)
	if err != nil {
		return
	}
	if err = checkChangeSize(data, changeSizeLimit); err != nil {
		log.With("objectID", params.State.RootId()).
			Errorf("change size (%d bytes) is above the limit of %d bytes", len(data), changeSizeLimit)
		return "", err
	}

	addResult, err := s.ObjectTree.AddContent(context.Background(), objecttree.SignableChangeContent{
		Data:        data,
		Key:         s.accountKeysService.Account().SignKey,
		IsSnapshot:  change.Snapshot != nil,
		IsEncrypted: true,
		DataType:    dataType,
		Timestamp:   params.Time.Unix(),
	})
	if err != nil {
		return
	}
	id = addResult.Heads[0]

	if change.Snapshot != nil {
		s.lastSnapshotId = id
		s.changesSinceSnapshot = 0
		log.Debugf("%s: pushed snapshot", s.id)
	} else {
		s.changesSinceSnapshot++
		log.Debugf("%s: pushed %d changes", s.id, len(change.Content))
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
				Blocks:                   params.State.BlocksToSave(),
				Details:                  params.State.Details(),
				ObjectTypes:              domain.MarshalTypeKeys(params.State.ObjectTypeKeys()),
				Collections:              params.State.Store(),
				RelationLinks:            params.State.PickRelationLinks(),
				Key:                      params.State.UniqueKeyInternal(),
				OriginalCreatedTimestamp: params.State.OriginalCreatedTimestamp(),
				FileInfo:                 params.State.GetFileInfo().ToModel(),
			},
			FileKeys: s.getFileHashesForSnapshot(params.FileChangedHashes),
		}
	}
	c.Content = params.Changes
	c.FileKeys = s.getFileKeysByHashes(params.FileChangedHashes)
	return c
}

func checkChangeSize(data []byte, maxSize int) error {
	log.Debugf("Change size is %d bytes", len(data))
	if len(data) > maxSize {
		return ErrBigChangeSize
	}
	return nil
}

func (s *source) ListIds() (ids []string, err error) {
	if s.space == nil {
		return
	}
	ids = slice.Filter(s.space.StoredIds(), func(id string) bool {
		t, err := s.sbtProvider.Type(s.spaceID, id)
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
	err := s.ObjectTree.IterateRoot(UnmarshalChange, func(c *objecttree.Change) (isContinue bool) {
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
		fk, err := s.fileService.FileGetKeys(domain.FileId(h))
		if err != nil {
			// New file
			log.Debugf("can't get file key for hash: %v: %v", h, err)
			continue
		}
		// Migrated file
		fileKeys = append(fileKeys, &pb.ChangeFileKeys{
			Hash: fk.FileId.String(),
			Keys: fk.EncryptionKeys,
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

func BuildState(spaceId string, initState *state.State, ot objecttree.ReadableObjectTree, applyState bool) (st *state.State, appliedContent []*pb.ChangeContent, changesAppliedSinceSnapshot int, err error) {
	var (
		startId    string
		lastChange *objecttree.Change
		count      int
	)
	// if the state has no first change
	if initState == nil {
		startId = ot.Root().Id
	} else {
		st = newState(st, initState)
		startId = st.ChangeId()
	}

	// todo: can we avoid unmarshaling here? we already had this data
	_, uniqueKeyInternalKey, err := typeprovider.GetTypeAndKeyFromRoot(ot.Header())
	if err != nil {
		return
	}
	var lastMigrationVersion uint32
	err = ot.IterateFrom(startId, UnmarshalChange,
		func(change *objecttree.Change) bool {
			count++
			lastChange = change
			// that means that we are starting from tree root
			if change.Id == ot.Id() {
				if uniqueKeyInternalKey != "" {
					st = newState(st, state.NewDocWithInternalKey(ot.Id(), nil, uniqueKeyInternalKey).(*state.State))
				} else {
					st = newState(st, state.NewDoc(ot.Id(), nil).(*state.State))
				}
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
					st = newState(st, state.NewDocFromSnapshot(ot.Id(), model.Snapshot, state.WithChangeId(startId), state.WithInternalKey(uniqueKeyInternalKey)).(*state.State))
				} else {
					st = newState(st, st.NewState())
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
	if applyState {
		_, _, err = state.ApplyStateFastOne(st)
		if err != nil {
			return
		}
	}

	if lastChange != nil && !st.IsTheHeaderChange() {
		st.SetLastModified(lastChange.Timestamp, domain.NewParticipantId(spaceId, lastChange.Identity.Account()))
	}
	st.SetMigrationVersion(lastMigrationVersion)
	return
}

func newState(st *state.State, toAssign *state.State) *state.State {
	st = toAssign
	return st
}
