package api

import (
	"context"
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

// objectCreateAdapter implements apicore.ObjectCreator over the standard
// object-creation service: snapshot → state.NewDocFromSnapshot →
// CreateSmartBlockFromState — the API v2 create path (APIV2.md §2 Phase 2).
// The whole document lands as the object's initial state, so composite
// creates (a set with its dataview) are one change set (§8/R10), and the
// creation routes through the editor machinery (restrictions, derived
// details, undo) exactly like user-initiated creates.
type objectCreateAdapter struct {
	creator objectcreator.Service
	spaces  space.Service
}

func newObjectCreateAdapter(creator objectcreator.Service, spaces space.Service) apicore.ObjectCreator {
	return &objectCreateAdapter{creator: creator, spaces: spaces}
}

func (a *objectCreateAdapter) CreateObjectFromSnapshot(ctx context.Context, spaceId string, snapshot *model.SmartBlockSnapshotBase) (string, error) {
	rootId := snapshotRootId(snapshot)
	if rootId == "" {
		return "", fmt.Errorf("snapshot has no root block")
	}
	createState, err := state.NewDocFromSnapshot(rootId, &pb.ChangeSnapshot{Data: snapshot})
	if err != nil {
		return "", fmt.Errorf("state from snapshot: %w", err)
	}
	createState.SetDetail(bundle.RelationKeyOrigin, domain.Int64(int64(model.ObjectOrigin_api)))

	typeKeys := createState.ObjectTypeKeys()
	if len(typeKeys) == 0 {
		typeKeys = []domain.TypeKey{bundle.TypeKeyPage}
	}

	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space %s: %w", spaceId, err)
	}
	// A document may reference bundled types/relations not yet present in the
	// space (a fresh space has only a few installed); install them so the
	// created object's type and relations resolve (mirrors the import path).
	if ids := bundledIdsToInstall(createState.AllRelationKeys(), typeKeys); len(ids) > 0 {
		if _, _, err := a.creator.InstallBundledObjects(ctx, spc, ids); err != nil {
			return "", fmt.Errorf("install bundled objects: %w", err)
		}
	}

	id, _, err := a.creator.CreateSmartBlockFromStateInSpace(ctx, spc, typeKeys, createState)
	if err != nil {
		return "", fmt.Errorf("create object from state: %w", err)
	}
	return id, nil
}

func (a *objectCreateAdapter) TypeIdByKey(ctx context.Context, spaceId string, key domain.TypeKey) (string, error) {
	spc, err := a.spaces.Get(ctx, spaceId)
	if err != nil {
		return "", fmt.Errorf("get space %s: %w", spaceId, err)
	}
	id, err := spc.GetTypeIdByKey(ctx, key)
	if err != nil {
		return "", fmt.Errorf("derive type id for %s: %w", key, err)
	}
	return id, nil
}

// snapshotRootId finds the snapshot's root block: the block carrying the
// smartblock content (anyblockjson.Unmarshal always emits exactly one).
func snapshotRootId(snapshot *model.SmartBlockSnapshotBase) string {
	if snapshot == nil {
		return ""
	}
	for _, b := range snapshot.Blocks {
		if b.GetSmartblock() != nil {
			return b.Id
		}
	}
	return ""
}

// bundledIdsToInstall lists the bundled-object source ids for every bundled
// relation key and type key referenced by the state (the same set the import
// path installs; InstallBundledObjects skips already-installed ids).
func bundledIdsToInstall(relationKeys []domain.RelationKey, typeKeys []domain.TypeKey) []string {
	ids := make([]string, 0, len(relationKeys)+len(typeKeys))
	for _, key := range relationKeys {
		if bundle.HasRelation(key) {
			ids = append(ids, key.BundledURL())
		}
	}
	for _, key := range typeKeys {
		if bundle.HasObjectTypeByKey(key) {
			ids = append(ids, addr.BundledObjectTypeURLPrefix+string(key))
		}
	}
	return ids
}
