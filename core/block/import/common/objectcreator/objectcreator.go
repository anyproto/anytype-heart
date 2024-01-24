package objectcreator

import (
	"context"
	"errors"
	"fmt"
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
	widgetObject "github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/syncer"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("import")

// Service incapsulate logic with creation of given smartblocks
type Service interface {
	//nolint:lll
	Create(dataObject *DataObject, sn *common.Snapshot) (*types.Struct, string, error)
}

type ObjectCreator struct {
	service        *block.Service
	spaceService   space.Service
	objectStore    objectstore.ObjectStore
	relationSyncer syncer.RelationSyncer
	syncFactory    *syncer.Factory
	fileStore      filestore.FileStore
	objectCreator  objectcreator.Service
	mu             sync.Mutex
}

func New(service *block.Service,
	syncFactory *syncer.Factory,
	objectStore objectstore.ObjectStore,
	relationSyncer syncer.RelationSyncer,
	fileStore filestore.FileStore,
	spaceService space.Service,
	objectCreator objectcreator.Service,
) Service {
	return &ObjectCreator{
		service:        service,
		syncFactory:    syncFactory,
		objectStore:    objectStore,
		relationSyncer: relationSyncer,
		fileStore:      fileStore,
		spaceService:   spaceService,
		objectCreator:  objectCreator,
	}
}

// Create creates smart blocks from given snapshots
func (oc *ObjectCreator) Create(dataObject *DataObject, sn *common.Snapshot) (*types.Struct, string, error) {
	snapshot := sn.Snapshot.Data
	oldIDtoNew := dataObject.oldIDtoNew
	fileIDs := dataObject.fileIDs
	ctx := dataObject.ctx
	origin := dataObject.origin
	spaceID := dataObject.spaceID

	var err error
	newID := oldIDtoNew[sn.Id]

	if sn.SbType == coresb.SmartBlockTypeFile {
		return nil, newID, nil
	}
	oc.setRootBlock(snapshot, newID)

	oc.injectImportDetails(sn, origin, spaceID)
	st := state.NewDocFromSnapshot(newID, sn.Snapshot, state.WithUniqueKeyMigration(sn.SbType)).(*state.State)
	st.SetLocalDetail(bundle.RelationKeyLastModifiedDate.String(), pbtypes.Int64(pbtypes.GetInt64(snapshot.Details, bundle.RelationKeyLastModifiedDate.String())))

	var filesToDelete []string
	defer func() {
		// delete file in ipfs if there is error after creation
		oc.onFinish(err, st, filesToDelete)
	}()

	common.UpdateObjectIDsInRelations(st, oldIDtoNew, fileIDs)

	// this function is needed to support old use cases
	oc.setFileImportedFlagAndOrigin(st, origin, oldIDtoNew)

	if err = common.UpdateLinksToObjects(st, oldIDtoNew, fileIDs); err != nil {
		log.With("objectID", newID).Errorf("failed to update objects ids: %s", err)
	}

	if sn.SbType == coresb.SmartBlockTypeWorkspace {
		oc.setSpaceDashboardID(spaceID, st)
		return nil, newID, nil
	}

	if sn.SbType == coresb.SmartBlockTypeWidget {
		return oc.updateWidgetObject(st)
	}

	// TODO Fix this
	// converter.UpdateObjectType(oldIDtoNew, st)
	for _, link := range st.GetRelationLinks() {
		if link.Format == model.RelationFormat_file {
			filesToDelete = oc.relationSyncer.Sync(spaceID, st, link.Key, origin)
		}
	}
	filesToDelete = append(filesToDelete, oc.handleCoverRelation(spaceID, st)...)
	typeKeys := st.ObjectTypeKeys()
	if sn.SbType == coresb.SmartBlockTypeObjectType {
		// we widen typeKeys here to install bundled templates for imported object type
		typeKeys = append(typeKeys, domain.TypeKey(st.UniqueKeyInternal()))
	}
	err = oc.installBundledRelationsAndTypes(ctx, spaceID, st.GetRelationLinks(), typeKeys, origin)
	if err != nil {
		log.With("objectID", newID).Errorf("failed to install bundled relations and types: %s", err)
	}
	var respDetails *types.Struct
	if payload := dataObject.createPayloads[newID]; payload.RootRawChange != nil {
		respDetails, err = oc.createNewObject(ctx, spaceID, payload, st, newID, oldIDtoNew)
		if err != nil {
			log.With("objectID", newID).Errorf("failed to create %s: %s", newID, err)
			return nil, "", err
		}
	} else {
		if canUpdateObject(sn.SbType) {
			respDetails = oc.updateExistingObject(st, oldIDtoNew, newID)
		}
	}
	oc.setFavorite(snapshot, newID)

	oc.setArchived(snapshot, newID)

	syncErr := oc.syncFilesAndLinks(newID, origin)
	if syncErr != nil {
		if errors.Is(syncErr, common.ErrFileLoad) {
			return respDetails, newID, syncErr
		}
	}

	return respDetails, newID, nil
}

func canUpdateObject(sbType coresb.SmartBlockType) bool {
	return sbType != coresb.SmartBlockTypeRelation && sbType != coresb.SmartBlockTypeObjectType && sbType != coresb.SmartBlockTypeRelationOption
}

func (oc *ObjectCreator) injectImportDetails(sn *common.Snapshot, origin model.ObjectOrigin, spaceID string) {
	lastModifiedDate := pbtypes.GetInt64(sn.Snapshot.Data.Details, bundle.RelationKeyLastModifiedDate.String())
	createdDate := pbtypes.GetInt64(sn.Snapshot.Data.Details, bundle.RelationKeyCreatedDate.String())
	if lastModifiedDate == 0 {
		if createdDate != 0 {
			sn.Snapshot.Data.Details.Fields[bundle.RelationKeyLastModifiedDate.String()] = pbtypes.Int64(int64(createdDate))
		} else {
			// we can't fallback to time.Now() because it will be inconsistent with the time used in object tree header.
			// So instead we should EXPLICITLY set creation date to the snapshot in all importers
			log.With("objectID", sn.Id).Warnf("both lastModifiedDate and createdDate are not set in the imported snapshot")
		}
	}

	if createdDate > 0 {
		// pass it explicitly to the snapshot
		sn.Snapshot.Data.OriginalCreatedTimestamp = createdDate
	}

	sn.Snapshot.Data.Details.Fields[bundle.RelationKeyOrigin.String()] = pbtypes.Int64(int64(origin))
	// we don't need to inject relatonLinks, they will be automatically injected for bundled relations

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
	origin model.ObjectOrigin,
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

	spc, err := oc.spaceService.Get(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("get space %s: %w", spaceID, err)
	}
	_, _, err = oc.objectCreator.InstallBundledObjects(ctx, spc, idsToCheck, origin == model.ObjectOrigin_usecase)
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
	spc, err := oc.spaceService.Get(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("get space %s: %w", spaceID, err)
	}
	sb, err := spc.CreateTreeObjectWithPayload(ctx, payload, func(id string) *smartblock.InitContext {
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
		err = spc.Do(newID, func(sb smartblock.SmartBlock) error {
			respDetails = sb.Details()
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("get existing object %s: %w", newID, err)
		}
	} else {
		log.With("objectID", newID).Errorf("failed to create %s: %s", newID, err)
		return nil, err
	}

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
		spc, err := oc.spaceService.Get(context.Background(), spaceID)
		if err != nil {
			log.Errorf("failed to get space: %v", err)
			return
		}
		err = block.Do(oc.service, spc.DerivedIDs().Workspace, func(ws basic.CommonOperations) error {
			if err := ws.SetDetails(nil, details, false); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			log.Errorf("failed to set spaceDashBoardID, %s", err)
		}
	}
}

func (oc *ObjectCreator) handleCoverRelation(spaceID string, st *state.State) []string {
	if pbtypes.GetInt64(st.Details(), bundle.RelationKeyCoverType.String()) != 1 {
		return nil
	}
	filesToDelete := oc.relationSyncer.Sync(spaceID, st, bundle.RelationKeyCoverId.String(), 0)
	return filesToDelete
}

func (oc *ObjectCreator) resetState(newID string, st *state.State) *types.Struct {
	var respDetails *types.Struct
	err := block.Do(oc.service, newID, func(b smartblock.SmartBlock) error {
		err := history.ResetToVersion(b, st)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set state %s: %s", newID, err)
		}
		commonOperations, ok := b.(basic.CommonOperations)
		if !ok {
			return nil
		}
		err = commonOperations.FeaturedRelationAdd(nil, bundle.RelationKeyType.String())
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set featuredRelations %s: %s", newID, err)
		}
		respDetails = b.CombinedDetails()
		return nil
	})
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to reset state %s: %s", newID, err)
	}
	return respDetails
}

func (oc *ObjectCreator) setFavorite(snapshot *model.SmartBlockSnapshotBase, newID string) {
	isFavorite := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsFavorite.String())
	if isFavorite {
		err := oc.service.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{ContextId: newID, IsFavorite: true})
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set isFavorite when importing object: %s", err)
		}
	}
}

func (oc *ObjectCreator) setArchived(snapshot *model.SmartBlockSnapshotBase, newID string) {
	isArchive := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsArchived.String())
	if isArchive {
		err := oc.service.SetPageIsArchived(pb.RpcObjectSetIsArchivedRequest{ContextId: newID, IsArchived: true})
		if err != nil {
			log.With(zap.String("object id", newID)).
				Errorf("failed to set isFavorite when importing object %s: %s", newID, err)
		}
	}
}

func (oc *ObjectCreator) syncFilesAndLinks(newID string, origin model.ObjectOrigin) error {
	tasks := make([]func() error, 0)
	// todo: rewrite it in order not to create state with URLs inside links
	err := block.Do(oc.service, newID, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		return st.Iterate(func(bl simple.Block) (isContinue bool) {
			s := oc.syncFactory.GetSyncer(bl)
			if s != nil {
				// We can't run syncer here because it will cause a deadlock, so we defer this operation
				tasks = append(tasks, func() error {
					err := s.Sync(newID, bl, origin)
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
	var fileLoadErr error
	for _, task := range tasks {
		err = task()
		if err != nil {
			log.With(zap.String("objectID", newID)).Errorf("failed to sync: %s", err)
			if errors.Is(err, common.ErrFileLoad) {
				fileLoadErr = err
			}
		}
	}
	return fileLoadErr
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

func (oc *ObjectCreator) setFileImportedFlagAndOrigin(st *state.State, origin model.ObjectOrigin, oldToNew map[string]string) {
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
		// we have file objects, so skip this logic
		if newID, ok := oldToNew[hash]; ok {
			if oc.saveFileKeys(st, newID) {
				return
			}
			continue
		}
		err = oc.fileStore.SetIsFileImported(hash, true)
		if err != nil {
			log.Errorf("failed to set isFileImported for file %s: %s", hash, err)
		}
		err = oc.fileStore.SetFileOrigin(hash, origin)
		if err != nil {
			log.Errorf("failed to set origin for file %s: %s", hash, err)
		}
	}
}

func (oc *ObjectCreator) saveFileKeys(st *state.State, newHash string) bool {
	fileKeys, err := oc.fileStore.GetFileKeys(newHash)
	if err != nil {
		return true
	}
	st.AddFileKeys(&pb.ChangeFileKeys{
		Hash: newHash,
		Keys: fileKeys,
	})
	return false
}

func (oc *ObjectCreator) updateWidgetObject(st *state.State) (*types.Struct, string, error) {
	err := block.DoState(oc.service, st.RootId(), func(oldState *state.State, sb smartblock.SmartBlock) error {
		blocks := st.Blocks()
		blocksMap := make(map[string]*model.Block, len(blocks))
		existingWidgetsTargetIDs, err := oc.getExistingWidgetsTargetIDs(oldState)
		if err != nil {
			return err
		}
		for _, block := range blocks {
			blocksMap[block.Id] = block
		}
		for _, block := range blocks {
			if widget := block.GetWidget(); widget != nil {
				if len(block.ChildrenIds) > 0 {
					err := oc.addWidgetBlock(oldState, block, blocksMap, existingWidgetsTargetIDs)
					if err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	return nil, "", err
}

func (oc *ObjectCreator) addWidgetBlock(oldState *state.State,
	block *model.Block,
	blocksMap map[string]*model.Block,
	existingWidgetsTargetIDs map[string]struct{},
) error {
	linkBlockID := block.ChildrenIds[0]
	if linkBlock, ok := blocksMap[linkBlockID]; ok {
		if oc.skipObject(linkBlock.GetLink().GetTargetBlockId(), existingWidgetsTargetIDs) {
			return nil
		}
		oldState.Add(simple.New(block))
		oldState.Add(simple.New(linkBlock))
		err := oldState.InsertTo("", model.Block_Inner, block.Id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (oc *ObjectCreator) skipObject(targetID string, existingWidgetsTargetIDs map[string]struct{}) bool {
	if widgetObject.IsPredefinedWidgetTargetId(targetID) {
		if _, ok := existingWidgetsTargetIDs[targetID]; ok {
			return true
		}
	}
	return false
}

func (oc *ObjectCreator) getExistingWidgetsTargetIDs(oldState *state.State) (map[string]struct{}, error) {
	existingWidgetsTargetIDs := make(map[string]struct{}, 0)
	err := oldState.Iterate(func(b simple.Block) (isContinue bool) {
		if b.Model().GetLink() != nil {
			existingWidgetsTargetIDs[b.Model().GetLink().GetTargetBlockId()] = struct{}{}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return existingWidgetsTargetIDs, nil
}
