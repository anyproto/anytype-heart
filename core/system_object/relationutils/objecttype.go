package relationutils

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, ot.Key)
	if err != nil {
		return nil
	}

	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyType.String():                 pbtypes.String(bundle.TypeKeyObjectType.BundledURL()),
		bundle.RelationKeyLayout.String():               pbtypes.Float64(float64(model.ObjectType_objectType)),
		bundle.RelationKeyName.String():                 pbtypes.String(ot.Name),
		bundle.RelationKeyCreator.String():              pbtypes.String(addr.AnytypeProfileId),
		bundle.RelationKeyIconEmoji.String():            pbtypes.String(ot.IconEmoji),
		bundle.RelationKeyUniqueKey.String():            pbtypes.String(uk.Marshal()),
		bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList(relationKeys),
		bundle.RelationKeyRecommendedLayout.String():    pbtypes.Float64(float64(ot.Layout)),
		bundle.RelationKeyDescription.String():          pbtypes.String(ot.Description),
		bundle.RelationKeyId.String():                   pbtypes.String(ot.Url),
		bundle.RelationKeyIsHidden.String():             pbtypes.Bool(ot.Hidden),
		bundle.RelationKeyIsArchived.String():           pbtypes.Bool(false),
		bundle.RelationKeyIsReadonly.String():           pbtypes.Bool(ot.Readonly),
		bundle.RelationKeySmartblockTypes.String():      pbtypes.IntList(sbTypes...),
		bundle.RelationKeySpaceId.String():              pbtypes.String(addr.AnytypeMarketplaceWorkspace),
	}}
}
