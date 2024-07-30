package relationutils

import (
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ObjectType struct {
	*model.ObjectType
}

func (ot *ObjectType) BundledTypeDetails() *domain.Details {
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

	return domain.NewDetailsFromMap(map[domain.RelationKey]any{
		bundle.RelationKeyType:                 bundle.TypeKeyObjectType.BundledURL(),
		bundle.RelationKeyLayout:               float64(model.ObjectType_objectType),
		bundle.RelationKeyName:                 ot.Name,
		bundle.RelationKeyCreator:              addr.AnytypeProfileId,
		bundle.RelationKeyIconEmoji:            ot.IconEmoji,
		bundle.RelationKeyUniqueKey:            uk.Marshal(),
		bundle.RelationKeyRecommendedRelations: relationIds,
		bundle.RelationKeyRecommendedLayout:    float64(ot.Layout),
		bundle.RelationKeyDescription:          ot.Description,
		bundle.RelationKeyId:                   ot.Url,
		bundle.RelationKeyIsHidden:             ot.Hidden,
		bundle.RelationKeyIsArchived:           false,
		bundle.RelationKeyIsReadonly:           ot.Readonly,
		bundle.RelationKeySmartblockTypes:      sbTypes,
		bundle.RelationKeySpaceId:              addr.AnytypeMarketplaceWorkspace,
		bundle.RelationKeyOrigin:               int64(model.ObjectOrigin_builtin),
		bundle.RelationKeyRevision:             ot.Revision,
	})
}
