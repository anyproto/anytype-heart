package importer

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSubObjectInWorkspace(details *types.Struct, workspaceID string) (id string, newDetails *types.Struct, err error)
	CreateSubObjectsInWorkspace(details []*types.Struct) (ids []string, objects []*types.Struct, err error)
}

type ObjectCreator struct {
	service         *block.Service
	objCreator      objectCreator
	core            core.Service
	updater         Updater
	relationCreator RelationCreator
	syncFactory     *syncer.Factory
	oldIDToNew      map[string]string
}

func NewCreator(service *block.Service,
	objCreator objectCreator,
	updater Updater,
	core core.Service,
	syncFactory *syncer.Factory,
	relationCreator RelationCreator) Creator {
	return &ObjectCreator{
		service:         service,
		objCreator:      objCreator,
		core:            core,
		updater:         updater,
		syncFactory:     syncFactory,
		relationCreator: relationCreator,
		oldIDToNew:      map[string]string{},
	}
}

// Create creates smart blocks from given snapshots
func (oc *ObjectCreator) Create(ctx *session.Context,
	sn *converter.Snapshot,
	relations []*converter.Relation,
	pageID string,
	oldIDtoNew map[string]string,
	updateExisting bool) (*types.Struct, error) {
	snapshot := sn.Snapshot
	isFavorite := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsFavorite.String())

	var err error

	newID := oldIDtoNew[pageID]
	var found bool
	for _, b := range snapshot.Blocks {
		if b.Id == newID {
			found = true
			break
		}
	}
	if !found && !updateExisting {
		oc.addRootBlock(snapshot, newID)
	}

	var workspaceID string
	if updateExisting {
		workspaceID, err = oc.core.GetWorkspaceIdForObject(newID)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to get workspace id %s: %s", pageID, err.Error())
		}
	}

	if workspaceID == "" {
		// todo: pass it explicitly
		workspaceID = oc.core.PredefinedBlocks().Account
	}

	if snapshot.Details != nil && snapshot.Details.Fields != nil {
		snapshot.Details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceID)
	}

	var details []*pb.RpcObjectSetDetailsDetail

	var oldRelationBlocksToNew map[string]*model.Block
	filesToDelete, oldRelationBlocksToNew, createdRelations, err := oc.relationCreator.CreateRelations(ctx, snapshot, newID, relations)
	if err != nil {
		return nil, fmt.Errorf("relation create '%s'", err)
	}

	if snapshot.Details != nil {
		for key, value := range snapshot.Details.Fields {
			details = append(details, &pb.RpcObjectSetDetailsDetail{
				Key:   key,
				Value: value,
			})
		}
	}

	st := state.NewDocFromSnapshot(newID, &pb.ChangeSnapshot{Data: snapshot}).(*state.State)
	st.SetRootId(newID)

	st.RemoveDetail(bundle.RelationKeyCreator.String(), bundle.RelationKeyLastModifiedBy.String())
	st.InjectDerivedDetails()

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

	if err = oc.updateLinksToObjects(st, oldIDtoNew, newID); err != nil {
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
		log.With(zap.String("object id", newID)).Errorf("failed to reset state: %s", err.Error())
	}

	if isFavorite {
		err = oc.service.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{ContextId: newID, IsFavorite: true})
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set isFavorite when importing object: %s", err.Error())
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

	return respDetails, nil
}

func (oc *ObjectCreator) addRootBlock(snapshot *model.SmartBlockSnapshotBase, pageID string) {
	var (
		childrenIds = make([]string, 0, len(snapshot.Blocks))
		err         error
	)
	for i, b := range snapshot.Blocks {
		if _, ok := b.Content.(*model.BlockContentOfSmartblock); ok {
			snapshot.Blocks[i].Id = pageID
			break
		}
	}
	if err != nil {
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
}

func (oc *ObjectCreator) deleteFile(hash string) {
	inboundLinks, err := oc.core.ObjectStore().GetOutboundLinksById(hash)
	if err != nil {
		log.With("file", hash).Errorf("failed to get inbound links for file: %s", err.Error())
		return
	}
	if len(inboundLinks) == 0 {
		if err = oc.core.ObjectStore().DeleteObject(hash); err != nil {
			log.With("file", hash).Errorf("failed to delete file from objectstore: %s", err.Error())
		}
		if err = oc.core.FileStore().DeleteByHash(hash); err != nil {
			log.With("file", hash).Errorf("failed to delete file from filestore: %s", err.Error())
		}
		if _, err = oc.core.FileOffload(hash); err != nil {
			log.With("file", hash).Errorf("failed to offload file: %s", err.Error())
		}
		if err = oc.core.FileStore().DeleteFileKeys(hash); err != nil {
			log.With("file", hash).Errorf("failed to delete file keys: %s", err.Error())
		}
	}
}

func (oc *ObjectCreator) updateRelationsIDs(st *state.State, pageID string, oldIDtoNew map[string]string) {
	for k, v := range st.Details().GetFields() {
		rel, err := bundle.GetRelation(bundle.RelationKey(k))
		if err != nil {
			log.With("object", pageID).Errorf("failed to find relation %s: %s", k, err.Error())
			continue
		}
		if rel.Format != model.RelationFormat_object {
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
	for _, rel := range rels {
		if relation, exist := createdRelations[rel.Name]; exist {
			if err := oc.addRelationToView(bl, relation, rel, dv); err != nil {
				log.Errorf("can't add relations to view: %s", err.Error())
			}
		}
	}
	return false
}

func (oc *ObjectCreator) addRelationToView(bl simple.Block, relation RelationsIDToFormat, rel *converter.Relation, dv simpleDataview.Block) error {
	for _, relFormat := range relation {
		if relFormat.Format == rel.Format {
			if len(bl.Model().GetDataview().GetViews()) == 0 {
				return nil
			}
			err := dv.AddViewRelation(bl.Model().GetDataview().GetViews()[0].GetId(), &model.BlockContentDataviewRelation{
				Key:       relFormat.ID,
				IsVisible: true,
				Width:     192,
			})
			if err != nil {
				return err
			}
			err = dv.AddRelation(&model.RelationLink{
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
	objectsInCollections := pbtypes.GetStringList(st.Store(), sb.CollectionStoreKey)
	newIDs := make([]string, 0)
	for _, id := range objectsInCollections {
		if newID, ok := oldIDtoNew[id]; ok {
			newIDs = append(newIDs, newID)
		}
	}
	st.StoreSlice(sb.CollectionStoreKey, newIDs)
}

func (oc *ObjectCreator) updateLinksToObjects(st *state.State, oldIDtoNew map[string]string, pageID string) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		switch a := bl.(type) {
		case link.Block:
			newTarget := oldIDtoNew[a.Model().GetLink().TargetBlockId]
			if newTarget == "" {
				// maybe we should panic here?
				log.With("object", st.RootId()).Errorf("cant find target id for link: %s", a.Model().GetLink().TargetBlockId)
				return true
			}

			a.Model().GetLink().TargetBlockId = newTarget
			st.Set(simple.New(a.Model()))
		case bookmark.Block:
			newTarget := oldIDtoNew[a.Model().GetBookmark().TargetObjectId]
			if newTarget == "" {
				// maybe we should panic here?
				log.With("object", pageID).Errorf("cant find target id for bookmark: %s", a.Model().GetBookmark().TargetObjectId)
				return true
			}

			a.Model().GetBookmark().TargetObjectId = newTarget
			st.Set(simple.New(a.Model()))
		case text.Block:
			for i, mark := range a.Model().GetText().GetMarks().GetMarks() {
				if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
					continue
				}
				newTarget := oldIDtoNew[mark.Param]
				if newTarget == "" {
					log.With("object", pageID).Errorf("cant find target id for mention: %s", mark.Param)
					continue
				}

				a.Model().GetText().GetMarks().GetMarks()[i].Param = newTarget
			}
			st.Set(simple.New(a.Model()))
		}
		return true
	})
}
