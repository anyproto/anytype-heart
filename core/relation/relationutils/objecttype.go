package relationutils

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"strings"
)

type ObjectType struct {
	*model.ObjectType
}

func (ot *ObjectType) ToStruct() *types.Struct {
	var (
		relationKeys []string
		prefix       string
	)

	if strings.HasPrefix(ot.Url, addr.BundledObjectTypeURLPrefix) {
		prefix = addr.BundledObjectTypeURLPrefix
	} else {
		prefix = addr.ObjectTypeKeyToIdPrefix
	}

	for i := range ot.RelationLinks {
		relationKeys = append(relationKeys, prefix+ot.RelationLinks[i].Key)
	}

	var sbTypes = make([]int, 0, len(ot.Types))
	for _, t := range ot.Types {
		sbTypes = append(sbTypes, int(t))
	}
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyType.String():                 pbtypes.String(bundle.TypeKeyObjectType.URL()),
		bundle.RelationKeyLayout.String():               pbtypes.Float64(float64(model.ObjectType_objectType)),
		bundle.RelationKeyName.String():                 pbtypes.String(ot.Name),
		bundle.RelationKeyCreator.String():              pbtypes.String(addr.AnytypeProfileId),
		bundle.RelationKeyIconEmoji.String():            pbtypes.String(ot.IconEmoji),
		bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList(relationKeys),
		bundle.RelationKeyRecommendedLayout.String():    pbtypes.Float64(float64(ot.Layout)),
		bundle.RelationKeyDescription.String():          pbtypes.String(ot.Description),
		bundle.RelationKeyId.String():                   pbtypes.String(ot.Url),
		bundle.RelationKeyIsHidden.String():             pbtypes.Bool(ot.Hidden),
		bundle.RelationKeyIsArchived.String():           pbtypes.Bool(false),
		bundle.RelationKeyIsReadonly.String():           pbtypes.Bool(ot.Readonly),
		bundle.RelationKeySmartblockTypes.String():      pbtypes.IntList(sbTypes...),
		bundle.RelationKeyWorkspaceId.String():          pbtypes.String(addr.AnytypeMarketplaceWorkspace),
	}}
}

// MigrateObjectTypeIds returns 2 slices:
// normalized – contains the slice of normalized ids. it contains original slice if there is nothing to normalize(no bundled object type IDs exist in the object)
// idsToMigrate - contains the slice of ids that are converted during the first step, nil if no ids were converted
func MigrateObjectTypeIds(ids []string) (normalized []string, idsToMigrate []string) {
	// shortcut if there is nothing to migrate
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
	for i := range ids {
		t, err := bundle.TypeKeyFromUrl(ids[i])
		if err == nil {
			idsToMigrate = append(idsToMigrate, ids[i])
			normalized[i] = addr.ObjectTypeKeyToIdPrefix + t.String()
		} else {
			normalized[i] = ids[i]
		}
	}

	return normalized, idsToMigrate
}

func MigrateObjectTypeId(id string) (normalized string, isMigrated bool) {
	t, err := bundle.TypeKeyFromUrl(id)
	if err == nil {
		return addr.ObjectTypeKeyToIdPrefix + t.String(), true
	}
	return id, false
}
