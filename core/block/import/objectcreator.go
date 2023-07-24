package importer

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/syncer"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const relationsLimit = 10

type ObjectCreator struct {
	service        *block.Service
	picker         getblock.Picker
	core           core.Service
	objectStore    objectstore.ObjectStore
	relationSyncer syncer.RelationSyncer
	syncFactory    *syncer.Factory
	fileStore      filestore.FileStore
	mu             sync.Mutex
}

func NewCreator(service *block.Service,
	core core.Service,
	syncFactory *syncer.Factory,
	objectStore objectstore.ObjectStore,
	relationSyncer syncer.RelationSyncer,
	fileStore filestore.FileStore,
	picker getblock.Picker,
) Creator {
	return &ObjectCreator{
		service:        service,
		core:           core,
		syncFactory:    syncFactory,
		objectStore:    objectStore,
		relationSyncer: relationSyncer,
		fileStore:      fileStore,
		picker:         picker,
	}
}

// Create creates smart blocks from given snapshots
func (oc *ObjectCreator) Create(
	ctx context.Context,
	spaceID string,
	sn *converter.Snapshot,
	oldIDtoNew map[string]string,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	fileIDs []string,
) (*types.Struct, string, error) {
	snapshot := sn.Snapshot.Data

	var err error
	newID := oldIDtoNew[sn.Id]
	oc.setRootBlock(snapshot, newID)

	oc.setWorkspaceID(spaceID, newID, snapshot)

	st := state.NewDocFromSnapshot(newID, sn.Snapshot).(*state.State)
	st.SetRootId(newID)
	// explicitly set last modified date, because all local details removed in NewDocFromSnapshot; createdDate covered in the object header
	lastModifiedDate := pbtypes.GetInt64(sn.Snapshot.Data.Details, bundle.RelationKeyLastModifiedDate.String())
	createdDate := pbtypes.GetInt64(sn.Snapshot.Data.Details, bundle.RelationKeyCreatedDate.String())
	if lastModifiedDate == 0 {
		if createdDate != 0 {
			lastModifiedDate = createdDate
		} else {
			// we can't fallback to time.Now() because it will be inconsistent with the time used in object tree header.
			// So instead we should EXPLICITLY set creation date to the snapshot in all importers
			log.With("objectID", sn.Id).With("ext", path.Ext(sn.FileName)).Warnf("both lastModifiedDate and createdDate are not set in the imported snapshot")
		}
	}
	st.SetLastModified(lastModifiedDate, oc.core.ProfileID(spaceID))
	var filesToDelete []string
	defer func() {
		// delete file in ipfs if there is error after creation
		oc.onFinish(err, st, filesToDelete)
	}()

	converter.UpdateObjectIDsInRelations(st, oldIDtoNew, fileIDs)
	if sn.SbType == coresb.SmartBlockTypeSubObject {
		oc.handleSubObject(spaceID, st, newID)
		return nil, newID, nil
	}

	if err = converter.UpdateLinksToObjects(st, oldIDtoNew, fileIDs); err != nil {
		log.With("objectID", newID).Errorf("failed to update objects ids: %s", err.Error())
	}

	if sn.SbType == coresb.SmartBlockTypeWorkspace {
		oc.setSpaceDashboardID(spaceID, st)
		return nil, newID, nil
	}

	converter.UpdateObjectType(oldIDtoNew, st)
	for _, link := range st.GetRelationLinks() {
		if link.Format == model.RelationFormat_file {
			filesToDelete = oc.relationSyncer.Sync(st, link.Key)
		}
	}
	oc.updateDetailsKey(st, oldIDtoNew)
	filesToDelete = append(filesToDelete, oc.handleCoverRelation(st)...)
	var respDetails *types.Struct
	if payload := createPayloads[newID]; payload.RootRawChange != nil {
		respDetails, err = oc.createNewObject(ctx, spaceID, payload, st, newID, oldIDtoNew)
		if err != nil {
			log.With("objectID", newID).Errorf("failed to create %s: %s", newID, err.Error())
			return nil, "", err
		}
	} else {
		respDetails = oc.updateExistingObject(st, oldIDtoNew, newID)
	}
	oc.setFavorite(snapshot, newID)

	oc.setArchived(snapshot, newID)

	syncErr := oc.syncFilesAndLinks(newID)
	if syncErr != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to sync %s: %s", newID, syncErr)
	}

	return respDetails, newID, nil
}

func (oc *ObjectCreator) updateExistingObject(st *state.State, oldIDtoNew map[string]string, newID string) *types.Struct {
	if st.Store() != nil {
		oc.updateLinksInCollections(st, oldIDtoNew, false)
	}
	return oc.resetState(newID, st)
}

func (oc *ObjectCreator) createNewObject(
	ctx context.Context,
	spaceID string,
	payload treestorage.TreeStorageCreatePayload,
	st *state.State,
	newID string,
	oldIDtoNew map[string]string) (*types.Struct, error) {
	sb, err := oc.service.CreateTreeObjectWithPayload(ctx, spaceID, payload, func(id string) *sb.InitContext {
		return &sb.InitContext{
			Ctx:         ctx,
			IsNewObject: true,
			State:       st,
		}
	})
	if err != nil {
		log.With("objectID", newID).Errorf("failed to create %s: %s", newID, err.Error())
		return nil, err
	}
	respDetails := sb.Details()
	// update collection after we create it
	if st.Store() != nil {
		oc.updateLinksInCollections(st, oldIDtoNew, true)
		oc.resetState(newID, st)
	}
	return respDetails, nil
}

func (oc *ObjectCreator) setRootBlock(snapshot *model.SmartBlockSnapshotBase, newID string) {
	var found bool
	for _, b := range snapshot.Blocks {
		if b.Id == newID {
			found = true
			break
		}
	}
	if !found {
		oc.addRootBlock(snapshot, newID)
	}
}

func (oc *ObjectCreator) addRootBlock(snapshot *model.SmartBlockSnapshotBase, pageID string) {
	for i, b := range snapshot.Blocks {
		if _, ok := b.Content.(*model.BlockContentOfSmartblock); ok {
			snapshot.Blocks[i].Id = pageID
			return
		}
	}
	notRootBlockChild := make(map[string]bool, 0)
	for _, b := range snapshot.Blocks {
		for _, id := range b.ChildrenIds {
			notRootBlockChild[id] = true
		}
	}
	childrenIds := make([]string, 0)
	for _, b := range snapshot.Blocks {
		if _, ok := notRootBlockChild[b.Id]; !ok {
			childrenIds = append(childrenIds, b.Id)
		}
	}
	snapshot.Blocks = append(snapshot.Blocks, &model.Block{
		Id: pageID,
		Content: &model.BlockContentOfSmartblock{
			Smartblock: &model.BlockContentSmartblock{},
		},
		ChildrenIds: childrenIds,
	})
}

func (oc *ObjectCreator) setWorkspaceID(spaceID string, newID string, snapshot *model.SmartBlockSnapshotBase) {
	if oc.core.PredefinedObjects(spaceID).Account == newID {
		return
	}
	workspaceID, err := oc.core.GetWorkspaceIdForObject(spaceID, newID)
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to get workspace id %s: %s", newID, err.Error())
	}
	if workspaceID == "" {
		return
	}

	if snapshot.Details != nil && snapshot.Details.Fields != nil {
		snapshot.Details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceID)
	}
}

func (oc *ObjectCreator) onFinish(err error, st *state.State, filesToDelete []string) {
	if err != nil {
		for _, bl := range st.Blocks() {
			if f := bl.GetFile(); f != nil {
				oc.deleteFile(f.Hash)
			}
			for _, hash := range filesToDelete {
				oc.deleteFile(hash)
			}
		}
	}
}

func (oc *ObjectCreator) deleteFile(hash string) {
	inboundLinks, err := oc.objectStore.GetOutboundLinksByID(hash)
	if err != nil {
		log.With("file", hash).Errorf("failed to get inbound links for file: %s", err)
	}
	if len(inboundLinks) == 0 {
		err = oc.service.DeleteObject(hash)
		if err != nil {
			log.With("file", hash).Errorf("failed to delete file: %s", err)
		}
	}
}

func (oc *ObjectCreator) handleSubObject(spaceID string, st *state.State, newID string) {
	oc.mu.Lock()
	defer oc.mu.Unlock()
	if deleted := pbtypes.GetBool(st.CombinedDetails(), bundle.RelationKeyIsDeleted.String()); deleted {
		err := oc.service.RemoveSubObjectsInWorkspace(spaceID, []string{newID}, true)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to remove from collections %s: %s", newID, err.Error())
		}
		return
	}

	// RQ: the rest handling were removed
}

func (oc *ObjectCreator) setSpaceDashboardID(spaceID string, st *state.State) {
	// hand-pick relation because space is a special case
	var details []*pb.RpcObjectSetDetailsDetail
	spaceDashBoardID := pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeySpaceDashboardId.String())
	if spaceDashBoardID != "" {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeySpaceDashboardId.String(),
			Value: pbtypes.String(spaceDashBoardID),
		})
	}

	spaceName := pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeyName.String())
	if spaceName != "" {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(spaceName),
		})
	}

	iconOption := pbtypes.GetInt64(st.CombinedDetails(), bundle.RelationKeyIconOption.String())
	if iconOption != 0 {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyIconOption.String(),
			Value: pbtypes.Int64(iconOption),
		})
	}
	if len(details) > 0 {
		err := block.Do(oc.service, oc.core.PredefinedObjects(spaceID).Account, func(ws basic.CommonOperations) error {
			if err := ws.SetDetails(nil, details, false); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			log.Errorf("failed to set spaceDashBoardID, %s", err.Error())
		}
	}
}

func (oc *ObjectCreator) updateDetailsKey(st *state.State, oldIDtoNew map[string]string) {
	details := st.Details()
	keyToUpdate := make([]string, 0)
	for k, v := range details.GetFields() {
		if newKey, ok := oldIDtoNew[addr.RelationKeyToIdPrefix+k]; ok && newKey != addr.RelationKeyToIdPrefix+k {
			relKey := strings.TrimPrefix(newKey, addr.RelationKeyToIdPrefix)
			st.SetDetail(relKey, v)
			keyToUpdate = append(keyToUpdate, k)
		}
	}
	oc.updateRelationLinks(st, keyToUpdate, oldIDtoNew)
	st.RemoveRelation(keyToUpdate...)

}

func (oc *ObjectCreator) updateRelationLinks(st *state.State, keyToUpdate []string, oldToNewIDs map[string]string) {
	relLinksToUpdate := make([]*model.RelationLink, 0)
	for _, key := range keyToUpdate {
		if relLink := st.GetRelationLinks().Get(key); relLink != nil {
			newKey := oldToNewIDs[addr.RelationKeyToIdPrefix+key]
			relLinksToUpdate = append(relLinksToUpdate, &model.RelationLink{
				Key:    strings.TrimPrefix(newKey, addr.RelationKeyToIdPrefix),
				Format: relLink.Format,
			})
		}
	}
	st.AddRelationLinks(relLinksToUpdate...)
}

func (oc *ObjectCreator) handleCoverRelation(st *state.State) []string {
	if pbtypes.GetInt64(st.Details(), bundle.RelationKeyCoverType.String()) != 1 {
		return nil
	}
	filesToDelete := oc.relationSyncer.Sync(st, bundle.RelationKeyCoverId.String())
	return filesToDelete
}

func (oc *ObjectCreator) resetState(newID string, st *state.State) *types.Struct {
	var respDetails *types.Struct
	err := getblock.Do(oc.picker, newID, func(b sb.SmartBlock) error {
		err := history.ResetToVersion(b, st)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set state %s: %s", newID, err.Error())
		}
		commonOperations, ok := b.(basic.CommonOperations)
		if !ok {
			return fmt.Errorf("common operations is not allowed for this object")
		}
		err = commonOperations.FeaturedRelationAdd(nil, bundle.RelationKeyType.String())
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set featuredRelations %s: %s", newID, err.Error())
		}
		respDetails = b.CombinedDetails()
		return nil
	})
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to reset state %s: %s", newID, err.Error())
	}
	return respDetails
}

func (oc *ObjectCreator) setFavorite(snapshot *model.SmartBlockSnapshotBase, newID string) {
	isFavorite := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsFavorite.String())
	if isFavorite {
		err := oc.service.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{ContextId: newID, IsFavorite: true})
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set isFavorite when importing object: %s", err.Error())
		}
	}
}

func (oc *ObjectCreator) setArchived(snapshot *model.SmartBlockSnapshotBase, newID string) {
	isArchive := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsArchived.String())
	if isArchive {
		err := oc.service.SetPageIsArchived(pb.RpcObjectSetIsArchivedRequest{ContextId: newID, IsArchived: true})
		if err != nil {
			log.With(zap.String("object id", newID)).
				Errorf("failed to set isFavorite when importing object %s: %s", newID, err.Error())
		}
	}
}

func (oc *ObjectCreator) syncFilesAndLinks(newID string) error {
	tasks := make([]func() error, 0)
	var fileHashes []string

	err := getblock.Do(oc.picker, newID, func(b sb.SmartBlock) error {
		st := b.NewState()
		fileHashes = st.GetAllFileHashes(st.FileRelationKeys())
		return st.Iterate(func(bl simple.Block) (isContinue bool) {
			s := oc.syncFactory.GetSyncer(bl)
			if s != nil {
				// We can't run syncer here because it will cause a deadlock, so we defer this operation
				tasks = append(tasks, func() error {
					return s.Sync(newID, bl)
				})
			}
			if bl != nil {
				if file := bl.Model().GetFile(); file != nil {
					fileHashes = append(fileHashes, file.Hash)
				}
			}
			return true
		})
	})
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err := task(); err != nil {
			log.With(zap.String("objectID", newID)).Errorf("syncer: %s", err)
		}
	}

	for _, hash := range fileHashes {
		err = oc.fileStore.SetIsFileImported(hash, true)
		if err != nil {
			return fmt.Errorf("failed to set isFileImported for file %s: %s", hash, err)
		}
	}
	return nil
}

func (oc *ObjectCreator) updateLinksInCollections(st *state.State, oldIDtoNew map[string]string, isNewCollection bool) {
	err := block.Do(oc.service, st.RootId(), func(b sb.SmartBlock) error {
		originalState := b.NewState()
		var existedObjects []string
		if !isNewCollection {
			existedObjects = originalState.GetStoreSlice(template.CollectionStoreKey)
		}
		oc.mergeCollections(existedObjects, st, oldIDtoNew)
		return nil
	})
	if err != nil {
		log.Errorf("failed to get existed objects in collection, %s", err)
	}
}

func (oc *ObjectCreator) mergeCollections(existedObjects []string, st *state.State, oldIDtoNew map[string]string) {
	objectsInCollections := st.GetStoreSlice(template.CollectionStoreKey)
	for i, id := range objectsInCollections {
		if newID, ok := oldIDtoNew[id]; ok {
			objectsInCollections[i] = newID
		}
	}
	result := lo.Union(existedObjects, objectsInCollections)
	st.UpdateStoreSlice(template.CollectionStoreKey, result)
}
