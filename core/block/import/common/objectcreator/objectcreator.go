package objectcreator

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
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
)

var log = logging.Logger("import")

// Service encapsulate logic with creation of given smartblocks
type Service interface {
	//nolint:lll
	Create(dataObject *DataObject, sn *common.Snapshot) (*domain.Details, string, error)
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
func (oc *ObjectCreator) Create(dataObject *DataObject, sn *common.Snapshot) (*domain.Details, string, error) {
	snapshot := sn.Snapshot.Data
	oldIDtoNew := dataObject.oldIDtoNew
	ctx := dataObject.ctx
	origin := dataObject.origin
	spaceID := dataObject.spaceID

	newID := oldIDtoNew[sn.Id]

	if sn.Snapshot.SbType == coresb.SmartBlockTypeFile {
		return nil, newID, nil
	}

	oc.setRootBlock(snapshot, newID)

	oc.injectImportDetails(sn, origin)
	st := state.NewDocFromSnapshot(newID, sn.Snapshot.ToProto()).(*state.State)
	st.SetLocalDetail(bundle.RelationKeyLastModifiedDate, snapshot.Details.Get(bundle.RelationKeyLastModifiedDate))

	var (
		filesToDelete []string
		err           error
	)
	defer func() {
		// delete file in ipfs if there is error after creation
		oc.onFinish(err, spaceID, st, filesToDelete)
	}()

	common.UpdateObjectIDsInRelations(st, oldIDtoNew, dataObject.relationKeysToFormat)

	if err = common.UpdateLinksToObjects(st, oldIDtoNew); err != nil {
		log.With("objectID", newID).Errorf("failed to update objects ids: %s", err)
	}

	oc.updateKeys(st, oldIDtoNew)
	if sn.Snapshot.SbType == coresb.SmartBlockTypeWorkspace {
		oc.setSpaceDashboardID(spaceID, st)
		return nil, newID, nil
	}

	if sn.Snapshot.SbType == coresb.SmartBlockTypeWidget {
		return oc.updateWidgetObject(st)
	}

	st.ModifyLinkedFilesInDetails(oc.objectStore.SpaceIndex(spaceID), func(fileId string) string {
		newFileId := oc.relationSyncer.Sync(spaceID, fileId, dataObject.newIdsSet, origin)
		if newFileId != fileId {
			filesToDelete = append(filesToDelete, fileId)
		}
		return newFileId
	})

	typeKeys := st.ObjectTypeKeys()
	if sn.Snapshot.SbType == coresb.SmartBlockTypeObjectType {
		// we widen typeKeys here to install bundled templates for imported object type
		typeKeys = append(typeKeys, domain.TypeKey(st.UniqueKeyInternal()))
	}
	err = oc.installBundledRelationsAndTypes(ctx, spaceID, st.AllRelationKeys(), typeKeys, origin)
	if err != nil {
		log.With("objectID", newID).Errorf("failed to install bundled relations and types: %s", err)
	}
	var respDetails *domain.Details
	if payload := dataObject.createPayloads[newID]; payload.RootRawChange != nil {
		respDetails, err = oc.createNewObject(ctx, spaceID, payload, st, newID, oldIDtoNew)
		if err != nil {
			log.With("objectID", newID).Errorf("failed to create %s: %s", newID, err)
			return nil, "", err
		}
	} else {
		if canUpdateObject(sn.Snapshot.SbType) {
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
		sbType != coresb.SmartBlockTypeRelationOption &&
		sbType != coresb.SmartBlockTypeFileObject &&
		sbType != coresb.SmartBlockTypeParticipant
}

func (oc *ObjectCreator) injectImportDetails(sn *common.Snapshot, origin objectorigin.ObjectOrigin) {
	lastModifiedDate := sn.Snapshot.Data.Details.GetInt64(bundle.RelationKeyLastModifiedDate)
	createdDate := sn.Snapshot.Data.Details.GetInt64(bundle.RelationKeyCreatedDate)
	if lastModifiedDate == 0 {
		if createdDate != 0 {
			sn.Snapshot.Data.Details.SetInt64(bundle.RelationKeyLastModifiedDate, createdDate)
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

	sn.Snapshot.Data.Details.SetInt64(bundle.RelationKeyOrigin, int64(origin.Origin))
	sn.Snapshot.Data.Details.SetInt64(bundle.RelationKeyImportType, int64(origin.ImportType))
	// we don't need to inject relatonLinks, they will be automatically injected for bundled relations
}

func (oc *ObjectCreator) updateExistingObject(st *state.State, oldIDtoNew map[string]string, newID string) *domain.Details {
	if st.Store() != nil {
		oc.updateLinksInCollections(st, oldIDtoNew, false)
	}
	return oc.resetState(newID, st)
}

func (oc *ObjectCreator) installBundledRelationsAndTypes(
	ctx context.Context,
	spaceID string,
	relationKeys []domain.RelationKey,
	objectTypeKeys []domain.TypeKey,
	origin objectorigin.ObjectOrigin,
) error {

	idsToCheck := make([]string, 0, len(relationKeys)+len(objectTypeKeys))
	for _, key := range relationKeys {
		// TODO: check if we have them in oldIDtoNew
		if !bundle.HasRelation(key) {
			continue
		}

		idsToCheck = append(idsToCheck, key.BundledURL())
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
	oldIDtoNew map[string]string) (*domain.Details, error) {
	var respDetails *domain.Details
	spc, err := oc.spaceService.Get(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("get space %s: %w", spaceID, err)
	}
	sb, err := spc.CreateTreeObjectWithPayload(ctx, payload, func(id string) *smartblock.InitContext {
		// at this point, collection contains uuids, we need to replace them with new ids
		// it should happen before first index, otherwise uuids can be indexed as real objects
		if st.Store() != nil {
			oc.replaceInCollection(st, oldIDtoNew)
		}
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
	return respDetails, nil
}

func (oc *ObjectCreator) setRootBlock(snapshot *common.StateSnapshot, newID string) {
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

func (oc *ObjectCreator) onFinish(err error, spaceId string, st *state.State, filesToDelete []string) {
	if err != nil {
		for _, bl := range st.Blocks() {
			if f := bl.GetFile(); f != nil {
				oc.deleteFile(spaceId, f.Hash)
			}
			for _, hash := range filesToDelete {
				oc.deleteFile(spaceId, hash)
			}
		}
	}
}

func (oc *ObjectCreator) deleteFile(spaceId string, hash string) {
	inboundLinks, err := oc.objectStore.SpaceIndex(spaceId).GetOutboundLinksById(hash)
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
	var details []domain.Detail
	ids := st.CombinedDetails().GetStringList(bundle.RelationKeySpaceDashboardId)
	if len(ids) > 0 {
		details = append(details, domain.Detail{
			Key:   bundle.RelationKeySpaceDashboardId,
			Value: domain.StringList(ids),
		})
	}

	spaceName := st.CombinedDetails().GetString(bundle.RelationKeyName)
	if spaceName != "" {
		details = append(details, domain.Detail{
			Key:   bundle.RelationKeyName,
			Value: domain.String(spaceName),
		})
	}

	iconOption := st.CombinedDetails().GetInt64(bundle.RelationKeyIconOption)
	if iconOption != 0 {
		details = append(details, domain.Detail{
			Key:   bundle.RelationKeyIconOption,
			Value: domain.Int64(iconOption),
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

func (oc *ObjectCreator) resetState(newID string, st *state.State) *domain.Details {
	var respDetails *domain.Details
	err := cache.Do(oc.objectGetterDeleter, newID, func(b smartblock.SmartBlock) error {
		currentRevision := b.Details().GetInt64(bundle.RelationKeyRevision)
		newRevision := st.Details().GetInt64(bundle.RelationKeyRevision)
		if currentRevision > newRevision {
			log.With(zap.String("object id", newID)).Warnf("skipping object %s, revision %d > %d", st.Details().GetString(bundle.RelationKeyUniqueKey), currentRevision, newRevision)
			// never update objects with older revision
			// we use revision for bundled objects like relations and object types
			return nil
		}
		if st.ObjectTypeKey() == bundle.TypeKeyObjectType {
			template.InitTemplate(st, template.WithDetail(bundle.RelationKeyRecommendedLayout, domain.Int64(model.ObjectType_basic)))
		}
		err := history.ResetToVersion(b, st)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set state %s: %s", newID, err)
		}
		respDetails = b.CombinedDetails()
		return nil
	})
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to reset state %s: %s", newID, err)
	}
	return respDetails
}

func (oc *ObjectCreator) setFavorite(snapshot *common.StateSnapshot, newID string) {
	isFavorite := snapshot.Details.GetBool(bundle.RelationKeyIsFavorite)
	if isFavorite {
		err := oc.detailsService.SetIsFavorite(newID, true, false)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set isFavorite when importing object: %s", err)
		}
	}
}

func (oc *ObjectCreator) setArchived(snapshot *common.StateSnapshot, newID string) {
	isArchive := snapshot.Details.GetBool(bundle.RelationKeyIsArchived)
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

func (oc ObjectCreator) replaceInCollection(st *state.State, oldIDtoNew map[string]string) {
	objectsInCollections := st.GetStoreSlice(template.CollectionStoreKey)
	newObjs := make([]string, 0, len(objectsInCollections))
	for _, id := range objectsInCollections {
		if newId, ok := oldIDtoNew[id]; ok {
			newObjs = append(newObjs, newId)
		}
	}
	st.UpdateStoreSlice(template.CollectionStoreKey, newObjs)
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

func (oc *ObjectCreator) updateWidgetObject(st *state.State) (*domain.Details, string, error) {
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
	for key, value := range st.Details().Iterate() {
		if newKey, ok := oldIDtoNew[key.String()]; ok && newKey != key.String() {
			oc.updateDetails(st, domain.RelationKey(newKey), value, key)
		}
	}
	if newKey, ok := oldIDtoNew[st.ObjectTypeKey().String()]; ok {
		st.SetObjectTypeKey(domain.TypeKey(newKey))
	}
}

func (oc *ObjectCreator) updateDetails(st *state.State, newKey domain.RelationKey, value domain.Value, key domain.RelationKey) {
	st.SetDetail(newKey, value)
	st.RemoveRelation(key)
}
