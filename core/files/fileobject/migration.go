package fileobject

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/net/peer"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

// migrationItem is a queue item for file object migration. Should be fully serializable
type migrationItem struct {
	FileObjectId  string
	SpaceId       string
	CreateRequest filemodels.CreateRequest
	UniqueKeyRaw  string
}

func (it *migrationItem) Key() string {
	return it.SpaceId + "/" + it.CreateRequest.FileId.String()
}

func makeMigrationItem() *migrationItem {
	return &migrationItem{}
}

func (s *service) MigrateFileIdsInBlocks(st *state.State, spc source.Space) {
	if !spc.IsPersonal() {
		return
	}
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if migrator, ok := b.(simple.FileMigrator); ok {
			migrator.MigrateFile(func(oldId string) (newId string) {
				return s.migrateFileId(spc.(clientspace.Space), st.RootId(), oldId)
			})
		}
		return true
	})
}

func (s *service) MigrateFileIdsInDetails(st *state.State, spc source.Space) {
	if !spc.IsPersonal() {
		return
	}
	st.ModifyLinkedFilesInDetails(func(id string) string {
		return s.migrateFileId(spc.(clientspace.Space), st.RootId(), id)
	})
	st.ModifyLinkedObjectsInDetails(func(id string) string {
		return s.migrateFileId(spc.(clientspace.Space), st.RootId(), id)
	})
}

func (s *service) migrateFileId(space clientspace.Space, objectId string, fileId string) string {
	// Don't migrate empty or its own id
	if fileId == "" || objectId == fileId {
		return fileId
	}
	if !domain.IsFileId(fileId) {
		return fileId
	}

	// Add fileId as uniqueKey to avoid migration of the same file
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeFileObject, fileId)
	if err != nil {
		return fileId
	}
	fileObjectId, err := space.DeriveObjectIdWithAccountSignature(context.Background(), uniqueKey)
	if err != nil {
		log.Errorf("can't derive object id for fileId %s: %v", fileId, err)
		return fileId
	}
	return fileObjectId
}

// MigrateFiles creates file objects from old files data. We don't want to miss any file, so
// we can't count on migrating files using blocks or details, because eventually old file ids
// from blocks or details will be changed to file object ids. And in case of any migration error
// these files could be stuck un-migrated
func (s *service) MigrateFiles(st *state.State, spc source.Space, keysChanges []*pb.ChangeFileKeys) {
	if !spc.IsPersonal() {
		return
	}
	origin := objectorigin.FromDetails(st.Details())
	for _, keys := range keysChanges {
		err := s.migrateFile(spc.(clientspace.Space), origin, keys)
		if err != nil {
			log.Errorf("migrate file %s: %v", keys.Hash, err)
		}
	}
}

func (s *service) migrateFile(space clientspace.Space, origin objectorigin.ObjectOrigin, fileKeysChange *pb.ChangeFileKeys) error {
	fileId := domain.FileId(fileKeysChange.Hash)
	if !fileId.Valid() {
		return nil
	}

	// Add fileId as uniqueKey to avoid migration of the same file
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeFileObject, fileId.String())
	if err != nil {
	}
	fileObjectId, err := space.DeriveObjectIdWithAccountSignature(context.Background(), uniqueKey)
	if err != nil {
		return fmt.Errorf("derive object id for fileId %s: %w", fileId, err)
	}

	queueIt := &migrationItem{
		FileObjectId: fileObjectId,
		SpaceId:      space.Id(),
		CreateRequest: filemodels.CreateRequest{
			FileId:         fileId,
			EncryptionKeys: fileKeysChange.Keys,
			ObjectOrigin:   origin,
		},
		UniqueKeyRaw: uniqueKey.Marshal(),
	}
	err = s.migrationQueue.Add(queueIt)
	if err != nil {
		return fmt.Errorf("add to migration queue: %w", err)
	}
	return nil
}

func (s *service) migrationQueueHandler(ctx context.Context, it *migrationItem) (persistentqueue.Action, error) {
	space, err := s.spaceService.Get(ctx, it.SpaceId)
	if err != nil {
		return persistentqueue.ActionDone, fmt.Errorf("get space: %w", err)
	}
	if !space.IsPersonal() {
		return persistentqueue.ActionDone, nil
	}

	ctx = peer.CtxWithPeerId(ctx, peer.CtxResponsiblePeers)
	_, err = space.GetObject(ctx, it.FileObjectId)
	// Already migrated or deleted file object
	if err == nil || errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
		return persistentqueue.ActionDone, nil
	}

	uniqueKey, err := domain.UnmarshalUniqueKey(it.UniqueKeyRaw)
	if err != nil {
		return persistentqueue.ActionDone, fmt.Errorf("unmarshal unique key: %w", err)
	}
	err = s.migrateDeriveObject(context.Background(), space, it.CreateRequest, uniqueKey)
	if err != nil {
		log.Errorf("create file object for fileId %s: %v", it.CreateRequest.FileId, err)
	}
	return persistentqueue.ActionDone, nil
}

func (s *service) migrateDeriveObject(ctx context.Context, space clientspace.Space, req filemodels.CreateRequest, uniqueKey domain.UniqueKey) (err error) {
	if req.FileId == "" {
		return fmt.Errorf("file hash is empty")
	}
	details := s.makeInitialDetails(req.FileId, req.ObjectOrigin, req.ImageKind)

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(details)
	createState.SetFileInfo(state.FileInfo{
		FileId:         req.FileId,
		EncryptionKeys: req.EncryptionKeys,
	})

	// Type will be changed after indexing, just use general type File for now
	id, _, err := s.objectCreator.CreateSmartBlockFromStateInSpaceWithOptions(ctx, space, []domain.TypeKey{bundle.TypeKeyFile}, createState)
	if errors.Is(err, treestorage.ErrTreeExists) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}
	fullFileId := domain.FullFileId{SpaceId: space.Id(), FileId: req.FileId}
	err = s.indexer.addToQueue(ctx, domain.FullID{SpaceID: space.Id(), ObjectID: id}, fullFileId)
	if err != nil {
		// Will be retried in background, so don't return error
		log.Errorf("add to index queue: %v", err)
		err = nil
	}

	syncReq := filesync.AddFileRequest{
		FileObjectId:   id,
		FileId:         fullFileId,
		UploadedByUser: false,
		Imported:       false,
	}
	err = s.addToSyncQueue(syncReq)
	if err != nil {
		return fmt.Errorf("add to sync queue: %w", err)
	}
	return nil
}
