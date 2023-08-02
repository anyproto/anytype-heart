package restriction

import (
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
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

	objectRestrictionsBySBType = map[model.SmartBlockType]ObjectRestrictions{
		model.SmartBlockType_ProfilePage: {
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Delete,
			model.Restrictions_Duplicate,
		},
		model.SmartBlockType_AnytypeProfile: objRestrictAll,
		model.SmartBlockType_Home: {
			model.Restrictions_Details,
			model.Restrictions_Relations,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
		},
		model.SmartBlockType_Workspace: {
			model.Restrictions_Blocks,
			model.Restrictions_Relations,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
		},
		model.SmartBlockType_File:            objFileRestrictions,
		model.SmartBlockType_Archive:         objRestrictAll,
		model.SmartBlockType_BundledRelation: objRestrictAll,
		model.SmartBlockType_SubObject: {
			model.Restrictions_Blocks,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
		},
		model.SmartBlockType_BundledObjectType: objRestrictAll,
		model.SmartBlockType_BundledTemplate:   objRestrictAll,
		model.SmartBlockType_Template: {
			model.Restrictions_Template,
		},
		model.SmartBlockType_Widget: {
			model.Restrictions_Relations,
			model.Restrictions_Details,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
		},
		model.SmartBlockType_MissingObject: objRestrictAll,
		model.SmartBlockType_Date:          objRestrictAll,
		model.SmartBlockType_AccountOld: {
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
		if _, err := s.store.GetObjectType(rh.ObjectType()); err != nil {
			r = append(r, model.Restrictions_Template)
		}
	}

	return
}

func GetRestrictionsForUniqueKey(uk uniquekey.UniqueKey) (r ObjectRestrictions) {
	switch uk.SmartblockType() {
	case model.SmartBlockType_STType:
		key := uk.(uniquekey.UniqueKeyInternal).InternalKey()
		if lo.Contains(bundle.SystemTypes, bundle.TypeKey(key)) {
			return sysTypesRestrictions
		}
	case model.SmartBlockType_STRelation:
		key := uk.(uniquekey.UniqueKeyInternal).InternalKey()
		if lo.Contains(bundle.SystemRelations, bundle.RelationKey(key)) {
			return sysRelationsRestrictions
		}
	}
	return
}

func GetDataviewRestrictionsForUniqueKey(uk uniquekey.UniqueKey) (r DataviewRestrictions) {
	r, _ = dataviewRestrictionsBySBType[model.SmartBlockType_SubObject]
	switch uk.SmartblockType() {
	case model.SmartBlockType_STType:
		key := uk.(uniquekey.UniqueKeyInternal).InternalKey()
		if lo.Contains(bundle.InternalTypes, bundle.TypeKey(key)) {
			return append(r.Copy(), model.RestrictionsDataviewRestrictions{
				BlockId:      DataviewBlockId,
				Restrictions: []model.RestrictionsDataviewRestriction{model.Restrictions_DVCreateObject},
			})
		}
	case model.SmartBlockType_STRelation:
		// should we handle this?
	}

	return
}
