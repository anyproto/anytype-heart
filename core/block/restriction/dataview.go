package restriction

import (
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

	dataviewRestrictionsByPb = map[model.SmartBlockType]DataviewRestrictions{
		model.SmartBlockType_MarketplaceRelation: dvRestrictMarketplace,
		model.SmartBlockType_MarketplaceTemplate: dvRestrictMarketplace,
		model.SmartBlockType_MarketplaceType:     dvRestrictMarketplace,
		model.SmartBlockType_Set:                 dvRestrictNo,
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
