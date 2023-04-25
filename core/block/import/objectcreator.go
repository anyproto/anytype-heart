package importer

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/relation"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type ObjectCreator struct {
	service       *block.Service
	objectCreator objectCreator
	core          core.Service
	syncFactory   *syncer.Factory
	oldIDToNew    map[string]string
}

type objectCreator interface {
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State) (id string, newDetails *types.Struct, err error)
}

func NewCreator(service *block.Service,
	objCreator objectCreator,
	core core.Service,
	syncFactory *syncer.Factory) Creator {
	return &ObjectCreator{service: service,
		objectCreator: objCreator,
		core:          core,
		syncFactory:   syncFactory,
		oldIDToNew:    map[string]string{}}
}

// Create creates smart blocks from given snapshots
func (oc *ObjectCreator) Create(ctx *session.Context, sn *converter.Snapshot, oldIDtoNew map[string]string, existing bool, workspaceID string) (*types.Struct, error) {
	snapshot := sn.Snapshot
	isFavorite := pbtypes.GetBool(snapshot.Details, bundle.RelationKeyIsFavorite.String())

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
	if !found && !existing {
		oc.addRootBlock(snapshot, newID)
	}

	st := state.NewDocFromSnapshot(newID, &pb.ChangeSnapshot{Data: snapshot}).(*state.State)

	st.SetRootId(newID)

	st.RemoveDetail(bundle.RelationKeyCreator.String(), bundle.RelationKeyLastModifiedBy.String())
	st.SetLocalDetail(bundle.RelationKeyCreator.String(), pbtypes.String(addr.AnytypeProfileId))
	st.SetLocalDetail(bundle.RelationKeyLastModifiedBy.String(), pbtypes.String(addr.AnytypeProfileId))
	st.InjectDerivedDetails()

	//if err = oc.validate(st); err != nil {
	//	return nil, fmt.Errorf("valdation failed '%s'", err)
	//}

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

	if err = converter.UpdateLinksToObjects(st, oldIDtoNew, pageID); err != nil {
		log.With("object", pageID).Errorf("failed to update objects ids: %s", err.Error())
	}

	oc.updateRelationsIDs(st, pageID, oldIDtoNew)

	if workspaceID == "" {
		workspaceID, err = oc.core.GetWorkspaceIdForObject(pageID)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to get workspace id %s: %s", pageID, err.Error())
		}
	}

	if snapshot.Details != nil {
		snapshot.Details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceID)
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
		err = oc.service.Do(newID, func(b sb.SmartBlock) error {
			if _, ok := snapshot.GetDetails().GetFields()[bundle.RelationKeyIsDeleted.String()]; ok {
				err := oc.service.RemoveSubObjectsInWorkspace([]string{newID}, workspaceID)
				if err != nil {
					log.With(zap.String("object id", newID)).Errorf("failed to remove from collections %s: %s", newID, err.Error())
				}
			}

			return b.SetDetails(ctx, details, true)
		})
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to reset state state %s: %s", newID, err.Error())
		}
		return nil, nil
	}

	err = oc.service.Do(newID, func(b sb.SmartBlock) error {
		err = b.SetObjectTypes(ctx, snapshot.ObjectTypes)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set object types %s: %s", newID, err.Error())
		}

		err = b.ResetToVersion(st)
		if err != nil {
			log.With(zap.String("object id", newID)).Errorf("failed to set state %s: %s", newID, err.Error())
		}
		return b.SetDetails(ctx, details, true)
	})
	if err != nil {
		log.With(zap.String("object id", newID)).Errorf("failed to reset state state %s: %s", newID, err.Error())
	}

	if isFavorite {
		err = oc.service.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{ContextId: newID, IsFavorite: true})
		if err != nil {
			log.With(zap.String("object id", newID)).
				Errorf("failed to set isFavorite when importing object %s: %s", newID, err.Error())
			err = nil
		}
	}

	st.Iterate(func(bl simple.Block) (isContinue bool) {
		s := oc.syncFactory.GetSyncer(bl)
		if s != nil {
			if sErr := s.Sync(ctx, newID, bl); sErr != nil {
				log.With(zap.String("object id", newID)).Errorf("sync: %s", sErr)
			}
		}
		return true
	})

	return nil, nil
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
