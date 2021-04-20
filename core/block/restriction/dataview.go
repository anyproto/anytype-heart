package restriction

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const DataviewBlockId = "dataview"

var (
	dvRestrictMarketplace = DataviewRestrictions{
		model.RestrictionsDataviewRestrictions{
			BlockId: DataviewBlockId,
			Restrictions: []model.RestrictionsDataviewRestriction{
				model.Restrictions_CreateView,
				model.Restrictions_CreateRelation,
				model.Restrictions_EditObject,
			},
		},
	}
	dvRestrictNo = DataviewRestrictions{
		model.RestrictionsDataviewRestrictions{
			BlockId: DataviewBlockId,
		},
	}

	dataviewRestrictionsByPb = map[pb.SmartBlockType]DataviewRestrictions{
		pb.SmartBlockType_MarketplaceRelation: dvRestrictMarketplace,
		pb.SmartBlockType_MarketplaceTemplate: dvRestrictMarketplace,
		pb.SmartBlockType_MarketplaceType:     dvRestrictMarketplace,
		pb.SmartBlockType_Set:                 dvRestrictNo,
	}
)

type DataviewRestrictions []model.RestrictionsDataviewRestrictions

func (dr DataviewRestrictions) Check(dataviewId string, cr ...model.RestrictionsDataviewRestriction) (err error) {
	for _, d := range dr {
		if d.BlockId == dataviewId {
			for _, r := range cr {
				for _, er := range d.Restrictions {
					if er == r {
						return ErrRestricted
					}
				}
			}
		}
	}
	return
}

func (s *service) DataviewRestrictionsByObj(obj Object) DataviewRestrictions {
	if dr, ok := dataviewRestrictionsByPb[obj.Type()]; ok {
		return dr
	}
	return nil
}
