package restriction

import (
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const DataviewBlockId = "dataview"

var (
	dvRestrictNo = DataviewRestrictions{
		model.RestrictionsDataviewRestrictions{
			BlockId: DataviewBlockId,
		},
	}
	dvRestrictAll = DataviewRestrictions{
		model.RestrictionsDataviewRestrictions{
			BlockId: DataviewBlockId,
			Restrictions: []model.RestrictionsDataviewRestriction{
				model.Restrictions_DVRelation,
				model.Restrictions_DVViews,
				model.Restrictions_DVRelation,
				model.Restrictions_DVCreateObject,
			},
		},
	}

	dataviewRestrictionsBySBType = map[smartblock.SmartBlockType]DataviewRestrictions{
		smartblock.SmartBlockTypePage: dvRestrictNo,
		smartblock.SmartBlockTypeDate: dvRestrictAll,
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

func (dr DataviewRestrictions) Copy() DataviewRestrictions {
	cp := make(DataviewRestrictions, len(dr))
	for i := range dr {
		rt := make([]model.RestrictionsDataviewRestriction, len(dr[i].Restrictions))
		copy(rt, dr[i].Restrictions)
		cp[i] = model.RestrictionsDataviewRestrictions{
			BlockId:      dr[i].BlockId,
			Restrictions: rt,
		}
	}
	return cp
}

func (dr DataviewRestrictions) Equal(dr2 DataviewRestrictions) bool {
	if len(dr) != len(dr2) {
		return false
	}
	var restrEqual = func(r1, r2 model.RestrictionsDataviewRestrictions) bool {
		if len(r1.Restrictions) != len(r2.Restrictions) {
			return false
		}
		for _, rr1 := range r1.Restrictions {
			var found bool
			for _, rr2 := range r2.Restrictions {
				if rr1 == rr2 {
					found = true
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	for _, drr := range dr {
		for _, drr2 := range dr2 {
			var found bool
			if drr.BlockId == drr2.BlockId {
				if !restrEqual(drr, drr2) {
					return false
				}
				found = true
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func getDataviewRestrictions(rh RestrictionHolder) DataviewRestrictions {
	if dr, ok := dataviewRestrictionsBySBType[rh.Type()]; ok {
		return dr
	}

	uk := rh.UniqueKey()
	if uk != nil {
		return getDataviewRestrictionsForUniqueKey(uk)
	}

	return nil
}

func getDataviewRestrictionsForUniqueKey(uk domain.UniqueKey) DataviewRestrictions {
	switch uk.SmartblockType() {
	case smartblock.SmartBlockTypeObjectType:
		key := uk.InternalKey()
		if lo.Contains(bundle.InternalTypes, domain.TypeKey(key)) {
			return DataviewRestrictions{
				model.RestrictionsDataviewRestrictions{
					BlockId:      DataviewBlockId,
					Restrictions: []model.RestrictionsDataviewRestriction{model.Restrictions_DVCreateObject},
				},
			}
		}
	case smartblock.SmartBlockTypeRelation:
		// should we handle this?
	}
	return nil
}
