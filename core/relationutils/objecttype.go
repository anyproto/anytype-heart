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

	sbTypes := make([]float64, 0, len(ot.Types))
	for _, t := range ot.Types {
		sbTypes = append(sbTypes, float64(t))
	}

	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, ot.Key)
	if err != nil {
		return nil
	}

	det := domain.NewDetails()
	det.SetString(bundle.RelationKeyType, bundle.TypeKeyObjectType.BundledURL())
	det.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_objectType))
	det.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))
	det.SetString(bundle.RelationKeyName, ot.Name)
	det.SetString(bundle.RelationKeyCreator, addr.AnytypeProfileId)
	det.SetString(bundle.RelationKeyUniqueKey, uk.Marshal())
	det.SetStringList(bundle.RelationKeyRecommendedRelations, relationIds)
	det.SetInt64(bundle.RelationKeyRecommendedLayout, int64(ot.Layout))
	det.SetString(bundle.RelationKeyDescription, ot.Description)
	det.SetString(bundle.RelationKeyId, ot.Url)
	det.SetBool(bundle.RelationKeyIsHidden, ot.Hidden)
	det.SetBool(bundle.RelationKeyIsArchived, false)
	det.SetBool(bundle.RelationKeyIsReadonly, ot.Readonly)
	det.SetFloat64List(bundle.RelationKeySmartblockTypes, sbTypes)
	det.SetString(bundle.RelationKeySpaceId, addr.AnytypeMarketplaceWorkspace)
	det.SetInt64(bundle.RelationKeyOrigin, int64(model.ObjectOrigin_builtin))
	det.SetInt64(bundle.RelationKeyRevision, ot.Revision)
	det.SetInt64(bundle.RelationKeyIconOption, ot.IconColor)
	det.SetString(bundle.RelationKeyIconName, ot.IconName)
	det.SetString(bundle.RelationKeySingleName, ot.SingleName)
	return det
}
