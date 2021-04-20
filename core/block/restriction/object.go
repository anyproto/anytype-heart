package restriction

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
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

	objectRestrictionsByPbType = map[pb.SmartBlockType]ObjectRestrictions{
		pb.SmartBlockType_Breadcrumbs:    objRestrictEdit,
		pb.SmartBlockType_ProfilePage:    {},
		pb.SmartBlockType_AnytypeProfile: objRestrictAll,
		pb.SmartBlockType_Page:           {},
		pb.SmartBlockType_Home: {
			model.Restrictions_Header,
			model.Restrictions_Relation,
		},
		pb.SmartBlockType_File:                objRestrictEdit,
		pb.SmartBlockType_MarketplaceRelation: objRestrictAll,
		pb.SmartBlockType_MarketplaceTemplate: objRestrictAll,
		pb.SmartBlockType_MarketplaceType:     objRestrictAll,
		pb.SmartBlockType_Archive:             objRestrictAll,
		pb.SmartBlockType_Set:                 {model.Restrictions_CreateBlock},
		pb.SmartBlockType_BundledRelation:     objRestrictAll,
		pb.SmartBlockType_IndexedRelation:     objRestrictAll,
		pb.SmartBlockType_BundledObjectType:   objRestrictAll,
		pb.SmartBlockType_ObjectType:          objRestrictEdit,
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
