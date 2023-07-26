package relationutils

import (
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type ObjectType struct {
	*model.ObjectType
}

func (ot *ObjectType) BundledTypeDetails() *types.Struct {
	var (
		relationKeys []string
	)

	for _, rl := range ot.RelationLinks {
		relationKeys = append(relationKeys, addr.BundledRelationURLPrefix+rl.Key)
	}

	var sbTypes = make([]int, 0, len(ot.Types))
	for _, t := range ot.Types {
		sbTypes = append(sbTypes, int(t))
	}

	uk, err := uniquekey.NewUniqueKey(model.SmartBlockType_STType, ot.Key)
	if err != nil {
		return nil
	}

	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyType.String():                 pbtypes.String(bundle.TypeKeyObjectType.URL()),
		bundle.RelationKeyLayout.String():               pbtypes.Float64(float64(model.ObjectType_objectType)),
		bundle.RelationKeyName.String():                 pbtypes.String(ot.Name),
		bundle.RelationKeyCreator.String():              pbtypes.String(addr.AnytypeProfileId),
		bundle.RelationKeyIconEmoji.String():            pbtypes.String(ot.IconEmoji),
		bundle.RelationKeyUniqueKey.String():            pbtypes.String(uk.String()),
		bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList(relationKeys),
		bundle.RelationKeyRecommendedLayout.String():    pbtypes.Float64(float64(ot.Layout)),
		bundle.RelationKeyDescription.String():          pbtypes.String(ot.Description),
		bundle.RelationKeyId.String():                   pbtypes.String(ot.Url),
		bundle.RelationKeyIsHidden.String():             pbtypes.Bool(ot.Hidden),
		bundle.RelationKeyIsArchived.String():           pbtypes.Bool(false),
		bundle.RelationKeyIsReadonly.String():           pbtypes.Bool(ot.Readonly),
		bundle.RelationKeySmartblockTypes.String():      pbtypes.IntList(sbTypes...),
		bundle.RelationKeyWorkspaceId.String():          pbtypes.String(addr.AnytypeMarketplaceWorkspace),
		bundle.RelationKeySpaceId.String():              pbtypes.String(addr.AnytypeMarketplaceWorkspace),
	}}
}

// deprecated
func MigrateObjectTypeIds(ids []string) (normalized []string, idsToMigrate []string) {
	// todo: remove this and all dependencies on it

	hasIdsToMigrate := false
	for i := range ids {
		_, err := bundle.TypeKeyFromUrl(ids[i])
		if err == nil {
			hasIdsToMigrate = true
			break
		}
	}
	if !hasIdsToMigrate {
		return ids, nil
	}

	// in-place migration for bundled object types moved into workspace
	normalized = make([]string, len(ids))
	idsToMigrate = make([]string, 0, len(ids))
	var migrated bool
	for i := range ids {
		normalized[i], migrated = MigrateObjectTypeId(ids[i])
		if migrated {
			idsToMigrate = append(idsToMigrate, ids[i])
		}
	}

	return normalized, idsToMigrate
}

// deprecated
func MigrateObjectTypeId(id string) (normalized string, isMigrated bool) {
	// todo: remove this and all dependencies on it
	t, err := bundle.TypeKeyFromUrl(id)
	if err == nil {
		return addr.ObjectTypeKeyToIdPrefix + t.String(), true
	}
	return id, false
}
