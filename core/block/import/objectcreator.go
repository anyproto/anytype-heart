package importer

import (
	"context"
	"fmt"
	"sync"

	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
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

	var (
		err    error
		pageID = sn.Id
	)

	newID := oldIDtoNew[pageID]
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

	workspaceID, err := oc.core.GetWorkspaceIdForObject(newID)
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to get workspace id %s: %s", pageID, err.Error())
	}

	if snapshot.Details != nil && snapshot.Details.Fields != nil {
		snapshot.Details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceID)
	}

	var oldRelationBlocksToNew map[string]*model.Block
	filesToDelete, oldRelationBlocksToNew, createdRelations, err := oc.relationCreator.CreateRelations(ctx, snapshot, newID, relations)
	if err != nil {
		return nil, "", fmt.Errorf("relation create '%s'", err)
	}
	var details []*pb.RpcObjectSetDetailsDetail
	if snapshot.Details != nil {
		for key, value := range snapshot.Details.Fields {
			details = append(details, &pb.RpcObjectSetDetailsDetail{
				Key:   key,
				Value: value,
			})
		}
	}
	if sn.SbType == coresb.SmartBlockTypeSubObject {
		return oc.handleSubObject(ctx, snapshot, newID, workspaceID, details), "", nil
	}

	st := state.NewDocFromSnapshot(newID, sn.Snapshot).(*state.State)
	st.SetRootId(newID)

	defer func() {
		// delete file in ipfs if there is error after creation
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
	}()

	if err = converter.UpdateLinksToObjects(st, oldIDtoNew, newID); err != nil {
		log.With("object", newID).Errorf("failed to update objects ids: %s", err.Error())
	}

	if sn.SbType == coresb.SmartBlockTypeCollection {
		oc.updateLinksInCollections(st, oldIDtoNew)
		if err = oc.addRelationsToCollectionDataView(st, relations, createdRelations); err != nil {
			log.With("object", newID).Errorf("failed to add relations to object view: %s", err.Error())
		}
	}

	var respDetails *types.Struct
	err = oc.service.Do(newID, func(b sb.SmartBlock) error {
		err = b.SetObjectTypes(ctx, snapshot.ObjectTypes)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set object types %s: %s", newID, err.Error())
		}

		err = b.ResetToVersion(st)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set state %s: %s", newID, err.Error())
		}

		err = b.SetDetails(ctx, details, true)
		if err != nil {
			return err
		}
		respDetails = b.CombinedDetails()
		return nil
	})
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to reset state %s: %s", newID, err.Error())
	}

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

	st.Iterate(func(bl simple.Block) (isContinue bool) {
		s := oc.syncFactory.GetSyncer(bl)
		if s != nil {
			if sErr := s.Sync(ctx, newID, bl); sErr != nil {
				log.With(zap.String("object id", newID)).Errorf("sync: %s", sErr)
			}
		}
		return true
	})

	return respDetails, newID, nil
}

func (oc *ObjectCreator) addRootBlock(snapshot *model.SmartBlockSnapshotBase, pageID string) {
	var (
		childrenIds = make([]string, 0, len(snapshot.Blocks))
	)
	for i, b := range snapshot.Blocks {
		if _, ok := b.Content.(*model.BlockContentOfSmartblock); ok {
			snapshot.Blocks[i].Id = pageID
			return
		}
	}
	notRootBlockChild := make(map[string]bool, 0)
	for _, b := range snapshot.Blocks {
		if len(b.ChildrenIds) != 0 {
			for _, id := range b.ChildrenIds {
				notRootBlockChild[id] = true
			}
		}
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
		log.With("file", hash).Errorf("failed to get inbound links for file: %s", err.Error())
		return
	}
	if len(inboundLinks) == 0 {
		if err = oc.objectStore.DeleteObject(hash); err != nil {
			log.With("file", hash).Errorf("failed to delete file from objectstore: %s", err.Error())
		}
		if err = oc.fileStore.DeleteByHash(hash); err != nil {
			log.With("file", hash).Errorf("failed to delete file from filestore: %s", err.Error())
		}
		if _, err = oc.core.FileOffload(hash); err != nil {
			log.With("file", hash).Errorf("failed to offload file: %s", err.Error())
		}
		if err = oc.fileStore.DeleteFileKeys(hash); err != nil {
			log.With("file", hash).Errorf("failed to delete file keys: %s", err.Error())
		}
	}
}

func (oc *ObjectCreator) handleSubObject(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	newID, workspaceID string,
	details []*pb.RpcObjectSetDetailsDetail) *types.Struct {
	defer oc.mu.Unlock()
	oc.mu.Lock()
	if snapshot.GetDetails() != nil && snapshot.GetDetails().GetFields() != nil {
		if _, ok := snapshot.GetDetails().GetFields()[bundle.RelationKeyIsDeleted.String()]; ok {
			err := oc.service.RemoveSubObjectsInWorkspace([]string{newID}, workspaceID, true)
			if err != nil {
				log.With(zap.String("object id", newID)).Errorf("failed to remove from collections %s: %s", newID, err.Error())
			}
		}
	}
	err := oc.service.Do(newID, func(b sb.SmartBlock) error {
		return b.SetDetails(ctx, details, true)
	})
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to reset state state %s: %s", newID, err.Error())
	}
	return nil
}

func (oc *ObjectCreator) updateRelationsIDs(st *state.State, pageID string, oldIDtoNew map[string]string) {
	for k, v := range st.Details().GetFields() {
		rel, err := bundle.GetRelation(bundle.RelationKey(k))
		if err != nil {
			log.With("object", pageID).Errorf("failed to find relation %s: %s", k, err.Error())
			continue
		}
		if rel.Format != model.RelationFormat_object &&
			rel.Format != model.RelationFormat_tag &&
			rel.Format != model.RelationFormat_status {
			continue
		}

		vals := pbtypes.GetStringListValue(v)
		for i, val := range vals {
			if bundle.HasRelation(val) {
				continue
			}
			newTarget := oldIDtoNew[val]
			if newTarget == "" {
				log.With("object", pageID).Errorf("cant find target id for relation %s: %s", k, val)
				continue
			}
			vals[i] = newTarget

		}
		st.SetDetail(k, pbtypes.StringList(vals))
	}
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
		existedObjects = pbtypes.GetStringList(s.Store(), sb.CollectionStoreKey)
		return nil
	})
	if err != nil {
		log.Errorf("failed to get existed objects in collection, %s", err)
	}
	objectsInCollections := pbtypes.GetStringList(st.Store(), sb.CollectionStoreKey)
	for i, id := range objectsInCollections {
		if newID, ok := oldIDtoNew[id]; ok {
			objectsInCollections[i] = newID
		}
	}
	result := slice.Union(existedObjects, objectsInCollections)
	st.StoreSlice(sb.CollectionStoreKey, result)
}
