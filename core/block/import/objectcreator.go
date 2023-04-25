package importer

import (
	"context"
	"fmt"
	"sync"

	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

const relationsLimit = 10

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSubObjectInWorkspace(details *types.Struct, workspaceID string) (id string, newDetails *types.Struct, err error)
	CreateSubObjectsInWorkspace(details []*types.Struct) (ids []string, objects []*types.Struct, err error)
}

type ObjectCreator struct {
	service         *block.Service
	objCreator      objectCreator
	core            core.Service
	objectStore     objectstore.ObjectStore
	fileStore       filestore.FileStore
	relationCreator RelationCreator
	syncFactory     *syncer.Factory
	mu              sync.Mutex
}

func NewCreator(service *block.Service,
	objCreator objectCreator,
	core core.Service,
	syncFactory *syncer.Factory,
	relationCreator RelationCreator,
	objectStore objectstore.ObjectStore,
	fileStore filestore.FileStore,
) Creator {
	return &ObjectCreator{
		service:         service,
		objCreator:      objCreator,
		core:            core,
		syncFactory:     syncFactory,
		relationCreator: relationCreator,
		objectStore:     objectStore,
		fileStore:       fileStore,
	}
}

// Create creates smart blocks from given snapshots
func (oc *ObjectCreator) Create(ctx *session.Context,
	sn *converter.Snapshot,
	relations []*converter.Relation,
	oldIDtoNew map[string]string,
	existing bool) (*types.Struct, string, error) {
	snapshot := sn.Snapshot.Data
	isFavorite := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsFavorite.String())
	isArchive := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsArchived.String())

	var err error
	newID := oldIDtoNew[sn.Id]
	oc.updateRootBlock(snapshot, newID)

	oc.setWorkspaceID(err, newID, snapshot)

	var oldRelationBlocksToNew map[string]*model.Block
	filesToDelete, oldRelationBlocksToNew, createdRelations, err := oc.relationCreator.CreateRelations(ctx, snapshot, newID, relations)
	if err != nil {
		return nil, "", fmt.Errorf("relation create '%s'", err)
	}

	st := state.NewDocFromSnapshot(newID, sn.Snapshot).(*state.State)
	st.SetRootId(newID)

	defer func() {
		// delete file in ipfs if there is error after creation
		oc.onFinish(err, st, filesToDelete)
	}()

	converter.UpdateRelationsIDs(st, newID, oldIDtoNew)
	details := oc.getDetails(st.Details())
	if sn.SbType == coresb.SmartBlockTypeSubObject {
		return oc.handleSubObject(ctx, snapshot, newID, details), "", nil
	}

	if err = converter.UpdateLinksToObjects(st, oldIDtoNew, newID); err != nil {
		log.With("object", newID).Errorf("failed to update objects ids: %s", err.Error())
	}

	if st.GetStoreSlice(template.CollectionStoreKey) != nil {
		oc.updateLinksInCollections(st, oldIDtoNew)
		if err = oc.addRelationsToCollectionDataView(st, relations, createdRelations); err != nil {
			log.With("object", newID).Errorf("failed to add relations to object view: %s", err.Error())
		}
	}

	if sn.SbType == coresb.SmartBlockTypeWorkspace {
		oc.handleWorkspace(ctx, details, newID, st, oldIDtoNew, sn.Snapshot.Data.Details)
		return nil, newID, nil
	}

	respDetails := oc.resetState(ctx, newID, st, details)

	if isFavorite {
		err = oc.service.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{ContextId: newID, IsFavorite: true})
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set isFavorite when importing object: %s", err.Error())
			err = nil
		}
	}

	if isArchive {
		err = oc.service.SetPageIsArchived(pb.RpcObjectSetIsArchivedRequest{ContextId: newID, IsArchived: true})
		if err != nil {
			log.With(zap.String("object id", newID)).
				Errorf("failed to set isFavorite when importing object %s: %s", newID, err.Error())
			err = nil
		}
	}

	oc.relationCreator.ReplaceRelationBlock(ctx, oldRelationBlocksToNew, newID)

	syncErr := oc.syncFilesAndLinks(ctx, st, newID)
	if syncErr != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to sync %s: %s", newID, err.Error())
	}

	return respDetails, newID, nil
}

func (oc *ObjectCreator) handleWorkspace(ctx *session.Context,
	details []*pb.RpcObjectSetDetailsDetail,
	newID string,
	st *state.State,
	oldToNew map[string]string,
	d *types.Struct) {
	if err := oc.createRelationsInWorkspace(newID, st); err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to create sub objects in workspace: %s", err.Error())
	}
	err := block.Do(oc.service, newID, func(b basic.CommonOperations) error {
		return b.SetDetails(ctx, details, true)
	})
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to set details %s: %s", newID, err.Error())
	}
	oc.setSpaceDashboardID(newID, d, oldToNew)
}

func (oc *ObjectCreator) getDetails(d *types.Struct) []*pb.RpcObjectSetDetailsDetail {
	var details []*pb.RpcObjectSetDetailsDetail
	for key, value := range d.Fields {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   key,
			Value: value,
		})
	}
	return details
}

func (oc *ObjectCreator) setWorkspaceID(err error, newID string, snapshot *model.SmartBlockSnapshotBase) {
	workspaceID, err := oc.core.GetWorkspaceIdForObject(newID)
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to get workspace id %s: %s", newID, err.Error())
	}

	if snapshot.Details != nil && snapshot.Details.Fields != nil {
		snapshot.Details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceID)
	}
}

func (oc *ObjectCreator) updateRootBlock(snapshot *model.SmartBlockSnapshotBase, newID string) {
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

func (oc *ObjectCreator) syncFilesAndLinks(ctx *session.Context, st *state.State, newID string) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		s := oc.syncFactory.GetSyncer(bl)
		if s != nil {
			if sErr := s.Sync(ctx, newID, bl); sErr != nil {
				log.With(zap.String("object id", newID)).Errorf("sync: %s", sErr)
			}
		}
		return true
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

func (oc *ObjectCreator) setSpaceDashboardID(newID string, details *types.Struct, oldIDtoNew map[string]string) {
	spaceDashBoardID := pbtypes.GetString(details, bundle.RelationKeySpaceDashboardId.String())
	if id, ok := oldIDtoNew[spaceDashBoardID]; ok {
		e := block.Do(oc.service, newID, func(ws basic.CommonOperations) error {
			if err := ws.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{
				{
					Key:   bundle.RelationKeySpaceDashboardId.String(),
					Value: pbtypes.String(id),
				},
			}, false); err != nil {
				return err
			}
			return nil
		})
		if e != nil {
			log.Errorf("failed to set spaceDashBoardID, %s", e)
		}
	}
}

func (oc *ObjectCreator) resetState(ctx *session.Context, newID string, st *state.State, details []*pb.RpcObjectSetDetailsDetail) *types.Struct {
	var respDetails *types.Struct
	err := oc.service.Do(newID, func(b sb.SmartBlock) error {
		err := history.ResetToVersion(b, st)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set state %s: %s", newID, err.Error())
		}
		commonOperations, ok := b.(basic.CommonOperations)
		if !ok {
			return fmt.Errorf("common operations is not allowed for this object")
		}
		err = commonOperations.SetDetails(ctx, details, true)
		if err != nil {
			return err
		}
		err = commonOperations.FeaturedRelationAdd(ctx, bundle.RelationKeyType.String())
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
		Id:          pageID,
		Content:     &model.BlockContentOfSmartblock{},
		ChildrenIds: childrenIds,
	})
}

func (oc *ObjectCreator) deleteFile(hash string) {
	inboundLinks, err := oc.objectStore.GetOutboundLinksById(hash)
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

func (oc *ObjectCreator) handleSubObject(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, newID string, details []*pb.RpcObjectSetDetailsDetail) *types.Struct {
	defer oc.mu.Unlock()
	oc.mu.Lock()
	if snapshot.GetDetails() != nil && snapshot.GetDetails().GetFields() != nil {
		if _, ok := snapshot.GetDetails().GetFields()[bundle.RelationKeyIsDeleted.String()]; ok {
			err := oc.service.RemoveSubObjectsInWorkspace([]string{newID}, oc.core.PredefinedBlocks().Account, true)
			if err != nil {
				log.With(zap.String("object id", newID)).Errorf("failed to remove from collections %s: %s", newID, err.Error())
			}
		}
	}
	err := block.Do(oc.service, newID, func(b basic.CommonOperations) error {
		return b.SetDetails(ctx, details, true)
	})
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to reset state state %s: %s", newID, err.Error())
	}
	return nil
}

func (oc *ObjectCreator) addRelationsToCollectionDataView(st *state.State, rels []*converter.Relation, createdRelations map[string]RelationsIDToFormat) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		if dv, ok := bl.(simpleDataview.Block); ok {
			return oc.handleDataviewBlock(bl, rels, createdRelations, dv)
		}
		return true
	})
}

func (oc *ObjectCreator) handleDataviewBlock(bl simple.Block, rels []*converter.Relation, createdRelations map[string]RelationsIDToFormat, dv simpleDataview.Block) bool {
	for i, rel := range rels {
		if relation, exist := createdRelations[rel.Name]; exist {
			isVisible := i <= relationsLimit
			if err := oc.addRelationToView(bl, relation, rel, dv, isVisible); err != nil {
				log.Errorf("can't add relations to view: %s", err.Error())
			}
		}
	}
	return false
}

func (oc *ObjectCreator) addRelationToView(bl simple.Block, relation RelationsIDToFormat, rel *converter.Relation, dv simpleDataview.Block, visible bool) error {
	for _, relFormat := range relation {
		if relFormat.Format == rel.Format {
			if len(bl.Model().GetDataview().GetViews()) == 0 {
				return nil
			}
			for _, view := range bl.Model().GetDataview().GetViews() {
				err := dv.AddViewRelation(view.GetId(), &model.BlockContentDataviewRelation{
					Key:       relFormat.ID,
					IsVisible: visible,
					Width:     192,
				})
				if err != nil {
					return err
				}
			}
			err := dv.AddRelation(&model.RelationLink{
				Key:    relFormat.ID,
				Format: relFormat.Format,
			})
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (oc *ObjectCreator) updateLinksInCollections(st *state.State, oldIDtoNew map[string]string) {
	var existedObjects []string
	err := block.DoStateCtx(oc.service, nil, st.RootId(), func(s *state.State, b sb.SmartBlock) error {
		existedObjects = s.GetStoreSlice(template.CollectionStoreKey)
		return nil
	})
	if err != nil {
		log.Errorf("failed to get existed objects in collection, %s", err)
	}
	objectsInCollections := st.GetStoreSlice(template.CollectionStoreKey)
	for i, id := range objectsInCollections {
		if newID, ok := oldIDtoNew[id]; ok {
			objectsInCollections[i] = newID
		}
	}
	result := slice.Union(existedObjects, objectsInCollections)
	st.UpdateStoreSlice(template.CollectionStoreKey, result)
}

// createRelationsInWorkspace compare current workspace store with imported and create objects, which are absent in current workspace
func (oc *ObjectCreator) createRelationsInWorkspace(newID string, st *state.State) error {
	var ids []string
	err := oc.service.Do(newID, func(b sb.SmartBlock) error {
		bs := b.NewState()
		oldStore := bs.Store()
		newStore := st.Store()
		ids = oc.compareStoresAndGetAbsentObjectsIDs(oldStore, newStore)
		return nil
	})
	if err != nil {
		return err
	}
	_, _, err = oc.service.AddSubObjectsToWorkspace(ids, newID)
	if err != nil {
		return err
	}
	return nil
}

func (oc *ObjectCreator) compareStoresAndGetAbsentObjectsIDs(oldStore *types.Struct, newStore *types.Struct) []string {
	var ids []string
	for colName, objects := range oldStore.GetFields() {
		oldStr := objects.GetStructValue()
		if objectsFromNewStore, ok := newStore.GetFields()[colName]; ok {
			newStr := objectsFromNewStore.GetStructValue()
			diff := pbtypes.StructDiff(oldStr, newStr)
			ids = append(ids, oc.getAbsentObjectsIDs(diff)...)
		}
	}
	return ids
}

func (oc *ObjectCreator) getAbsentObjectsIDs(diff *types.Struct) []string {
	var ids []string
	for objectName, details := range diff.GetFields() {
		var isSystem bool
		for _, relation := range bundle.SystemRelations {
			if string(relation) == objectName {
				isSystem = true
				break
			}
		}
		for _, objTypes := range bundle.SystemTypes {
			if string(objTypes) == objectName {
				isSystem = true
				break
			}
		}
		if isSystem {
			continue
		}
		if source := pbtypes.GetString(details.GetStructValue(), bundle.RelationKeySourceObject.String()); source != "" {
			ids = append(ids, source)
		}
	}
	return ids
}
