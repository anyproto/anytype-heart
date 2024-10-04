package objectcreator

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	widgetObject "github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/history"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/syncer"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("import")

// Service encapsulate logic with creation of given smartblocks
type Service interface {
	//nolint:lll
	Create(dataObject *DataObject, sn *common.Snapshot) (*types.Struct, string, error)
}

type ObjectGetterDeleter interface {
	cache.ObjectGetterComponent
	DeleteObject(objectId string) (err error)
}

type ObjectCreator struct {
	detailsService      detailservice.Service
	spaceService        space.Service
	objectStore         objectstore.ObjectStore
	relationSyncer      *syncer.FileRelationSyncer
	syncFactory         *syncer.Factory
	objectCreator       objectcreator.Service
	objectGetterDeleter ObjectGetterDeleter
}

func New(detailsService detailservice.Service,
	syncFactory *syncer.Factory,
	objectStore objectstore.ObjectStore,
	relationSyncer *syncer.FileRelationSyncer,
	spaceService space.Service,
	objectCreator objectcreator.Service,
	objectGetterDeleter ObjectGetterDeleter,
) Service {
	return &ObjectCreator{
		detailsService:      detailsService,
		syncFactory:         syncFactory,
		objectStore:         objectStore,
		relationSyncer:      relationSyncer,
		spaceService:        spaceService,
		objectCreator:       objectCreator,
		objectGetterDeleter: objectGetterDeleter,
	}
}

// Create creates smart blocks from given snapshots
func (oc *ObjectCreator) Create(dataObject *DataObject, sn *common.Snapshot) (*types.Struct, string, error) {
	snapshot := sn.Snapshot.Data
	oldIDtoNew := dataObject.oldIDtoNew
	ctx := dataObject.ctx
	origin := dataObject.origin
	spaceID := dataObject.spaceID

	newID := oldIDtoNew[sn.Id]

	if sn.SbType == coresb.SmartBlockTypeFile {
		return nil, newID, nil
	}

	oc.setRootBlock(snapshot, newID)

	oc.injectImportDetails(sn, origin)
	st := state.NewDocFromSnapshot(newID, sn.Snapshot, state.WithUniqueKeyMigration(sn.SbType)).(*state.State)
	st.SetLocalDetail(bundle.RelationKeyLastModifiedDate.String(), pbtypes.Int64(pbtypes.GetInt64(snapshot.Details, bundle.RelationKeyLastModifiedDate.String())))

	var (
		filesToDelete []string
		err           error
	)
	defer func() {
		// delete file in ipfs if there is error after creation
		oc.onFinish(err, st, filesToDelete)
	}()

	common.UpdateObjectIDsInRelations(st, oldIDtoNew)

	if err = common.UpdateLinksToObjects(st, oldIDtoNew); err != nil {
		log.With("objectID", newID).Errorf("failed to update objects ids: %s", err)
	}

	oc.updateKeys(st, oldIDtoNew)
	if sn.SbType == coresb.SmartBlockTypeWorkspace {
		oc.setSpaceDashboardID(spaceID, st)
		return nil, newID, nil
	}

	if sn.SbType == coresb.SmartBlockTypeWidget {
		return oc.updateWidgetObject(st)
	}

	st.ModifyLinkedFilesInDetails(func(fileId string) string {
		newFileId := oc.relationSyncer.Sync(spaceID, fileId, dataObject.newIdsSet, origin)
		if newFileId != fileId {
			filesToDelete = append(filesToDelete, fileId)
		}
		return newFileId
	})

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

	syncErr := oc.syncFilesAndLinks(dataObject.newIdsSet, domain.FullID{SpaceID: spaceID, ObjectID: newID}, origin)
	if syncErr != nil {
		if errors.Is(syncErr, common.ErrFileLoad) {
			return respDetails, newID, syncErr
		}
	}
	return respDetails, newID, nil
}

func canUpdateObject(sbType coresb.SmartBlockType) bool {
	return sbType != coresb.SmartBlockTypeRelation &&
		sbType != coresb.SmartBlockTypeObjectType &&
		sbType != coresb.SmartBlockTypeRelationOption &&
		sbType != coresb.SmartBlockTypeFileObject &&
		sbType != coresb.SmartBlockTypeParticipant
}

func (oc *ObjectCreator) injectImportDetails(sn *common.Snapshot, origin objectorigin.ObjectOrigin) {
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

	sn.Snapshot.Data.Details.Fields[bundle.RelationKeyOrigin.String()] = pbtypes.Int64(int64(origin.Origin))
	sn.Snapshot.Data.Details.Fields[bundle.RelationKeyImportType.String()] = pbtypes.Int64(int64(origin.ImportType))
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
	origin objectorigin.ObjectOrigin,
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
	_, _, err = oc.objectCreator.InstallBundledObjects(ctx, spc, idsToCheck, origin.Origin == model.ObjectOrigin_usecase)
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
		sb.Lock()
		respDetails = sb.Details()
		sb.Unlock()
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
		snapshot.Blocks = anymark.AddRootBlock(snapshot.Blocks, newID)
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
		err = oc.objectGetterDeleter.DeleteObject(hash)
		if err != nil {
			log.With("file", hash).Errorf("failed to delete file: %s", anyerror.CleanupError(err))
		}
	}
}

func (oc *ObjectCreator) setSpaceDashboardID(spaceID string, st *state.State) {
	// hand-pick relation because space is a special case
	var details []*model.Detail
	spaceDashBoardID := pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeySpaceDashboardId.String())
	if spaceDashBoardID != "" {
		details = append(details, &model.Detail{
			Key:   bundle.RelationKeySpaceDashboardId.String(),
			Value: pbtypes.String(spaceDashBoardID),
		})
	}

	spaceName := pbtypes.GetString(st.CombinedDetails(), bundle.RelationKeyName.String())
	if spaceName != "" {
		details = append(details, &model.Detail{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(spaceName),
		})
	}

	iconOption := pbtypes.GetInt64(st.CombinedDetails(), bundle.RelationKeyIconOption.String())
	if iconOption != 0 {
		details = append(details, &model.Detail{
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
		err = cache.Do(oc.objectGetterDeleter, spc.DerivedIDs().Workspace, func(ws basic.CommonOperations) error {
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

func (oc *ObjectCreator) resetState(newID string, st *state.State) *types.Struct {
	var respDetails *types.Struct
	err := cache.Do(oc.objectGetterDeleter, newID, func(b smartblock.SmartBlock) error {
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
		err := oc.detailsService.SetIsFavorite(newID, true)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set isFavorite when importing object: %s", err)
		}
	}
}

func (oc *ObjectCreator) setArchived(snapshot *model.SmartBlockSnapshotBase, newID string) {
	isArchive := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsArchived.String())
	if isArchive {
		err := oc.detailsService.SetIsArchived(newID, true)
		if err != nil {
			log.With(zap.String("object id", newID)).
				Errorf("failed to set isFavorite when importing object %s: %s", newID, err)
		}
	}
}

func (oc *ObjectCreator) syncFilesAndLinks(newIdsSet map[string]struct{}, id domain.FullID, origin objectorigin.ObjectOrigin) error {
	tasks := make([]func() error, 0)
	// todo: rewrite it in order not to create state with URLs inside links
	err := cache.Do(oc.objectGetterDeleter, id.ObjectID, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		return st.Iterate(func(bl simple.Block) (isContinue bool) {
			s := oc.syncFactory.GetSyncer(bl)
			if s != nil {
				// We can't run syncer here because it will cause a deadlock, so we defer this operation
				tasks = append(tasks, func() error {
					err := s.Sync(id, newIdsSet, bl, origin)
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
			log.With(zap.String("objectId", id.ObjectID)).Errorf("failed to sync: %s", err)
			if errors.Is(err, common.ErrFileLoad) {
				fileLoadErr = err
			}
		}
	}
	return fileLoadErr
}

func (oc *ObjectCreator) updateLinksInCollections(st *state.State, oldIDtoNew map[string]string, isNewCollection bool) {
	err := cache.Do(oc.objectGetterDeleter, st.RootId(), func(b smartblock.SmartBlock) error {
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

func (oc *ObjectCreator) updateWidgetObject(st *state.State) (*types.Struct, string, error) {
	err := cache.DoState(oc.objectGetterDeleter, st.RootId(), func(oldState *state.State, sb smartblock.SmartBlock) error {
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

func (oc *ObjectCreator) updateKeys(st *state.State, oldIDtoNew map[string]string) {
	for key, value := range st.Details().GetFields() {
		if newKey, ok := oldIDtoNew[key]; ok && newKey != key {
			oc.updateDetails(st, newKey, value, key)
		}
	}

	if newKey, ok := oldIDtoNew[st.ObjectTypeKey().String()]; ok {
		st.SetObjectTypeKey(domain.TypeKey(newKey))
	}
}

func (oc *ObjectCreator) updateDetails(st *state.State, newKey string, value *types.Value, key string) {
	st.SetDetail(newKey, value)
	link := oc.findRelationLinkByKey(st, key)
	if link != nil {
		link.Key = newKey
		st.AddRelationLinks(link)
	}
	st.RemoveRelation(key)
}

func (oc *ObjectCreator) findRelationLinkByKey(st *state.State, key string) *model.RelationLink {
	relationLinks := st.GetRelationLinks()
	var link *model.RelationLink
	for _, link = range relationLinks {
		if link.Key == key {
			break
		}
	}
	return link
}
