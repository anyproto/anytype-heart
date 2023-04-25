package restriction

import (
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const DataviewBlockId = "dataview"

var (
	dvRestrictNo = DataviewRestrictions{
		model.RestrictionsDataviewRestrictions{
			BlockId: DataviewBlockId,
		},
	}

	dataviewRestrictionsByPb = map[model.SmartBlockType]DataviewRestrictions{
		model.SmartBlockType_Page: dvRestrictNo,
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

func (s *service) getDataviewRestrictions(rh RestrictionHolder) DataviewRestrictions {
	layout, hasLayout := rh.Layout()
	if hasLayout && layout == model.ObjectType_objectType {

	}

	if dr, ok := dataviewRestrictionsByPb[rh.Type()]; ok {
		return dr
	}
	return nil
}

func (s *service) getDataviewRestrictionsForObjectType(id string) (r DataviewRestrictions) {
	r, _ = dataviewRestrictionsByPb[model.SmartBlockType_SubObject]
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return
	}
	var internal bool
	for _, st := range bundle.InternalTypes {
		if st.URL() == id {
			internal = true
			break
		}
	}
	if !internal {
		return
	}
	return append(r.Copy(), model.RestrictionsDataviewRestrictions{
		BlockId:      DataviewBlockId,
		Restrictions: []model.RestrictionsDataviewRestriction{model.Restrictions_DVCreateObject},
	})
}
