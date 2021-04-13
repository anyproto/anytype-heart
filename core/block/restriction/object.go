package restriction

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var (
	objRestrictAll = ObjectRestrictions{
		model.ObjectRestriction_CreateBlock,
		model.ObjectRestriction_Relation,
		model.ObjectRestriction_Header,
		model.ObjectRestriction_Delete,
	}
	objRestrictEdit = ObjectRestrictions{
		model.ObjectRestriction_CreateBlock,
		model.ObjectRestriction_Relation,
		model.ObjectRestriction_Header,
	}

	objectRestrictionsByPbType = map[pb.SmartBlockType]ObjectRestrictions{
		pb.SmartBlockType_Breadcrumbs: objRestrictEdit,
		pb.SmartBlockType_ProfilePage: {},
		pb.SmartBlockType_Home: {
			model.ObjectRestriction_Header,
			model.ObjectRestriction_Relation,
		},
		pb.SmartBlockType_File:                objRestrictEdit,
		pb.SmartBlockType_MarketplaceRelation: objRestrictAll,
		pb.SmartBlockType_MarketplaceTemplate: objRestrictAll,
		pb.SmartBlockType_MarketplaceType:     objRestrictAll,
		pb.SmartBlockType_Archive:             objRestrictAll,
		pb.SmartBlockType_Set:                 {model.ObjectRestriction_CreateBlock},
	}

	objectRestrictionsBySbType = map[smartblock.SmartBlockType]ObjectRestrictions{
		smartblock.SmartBlockTypeBundledRelation:   objRestrictAll,
		smartblock.SmartBlockTypeIndexedRelation:   objRestrictAll,
		smartblock.SmartBlockTypeBundledObjectType: objRestrictAll,
		smartblock.SmartBlockTypeObjectType:        objRestrictEdit,
	}
)

type ObjectRestrictions []model.ObjectRestriction

func (or ObjectRestrictions) Check(cr ...model.ObjectRestriction) (err error) {
	for _, r := range cr {
		for _, er := range or {
			if er == r {
				return ErrRestricted
			}
		}
	}
	return
}

func (s *service) ObjectRestrictionsById(obj Object) (r ObjectRestrictions) {
	var ok bool
	if r, ok = objectRestrictionsByPbType[obj.Type()]; ok {
		return
	}
	smType, err := smartblock.SmartBlockTypeFromID(obj.Id())
	if err == nil {
		if r, ok = objectRestrictionsBySbType[smType]; ok {
			return
		}
	}
	log.Warnf("restrctions not found for object: id='%s' type='%v'", obj.Id(), obj.Type())
	return objRestrictAll
}
