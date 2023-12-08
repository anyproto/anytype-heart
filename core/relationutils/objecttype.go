package relationutils

import (
	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

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
		relationIds []string
	)
	for _, rl := range ot.RelationLinks {
		relationIds = append(relationIds, domain.RelationKey(rl.Key).BundledURL())
	}
	if !slices.Contains(relationIds, bundle.RelationKeyDescription.BundledURL()) {
		relationIds = append(relationIds, bundle.RelationKeyDescription.BundledURL())
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
		bundle.RelationKeyRecommendedRelations.String(): pbtypes.StringList(relationIds),
		bundle.RelationKeyRecommendedLayout.String():    pbtypes.Float64(float64(ot.Layout)),
		bundle.RelationKeyDescription.String():          pbtypes.String(ot.Description),
		bundle.RelationKeyId.String():                   pbtypes.String(ot.Url),
		bundle.RelationKeyIsHidden.String():             pbtypes.Bool(ot.Hidden),
		bundle.RelationKeyIsArchived.String():           pbtypes.Bool(false),
		bundle.RelationKeyIsReadonly.String():           pbtypes.Bool(ot.Readonly),
		bundle.RelationKeySmartblockTypes.String():      pbtypes.IntList(sbTypes...),
		bundle.RelationKeySpaceId.String():              pbtypes.String(addr.AnytypeMarketplaceWorkspace),
		bundle.RelationKeyOrigin.String():               pbtypes.Int64(int64(model.ObjectOrigin_builtin)),
		bundle.RelationKeyRevision.String():             pbtypes.Int64(ot.Revision),
	}}
}
