package importer

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/relation"
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

type ObjectCreator struct {
	service       *block.Service
	objectCreator objectCreator
	core          core.Service
	updater       Updater
	syncFactory   *syncer.Factory
}

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State) (id string, newDetails *types.Struct, err error)
}

func NewCreator(service *block.Service, objCreator objectCreator, core core.Service, updater Updater, syncFactory *syncer.Factory) Creator {
	return &ObjectCreator{service: service, objectCreator: objCreator, core: core, updater: updater, syncFactory: syncFactory}
}

// Create creates smart blocks from given snapshots
func (oc *ObjectCreator) Create(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, pageID string, sbType smartblock.SmartBlockType, updateExisting bool) (*types.Struct, error) {
	isFavorite := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsFavorite.String())

	var err error

	if updateExisting {
		if details, err := oc.updater.Update(ctx, snapshot, pageID); err == nil {
			return details, nil
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

	if err = oc.validate(st); err != nil {
		return nil, fmt.Errorf("new id not found for '%s'", st.RootId())
	}

	defer func() {
		// delete file in ipfs if there is error after creation
		if err != nil {
			for _, bl := range st.Blocks() {
				if f := bl.GetFile(); f != nil {
					oc.deleteFile(f)
				}
			}
		}
	}()

	newID, details, err := oc.objectCreator.CreateSmartBlockFromState(context.TODO(), sbType, nil, nil, st)
	if err != nil {
		return nil, fmt.Errorf("crear object '%s'", st.RootId())
	}

	if isFavorite {
		err = oc.service.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{ContextId: pageID, IsFavorite: true})
		if err != nil {
			log.With(zap.String("object id", pageID)).Errorf("failed to set isFavorite when importing object %s: %s", pageID, err.Error())
			err = nil
		}
	}

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

func (oc *ObjectCreator) validate(st *state.State) (err error) {
	var relKeys []string
	for _, rel := range st.OldExtraRelations() {
		if !bundle.HasRelation(rel.Key) {
			log.Errorf("builtin objects should not contain custom relations, got %s in %s(%s)", rel.Name, st.RootId(), pbtypes.GetString(st.Details(), bundle.RelationKeyName.String()))
		}
	}
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if rb, ok := b.(relation.Block); ok {
			relKeys = append(relKeys, rb.Model().GetRelation().Key)
		}
		return true
	})
	for _, rk := range relKeys {
		if !st.HasRelation(rk) {
			return fmt.Errorf("bundled template validation: relation '%v' exists in block but not in extra relations", rk)
		}
	}
	return nil
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
		for _, b := range snapshot.Blocks {
			childrenIds = append(childrenIds, b.Id)
		}
		snapshot.Blocks = append(snapshot.Blocks, &model.Block{
			Id:          pageID,
			Content:     &model.BlockContentOfSmartblock{},
			ChildrenIds: childrenIds,
		})
	}
}

func (oc *ObjectCreator) deleteFile(f *model.BlockContentFile) {
	inboundLinks, err := oc.core.ObjectStore().GetOutboundLinksById(f.Hash)
	if err != nil {
		log.With("file", f.Hash).Errorf("failed to get inbound links for file: %s", err.Error())
		return
	}
	if len(inboundLinks) == 0 {
		if err = oc.core.ObjectStore().DeleteObject(f.Hash); err != nil {
			log.With("file", f.Hash).Errorf("failed to delete file from objectstore: %s", err.Error())
		}
		if err = oc.core.FileStore().DeleteByHash(f.Hash); err != nil {
			log.With("file", f.Hash).Errorf("failed to delete file from filestore: %s", err.Error())
		}
		if _, err = oc.core.FileOffload(f.Hash); err != nil {
			log.With("file", f.Hash).Errorf("failed to offload file: %s", err.Error())
		}
		if err = oc.core.FileStore().DeleteFileKeys(f.Hash); err != nil {
			log.With("file", f.Hash).Errorf("failed to delete file keys: %s", err.Error())
		}
	}
}
