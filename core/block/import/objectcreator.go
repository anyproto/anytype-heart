package importer

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sync"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/syncer"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
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
	objectCache    objectcache.Cache
	core           core.Service
	objectStore    objectstore.ObjectStore
	relationSyncer syncer.RelationSyncer
	syncFactory    *syncer.Factory
	fileStore      filestore.FileStore
	mu             sync.Mutex
}

func NewCreator(service *block.Service,
	cache objectcache.Cache,
	core core.Service,
	syncFactory *syncer.Factory,
	objectStore objectstore.ObjectStore,
	relationSyncer syncer.RelationSyncer,
	fileStore filestore.FileStore,
) Creator {
	return &ObjectCreator{
		service:        service,
		core:           core,
		syncFactory:    syncFactory,
		objectStore:    objectStore,
		relationSyncer: relationSyncer,
		fileStore:      fileStore,
		objectCache:    cache,
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

	st := state.NewDocFromSnapshot(newID, sn.Snapshot, state.WithUniqueKeyMigration(sn.SbType)).(*state.State)
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

	st.SetLastModified(lastModifiedDate, addr.IdentityPrefix+oc.core.AccountId())
	var filesToDelete []string
	defer func() {
		// delete file in ipfs if there is error after creation
		oc.onFinish(err, st, filesToDelete)
	}()

	converter.UpdateObjectIDsInRelations(st, oldIDtoNew, fileIDs)

	if err = converter.UpdateLinksToObjects(st, oldIDtoNew, fileIDs); err != nil {
		log.With("objectID", newID).Errorf("failed to update objects ids: %s", err.Error())
	}

	if sn.SbType == coresb.SmartBlockTypeWorkspace {
		oc.setSpaceDashboardID(spaceID, st)
		return nil, newID, nil
	}

	// TODO Fix this
	// converter.UpdateObjectType(oldIDtoNew, st)
	for _, link := range st.GetRelationLinks() {
		if link.Format == model.RelationFormat_file {
			filesToDelete = oc.relationSyncer.Sync(spaceID, st, link.Key)
		}
	}
	filesToDelete = append(filesToDelete, oc.handleCoverRelation(spaceID, st)...)
	oc.setFileAsImported(st)
	var respDetails *types.Struct
	err = oc.installBundledRelationsAndTypes(ctx, spaceID, st.GetRelationLinks(), st.ObjectTypeKeys())
	if err != nil {
		log.With("objectID", newID).Errorf("failed to install bundled relations and types: %s", err.Error())
	}
	if payload := createPayloads[newID]; payload.RootRawChange != nil {
		respDetails, err = oc.createNewObject(ctx, spaceID, payload, st, newID, oldIDtoNew)
		if err != nil {
			log.With("objectID", newID).Errorf("failed to create %s: %s", newID, err.Error())
			return nil, "", err
		}
	} else {
		if canUpdateObject(sn.SbType) {
			respDetails = oc.updateExistingObject(st, oldIDtoNew, newID)
		}
	}
	oc.setFavorite(snapshot, newID)

	oc.setArchived(snapshot, newID)

	syncErr := oc.syncFilesAndLinks(newID)
	if syncErr != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to sync %s: %s", newID, syncErr)
	}

	return respDetails, newID, nil
}

func canUpdateObject(sbType coresb.SmartBlockType) bool {
	return sbType != coresb.SmartBlockTypeRelation && sbType != coresb.SmartBlockTypeObjectType
}

func (oc *ObjectCreator) updateExistingObject(st *state.State, oldIDtoNew map[string]string, newID string) *types.Struct {
	if st.Store() != nil {
		oc.updateLinksInCollections(st, oldIDtoNew, false)
	}
	return oc.resetState(newID, st)
}

func (oc *ObjectCreator) installBundledRelationsAndTypes(
	ctx context.Context,
	spaceID string,
	links pbtypes.RelationLinks,
	objectTypeKeys []domain.TypeKey,
) error {

	idsToCheck := make([]string, 0, len(links)+len(objectTypeKeys))
	for _, link := range links {
		// TODO: check if we have them in oldIDtoNew
		if !bundle.HasRelation(link.Key) {
			continue
		}

		idsToCheck = append(idsToCheck, addr.BundledRelationURLPrefix+link.Key)
	}

	for _, typeKey := range objectTypeKeys {
		if !bundle.HasObjectTypeByKey(typeKey) {
			continue
		}
		// TODO: check if we have them in oldIDtoNew
		idsToCheck = append(idsToCheck, addr.BundledObjectTypeURLPrefix+string(typeKey))
	}

	_, _, err := oc.service.InstallBundledObjects(ctx, spaceID, idsToCheck)
	return err
}

func (oc *ObjectCreator) createNewObject(
	ctx context.Context,
	spaceID string,
	payload treestorage.TreeStorageCreatePayload,
	st *state.State,
	newID string,
	oldIDtoNew map[string]string) (*types.Struct, error) {
	var respDetails *types.Struct
	sb, err := oc.objectCache.CreateTreeObjectWithPayload(ctx, spaceID, payload, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{
			Ctx:         ctx,
			IsNewObject: true,
			State:       st,
			SpaceID:     spaceID,
		}
	})
	if err == nil {
		respDetails = sb.Details()
	} else if errors.Is(err, treestorage.ErrTreeExists) {
		err = getblock.Do(oc.service, newID, func(sb smartblock.SmartBlock) error {
			respDetails = sb.Details()
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("get existing object %s: %w", newID, err)
		}
	} else {
		log.With("objectID", newID).Errorf("failed to create %s: %s", newID, err.Error())
		return nil, err
	}
	log.With("objectID", newID).Infof("import object created %s", pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeyName.String()))

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
		err := block.Do(oc.service, oc.core.PredefinedObjects(spaceID).Workspace, func(ws basic.CommonOperations) error {
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

func (oc *ObjectCreator) handleCoverRelation(spaceID string, st *state.State) []string {
	if pbtypes.GetInt64(st.Details(), bundle.RelationKeyCoverType.String()) != 1 {
		return nil
	}
	filesToDelete := oc.relationSyncer.Sync(spaceID, st, bundle.RelationKeyCoverId.String())
	return filesToDelete
}

func (oc *ObjectCreator) resetState(newID string, st *state.State) *types.Struct {
	var respDetails *types.Struct
	err := block.Do(oc.service, newID, func(b smartblock.SmartBlock) error {
		err := history.ResetToVersion(b, st)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set state %s: %s", newID, err.Error())
		}
		commonOperations, ok := b.(basic.CommonOperations)
		if !ok {
			return nil
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
	// todo: rewrite it in order not to create state with URLs inside links
	err := block.Do(oc.service, newID, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		return st.Iterate(func(bl simple.Block) (isContinue bool) {
			s := oc.syncFactory.GetSyncer(bl)
			if s != nil {
				// We can't run syncer here because it will cause a deadlock, so we defer this operation
				tasks = append(tasks, func() error {
					err := s.Sync(newID, bl)
					if err != nil {
						return err
					}
					return nil
				})
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
	return nil
}

func (oc *ObjectCreator) updateLinksInCollections(st *state.State, oldIDtoNew map[string]string, isNewCollection bool) {
	err := block.Do(oc.service, st.RootId(), func(b smartblock.SmartBlock) error {
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

func (oc *ObjectCreator) setFileAsImported(st *state.State) {
	var fileHashes []string
	err := st.Iterate(func(bl simple.Block) (isContinue bool) {
		if fh, ok := bl.(simple.FileHashes); ok {
			fileHashes = fh.FillFileHashes(fileHashes)
		}
		return true
	})
	if err != nil {
		log.Errorf("failed to collect file hashes in state, %s", err)
	}

	for _, hash := range fileHashes {
		err = oc.fileStore.SetIsFileImported(hash, true)
		if err != nil {
			log.Errorf("failed to set isFileImported for file %s: %s", hash, err)
		}
	}
}
