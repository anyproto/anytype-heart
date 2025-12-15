package sourceimpl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/updatelistener"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objecthandler"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/reflection"
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

	ErrSpaceWithoutTreeBuilder = errors.New("space doesn't have tree builder")
)

func MarshalChange(change *pb.Change) (result []byte, dataType string, err error) {
	data := bytesPool.Get().([]byte)[:0]
	defer func() {
		bytesPool.Put(data)
	}()

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
	return unmarshalChange(treeChange, data, true)
}

func unmarshalChange(treeChange *objecttree.Change, data []byte, needSnapshot bool) (result any, err error) {
	var change proto.Message
	if needSnapshot {
		change = &pb.Change{}
	} else {
		change = &pb.ChangeNoSnapshot{}
	}
	if treeChange.DataType == dataTypeSnappy {
		buf := bytesPool.Get().([]byte)[:0]
		defer func() {
			bytesPool.Put(buf)
		}()

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
	if err = proto.Unmarshal(data, change); err != nil {
		return
	}
	if needSnapshot {
		return change, nil
	} else {
		noSnapshotChange := change.(*pb.ChangeNoSnapshot)
		return &pb.Change{
			Content:    noSnapshotChange.Content,
			FileKeys:   noSnapshotChange.FileKeys,
			Timestamp:  noSnapshotChange.Timestamp,
			Version:    noSnapshotChange.Version,
			ChangeType: noSnapshotChange.ChangeType,
		}, nil
	}
}

// NewUnmarshalTreeChange creates UnmarshalChange func that unmarshalls snapshot only for the first change and ignores it for following. It saves some memory
func NewUnmarshalTreeChange() objecttree.ChangeConvertFunc {
	var changeCount atomic.Int32
	return func(treeChange *objecttree.Change, data []byte) (result any, err error) {
		return unmarshalChange(treeChange, data, changeCount.CompareAndSwap(0, 1))
	}
}

func UnmarshalChangeWithDataType(dataType string, decrypted []byte) (res any, err error) {
	return UnmarshalChange(&objecttree.Change{DataType: dataType}, decrypted)
}

type SourceIdEndodedDetails interface {
	Id() string
	DetailsFromId() (*domain.Details, error)
}

func (s *service) newTreeSource(ctx context.Context, space source.Space, id string, buildOpts objecttreebuilder.BuildTreeOpts) (source.Source, error) {
	treeBuilder := space.TreeBuilder()
	if treeBuilder == nil {
		return nil, ErrSpaceWithoutTreeBuilder
	}
	ot, err := space.TreeBuilder().BuildTree(ctx, id, buildOpts)
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	sbt, _, err := typeprovider.GetTypeAndKeyFromRoot(ot.Header())
	if err != nil {
		return nil, err
	}

	src := &treeSource{
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
		spaceIndex:         s.objectStore.SpaceIndex(space.Id()),
		fileObjectMigrator: s.fileObjectMigrator,
		formatFetcher:      s.formatFetcher,
	}
	if sbt == smartblock.SmartBlockTypeChatDerivedObject || sbt == smartblock.SmartBlockTypeAccountObject {
		return &store{treeSource: src, sbType: sbt, diffManagers: map[string]*diffManager{}, spaceService: s.spaceService}, nil
	}

	return src, nil
}

type fileObjectMigrator interface {
	MigrateFiles(st *state.State, spc source.Space, keysChanges []*pb.ChangeFileKeys)
	MigrateFileIdsInDetails(st *state.State, spc source.Space)
}

type treeSource struct {
	objecttree.ObjectTree
	id                   string
	space                source.Space
	spaceID              string
	smartblockType       smartblock.SmartBlockType
	lastSnapshotId       string
	changesSinceSnapshot int
	receiver             source.ChangeReceiver
	unsubscribe          func()
	closed               chan struct{}

	fileService        files.Service
	accountService     accountService
	accountKeysService accountservice.Service
	sbtProvider        typeprovider.SmartBlockTypeProvider
	objectStore        objectstore.ObjectStore
	spaceIndex         spaceindex.Store
	fileObjectMigrator fileObjectMigrator
	formatFetcher      relationutils.RelationFormatFetcher
}

var _ updatelistener.UpdateListener = (*treeSource)(nil)

func (s *treeSource) Tree() objecttree.ObjectTree {
	return s.ObjectTree
}

func (s *treeSource) Update(ot objecttree.ObjectTree) error {
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
	return nil
}

func (s *treeSource) Rebuild(ot objecttree.ObjectTree) error {
	if s.ObjectTree == nil {
		return nil
	}

	doc, err := s.buildState()
	if err != nil {
		log.With(zap.Error(err)).Debug("failed to build state")
		return nil
	}
	st := doc.(*state.State)
	err = s.receiver.StateRebuild(st)
	if err != nil {
		log.With(zap.Error(err)).Debug("failed to send the state to receiver")
	}
	return nil
}

func (s *treeSource) ReadOnly() bool {
	return false
}

func (s *treeSource) Id() string {
	return s.id
}

func (s *treeSource) SpaceID() string {
	return s.spaceID
}

func (s *treeSource) Type() smartblock.SmartBlockType {
	return s.smartblockType
}

func (s *treeSource) ReadDoc(_ context.Context, receiver source.ChangeReceiver, allowEmpty bool) (doc state.Doc, err error) {
	return s.readDoc(receiver)
}

func (s *treeSource) readDoc(receiver source.ChangeReceiver) (doc state.Doc, err error) {
	s.receiver = receiver
	setter, ok := s.ObjectTree.(synctree.ListenerSetter)
	if !ok {
		err = fmt.Errorf("should be able to set listner inside object tree")
		return
	}
	setter.SetListener(s)
	return s.buildState()
}

func (s *treeSource) buildState() (doc state.Doc, err error) {
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
	migration := source.NewSubObjectsAndProfileLinksMigration(s.smartblockType, s.space, s.accountService.MyParticipantId(s.spaceID), s.spaceIndex, s.formatFetcher)
	migration.Migrate(st)

	// we need to have required internal relations for all objects, including system
	st.AddBundledRelationLinks(bundle.RequiredInternalRelations...)
	if s.Type() == smartblock.SmartBlockTypePage || s.Type() == smartblock.SmartBlockTypeProfilePage {
		template.WithRelations([]domain.RelationKey{bundle.RelationKeyBacklinks})(st)
		template.WithFeaturedRelationsBlock(st)
	}

	if s.Type() == smartblock.SmartBlockTypeWidget {
		// todo: remove this after 0.41 release
		state.CleanupLayouts(st)
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
	if _, _, err = state.ApplyState(s.spaceID, st, false); err != nil {
		return
	}
	return st, nil
}

func (s *treeSource) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	header := s.ObjectTree.UnmarshalledHeader()
	createdDate = header.Timestamp
	if header.Identity != nil {
		creatorObjectId = domain.NewParticipantId(s.spaceID, header.Identity.Account())
	}
	return
}

func (s *treeSource) PushChange(params source.PushChangeParams) (id string, err error) {
	for _, change := range params.Changes {
		name := reflection.GetChangeContent(change.Value)
		if name == "" {
			log.Errorf("can't detect change content for %s", change.Value)
		} else {
			ev := &metrics.ChangeEvent{
				ChangeName: name,
				SbType:     s.smartblockType.String(),
				Count:      1,
			}
			metrics.Service.SendSampled(ev)
		}
	}

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
		Data:              data,
		Key:               s.ObjectTree.AclList().AclState().Key(),
		IsSnapshot:        change.Snapshot != nil,
		ShouldBeEncrypted: true,
		DataType:          dataType,
		Timestamp:         params.Time.Unix(),
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

func (s *treeSource) buildChange(params source.PushChangeParams) (c *pb.Change) {
	c = &pb.Change{
		Timestamp:  params.Time.Unix(),
		Version:    params.State.MigrationVersion(),
		ChangeType: params.ChangeType.Raw(),
	}
	if params.DoSnapshot || s.needSnapshot() || len(params.Changes) == 0 {
		c.Snapshot = &pb.ChangeSnapshot{
			Data: &model.SmartBlockSnapshotBase{
				Blocks:      params.State.BlocksToSave(),
				Details:     params.State.Details().ToProto(),
				ObjectTypes: domain.MarshalTypeKeys(params.State.ObjectTypeKeys()),
				Collections: params.State.Store(),
				// TODO: GO-4284 We need to use PickRelationLinks here because we build a state.
				// Changes on RelationLinks could go to old clients
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
		return source.ErrBigChangeSize
	}
	return nil
}

func (s *treeSource) ListIds() (ids []string, err error) {
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

func (s *treeSource) needSnapshot() bool {
	if s.ObjectTree.Heads()[0] == s.ObjectTree.Id() {
		return true
	}
	return snapshotChance(s.changesSinceSnapshot)
}

func (s *treeSource) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return s.getFileHashesForSnapshot(nil)
}

func (s *treeSource) getFileHashesForSnapshot(changeHashes []string) []*pb.ChangeFileKeys {
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

func (s *treeSource) getFileKeysByHashes(hashes []string) []*pb.ChangeFileKeys {
	fileKeys := make([]*pb.ChangeFileKeys, 0, len(hashes))
	for _, h := range hashes {
		fileId := domain.FileId(h)
		keys, err := s.objectStore.GetFileKeys(fileId)
		if err != nil {
			// New file
			log.Debugf("can't get file key for hash: %v: %v", h, err)
			continue
		}

		// Migrated file
		fileKeys = append(fileKeys, &pb.ChangeFileKeys{
			Hash: fileId.String(),
			Keys: keys,
		})
	}
	return fileKeys
}

func (s *treeSource) Heads() []string {
	if s.ObjectTree == nil {
		return nil
	}
	heads := s.ObjectTree.Heads()
	headsCopy := make([]string, 0, len(heads))
	headsCopy = append(headsCopy, heads...)
	return headsCopy
}

func (s *treeSource) Close() (err error) {
	if s.unsubscribe != nil {
		s.unsubscribe()
		<-s.closed
	}
	return s.ObjectTree.Close()
}

func cleanUpChange(objectId string, change *objecttree.Change, model *pb.Change) {
	// cover the case of conflicting root snapshots
	// emptying the object name
	// GO-5592
	if len(change.PreviousIds) == 1 &&
		change.PreviousIds[0] == objectId {
		for i, c := range model.Content {
			switch tt := c.Value.(type) {
			case *pb.ChangeContentValueOfDetailsSet:
				if tt.DetailsSet.Key == bundle.RelationKeyName.String() &&
					tt.DetailsSet.Value.GetStringValue() == "" {
					model.Content = append(model.Content[:i], model.Content[i+1:]...)
					return
				}
			}
		}
	}
}

func BuildState(spaceId string, initState *state.State, ot objecttree.ReadableObjectTree, applyState bool) (st *state.State, appliedContent []*pb.ChangeContent, changesAppliedSinceSnapshot int, err error) {
	var (
		startId string
		count   int
	)
	// if the state has no first change
	if initState == nil {
		startId = ot.Root().Id
	} else {
		st = initState
		startId = st.ChangeId()
	}

	// todo: can we avoid unmarshaling here? we already had this data
	sbt, uniqueKeyInternalKey, err := typeprovider.GetTypeAndKeyFromRoot(ot.Header())
	if err != nil {
		return
	}

	sbHandler := objecthandler.GetSmartblockHandler(sbt)

	var iterErr error
	var lastMigrationVersion uint32
	err = ot.IterateFrom(startId, NewUnmarshalTreeChange(),
		func(change *objecttree.Change) bool {
			count++

			// that means that we are starting from the tree root
			if change.Id == ot.Id() {
				if st != nil {
					st = st.NewState()
				} else if uniqueKeyInternalKey != "" {
					st = state.NewDocWithInternalKey(ot.Id(), nil, uniqueKeyInternalKey).(*state.State)
				} else {
					st = state.NewDoc(ot.Id(), nil).(*state.State)
				}
				st.SetChangeId(change.Id)
				return true
			}

			model := change.Model.(*pb.Change)

			if model.ChangeType == domain.ChangeTypeUserChange.Raw() {
				sbHandler.CollectLastModifiedInfo(change)
			}

			if model.Version > lastMigrationVersion {
				lastMigrationVersion = model.Version
			}
			if startId == change.Id {
				if st == nil {
					changesAppliedSinceSnapshot = 0
					st, iterErr = state.NewDocFromSnapshot(ot.Id(), model.Snapshot, state.WithChangeId(startId), state.WithInternalKey(uniqueKeyInternalKey))
					if iterErr != nil {
						return false
					}
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

			cleanUpChange(ot.Id(), change, model)
			appliedContent = append(appliedContent, model.Content...)
			st.SetChangeId(change.Id)
			st.ApplyChangeIgnoreErr(model.Content...)
			st.AddFileKeys(model.FileKeys...)

			return true
		})

	if err != nil {
		return
	}
	if iterErr != nil {
		err = fmt.Errorf("iter: %w", iterErr)
		return
	}
	if applyState {
		_, _, err = state.ApplyStateFastOne(spaceId, st)
		if err != nil {
			return
		}
	}

	if !st.IsTheHeaderChange() {
		ts, accountId := sbHandler.GetLastModifiedInfo()
		st.SetLastModified(ts, domain.NewParticipantId(spaceId, accountId))
	}
	st.SetMigrationVersion(lastMigrationVersion)
	return
}
