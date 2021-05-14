package restriction

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var (
	objRestrictAll = ObjectRestrictions{
		model.Restrictions_CreateBlock,
		model.Restrictions_Relation,
		model.Restrictions_Header,
		model.Restrictions_Delete,
	}
	objRestrictEdit = ObjectRestrictions{
		model.Restrictions_CreateBlock,
		model.Restrictions_Relation,
		model.Restrictions_Header,
	}

	objectRestrictionsByPbType = map[model.SmartBlockType]ObjectRestrictions{
		model.SmartBlockType_Breadcrumbs:    objRestrictEdit,
		model.SmartBlockType_ProfilePage:    {},
		model.SmartBlockType_AnytypeProfile: objRestrictAll,
		model.SmartBlockType_Page:           {},
		model.SmartBlockType_Home: {
			model.Restrictions_Header,
			model.Restrictions_Relation,
		},
		model.SmartBlockType_File:                objRestrictEdit,
		model.SmartBlockType_MarketplaceRelation: objRestrictAll,
		model.SmartBlockType_MarketplaceTemplate: objRestrictAll,
		model.SmartBlockType_MarketplaceType:     objRestrictAll,
		model.SmartBlockType_Archive:             objRestrictAll,
		model.SmartBlockType_Set:                 {model.Restrictions_CreateBlock},
		model.SmartBlockType_BundledRelation:     objRestrictAll,
		model.SmartBlockType_IndexedRelation:     objRestrictAll,
		model.SmartBlockType_BundledObjectType:   objRestrictAll,
		model.SmartBlockType_STObjectType:        objRestrictEdit,
		model.SmartBlockType_BundledTemplate:     objRestrictAll,
	}
)

type ObjectRestrictions []model.RestrictionsObjectRestriction

func (or ObjectRestrictions) Check(cr ...model.RestrictionsObjectRestriction) (err error) {
	for _, r := range cr {
		for _, er := range or {
			if er == r {
				return ErrRestricted
			}
		}
	}
	return
}

func (s *service) ObjectRestrictionsByObj(obj Object) (r ObjectRestrictions) {
	var ok bool
	if r, ok = objectRestrictionsByPbType[obj.Type()]; ok {
		return
	}
	log.Warnf("restrctions not found for object: id='%s' type='%v'", obj.Id(), obj.Type())
	return objRestrictAll
}
