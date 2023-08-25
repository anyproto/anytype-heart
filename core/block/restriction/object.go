package restriction

import (
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	objRestrictAll = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_Relations,
		model.Restrictions_Details,
		model.Restrictions_Delete,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Duplicate,
	}
	objFileRestrictions = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_Relations,
		model.Restrictions_Details,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Duplicate,
	}
	objRestrictEdit = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
	}
	collectionRestrictions = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
	}
	sysTypesRestrictions = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Details,
		model.Restrictions_Delete,
	}
	sysRelationsRestrictions = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Delete,
		model.Restrictions_Relations,
		model.Restrictions_Details,
	}

	objectRestrictionsByLayout = map[model.ObjectTypeLayout]ObjectRestrictions{
		model.ObjectType_basic:      {},
		model.ObjectType_profile:    {},
		model.ObjectType_todo:       {},
		model.ObjectType_set:        collectionRestrictions,
		model.ObjectType_collection: collectionRestrictions,
		model.ObjectType_objectType: objRestrictEdit,
		model.ObjectType_relation:   objRestrictEdit,
		model.ObjectType_file:       objRestrictAll,
		model.ObjectType_dashboard: {
			model.Restrictions_Details,
			model.Restrictions_Relations,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
		},
		model.ObjectType_image: objRestrictAll,
		model.ObjectType_note:  {},
		model.ObjectType_space: {
			model.Restrictions_Template,
		},

		model.ObjectType_bookmark:       {},
		model.ObjectType_relationOption: objRestrictEdit,
		model.ObjectType_relationOptionsList: {
			model.Restrictions_Template,
		},
		model.ObjectType_database: {
			model.Restrictions_Template,
		},
	}

	objectRestrictionsBySBType = map[smartblock.SmartBlockType]ObjectRestrictions{
		smartblock.SmartBlockTypeProfilePage: {
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Delete,
			model.Restrictions_Duplicate,
		},
		smartblock.SmartBlockTypeAnytypeProfile: objRestrictAll,
		smartblock.SmartBlockTypeHome: {
			model.Restrictions_Details,
			model.Restrictions_Relations,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
		},
		smartblock.SmartBlockTypeWorkspace: {
			model.Restrictions_Blocks,
			model.Restrictions_Relations,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
		},
		smartblock.SmartBlockTypeFile:            objFileRestrictions,
		smartblock.SmartBlockTypeArchive:         objRestrictAll,
		smartblock.SmartBlockTypeBundledRelation: objRestrictAll,
		smartblock.SmartBlockTypeSubObject: {
			model.Restrictions_Blocks,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
		},
		smartblock.SmartBlockTypeBundledObjectType: objRestrictAll,
		smartblock.SmartBlockTypeBundledTemplate:   objRestrictAll,
		smartblock.SmartBlockTypeTemplate: {
			model.Restrictions_Template,
		},
		smartblock.SmartBlockTypeWidget: {
			model.Restrictions_Relations,
			model.Restrictions_Details,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
		},
		smartblock.SmartBlockTypeMissingObject: objRestrictAll,
		smartblock.SmartBlockTypeDate:          objRestrictAll,
		smartblock.SmartBlockTypeAccountOld: {
			model.Restrictions_Template,
		},
	}
)

type ObjectRestrictions []model.RestrictionsObjectRestriction

func (or ObjectRestrictions) Check(cr ...model.RestrictionsObjectRestriction) (err error) {
	for _, r := range cr {
		for _, er := range or {
			if er == r {
				return fmt.Errorf("%w: %s", ErrRestricted, r.String())
			}
		}
	}
	return
}

func (or ObjectRestrictions) Equal(or2 ObjectRestrictions) bool {
	if len(or) != len(or2) {
		return false
	}
	for _, r := range or {
		if or2.Check(r) == nil {
			return false
		}
	}
	return true
}

func (or ObjectRestrictions) Copy() ObjectRestrictions {
	obj := make(ObjectRestrictions, len(or))
	copy(obj, or)
	return obj
}

func (or ObjectRestrictions) ToPB() *types.Value {
	var ints = make([]int, len(or))
	for i, v := range or {
		ints[i] = int(v)
	}
	return pbtypes.IntList(ints...)
}

func (s *service) getObjectRestrictions(rh RestrictionHolder) (r ObjectRestrictions) {
	uk := rh.UniqueKey()
	if uk != nil {
		return GetRestrictionsForUniqueKey(rh.UniqueKey())
	}

	var ok bool
	if r, ok = objectRestrictionsBySBType[rh.Type()]; ok {
		return
	}

	if l, has := rh.Layout(); has {
		if r, ok = objectRestrictionsByLayout[l]; !ok {
			r = ObjectRestrictions{}
		}
	}

	if !errors.Is(r.Check(model.Restrictions_Template), ErrRestricted) {
		if ok, err := s.systemObjectService.HasObjectType(rh.ObjectTypeID()); err != nil || !ok {
			r = append(r, model.Restrictions_Template)
		}
	}

	return
}

func GetRestrictionsForUniqueKey(uk domain.UniqueKey) (r ObjectRestrictions) {
	switch uk.SmartblockType() {
	case smartblock.SmartBlockTypeObjectType:
		key := uk.InternalKey()
		if lo.Contains(bundle.SystemTypes, bundle.TypeKey(key)) {
			return sysTypesRestrictions
		}
	case smartblock.SmartBlockTypeRelation:
		key := uk.InternalKey()
		if lo.Contains(bundle.SystemRelations, bundle.RelationKey(key)) {
			return sysRelationsRestrictions
		}
	}
	return
}

func GetDataviewRestrictionsForUniqueKey(uk domain.UniqueKey) (r DataviewRestrictions) {
	// TODO What is happening here?
	r = dataviewRestrictionsBySBType[smartblock.SmartBlockTypeSubObject]
	switch uk.SmartblockType() {
	case smartblock.SmartBlockTypeObjectType:
		key := uk.InternalKey()
		if lo.Contains(bundle.InternalTypes, bundle.TypeKey(key)) {
			return append(r.Copy(), model.RestrictionsDataviewRestrictions{
				BlockId:      DataviewBlockId,
				Restrictions: []model.RestrictionsDataviewRestriction{model.Restrictions_DVCreateObject},
			})
		}
	case smartblock.SmartBlockTypeRelation:
		// should we handle this?
	}

	return
}
