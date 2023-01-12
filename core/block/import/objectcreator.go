package importer

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State) (id string, newDetails *types.Struct, err error)
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
}

func NewCreator(service *block.Service,
	objCreator objectCreator,
	updater Updater,
	syncFactory *syncer.Factory,
	relationCreator RelationCreator) Creator {
	return &ObjectCreator{
		service:         service,
		objCreator:      objCreator,
		updater:         updater,
		syncFactory:     syncFactory,
		relationCreator: relationCreator,
	}
}

// Create creates smart blocks from given snapshots
func (oc *ObjectCreator) Create(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	relations []*converter.Relation,
	pageID string,
	sbType smartblock.SmartBlockType,
	updateExisting bool) (*types.Struct, error) {
	isFavorite := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsFavorite.String())

	var err error

	if updateExisting {
		var (
			filesToDelete []string
			details       *types.Struct
		)
		if details, filesToDelete, err = oc.updater.Update(ctx, snapshot, relations, pageID); err == nil {
			return details, nil
		}
		for _, hash := range filesToDelete {
			oc.deleteFile(hash)
		}
		log.Warn("failed to update existing object: %s", err)
	}

	var found bool
	for _, b := range snapshot.Blocks {
		if b.Id == pageID {
			found = true
			break
		}
	}
	if !found {
		oc.addRootBlock(snapshot, pageID)
	}

	st := state.NewDocFromSnapshot(pageID, &pb.ChangeSnapshot{Data: snapshot}).(*state.State)

	st.SetRootId(pageID)

	st.RemoveDetail(bundle.RelationKeyCreator.String(), bundle.RelationKeyLastModifiedBy.String())
	st.SetLocalDetail(bundle.RelationKeyCreator.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyLastModifiedBy.String(), pbtypes.String(addr.AnytypeProfileId))
	st.InjectDerivedDetails()

	var filesToDelete []string
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

	newID, details, err := oc.objCreator.CreateSmartBlockFromState(context.TODO(), sbType, nil, nil, st)
	if err != nil {
		return nil, fmt.Errorf("create object '%s'", st.RootId())
	}

	var oldRelationBlocksToNew map[string]*model.Block
	filesToDelete, oldRelationBlocksToNew, err = oc.relationCreator.CreateRelations(ctx, snapshot, pageID, relations)
	if err != nil {
		return nil, fmt.Errorf("relation create '%s'", err)
	}

	if isFavorite {
		err = oc.service.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{ContextId: pageID, IsFavorite: true})
		if err != nil {
			log.With(zap.String("object id", pageID)).Errorf("failed to set isFavorite when importing object %s: %s", pageID, err.Error())
			err = nil
		}
	}

	oc.relationCreator.ReplaceRelationBlock(ctx, oldRelationBlocksToNew, pageID)

	st.Iterate(func(bl simple.Block) (isContinue bool) {
		s := oc.syncFactory.GetSyncer(bl)
		if s != nil {
			if serr := s.Sync(ctx, newID, bl); serr != nil {
				log.With(zap.String("object id", pageID)).Errorf("sync: %s", serr)
			}
		}
		return true
	})

	return details, nil
}

func (oc *ObjectCreator) addRootBlock(snapshot *model.SmartBlockSnapshotBase, pageID string) {
	var (
		childrenIds = make([]string, 0, len(snapshot.Blocks))
		err         error
	)
	for i, b := range snapshot.Blocks {
		_, err = thread.Decode(b.Id)
		if err == nil {
			childrenIds = append(childrenIds, b.ChildrenIds...)
			snapshot.Blocks[i] = &model.Block{
				Id:          pageID,
				Content:     &model.BlockContentOfSmartblock{},
				ChildrenIds: childrenIds,
			}
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
