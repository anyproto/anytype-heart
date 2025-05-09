package restriction

import (
	"fmt"
	"slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
		model.Restrictions_Publish,
	}
	objRestrictEditAndDuplicate = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Duplicate,
		model.Restrictions_Publish,
	}
	objRestrictEdit = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Publish,
	}
	objRestrictEditAndTemplate = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Publish,
	}
	sysTypesRestrictionsEdit = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Details,
		model.Restrictions_Delete,
		model.Restrictions_Publish,
	}
	sysTypesRestrictions = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Delete,
		model.Restrictions_Publish,
	}
	sysRelationsRestrictions = ObjectRestrictions{
		model.Restrictions_Blocks,
		model.Restrictions_LayoutChange,
		model.Restrictions_TypeChange,
		model.Restrictions_Template,
		model.Restrictions_Delete,
		model.Restrictions_Relations,
		model.Restrictions_Details,
		model.Restrictions_Publish,
	}

	objectRestrictionsByLayout = map[model.ObjectTypeLayout]ObjectRestrictions{
		model.ObjectType_basic:   {},
		model.ObjectType_profile: {},
		model.ObjectType_todo:    {},
		model.ObjectType_set:     objRestrictEdit,
		model.ObjectType_collection: {
			model.Restrictions_Blocks,
			model.Restrictions_LayoutChange,
			model.Restrictions_Publish,
		},
		model.ObjectType_objectType: objRestrictEditAndTemplate,
		model.ObjectType_relation:   objRestrictEditAndTemplate,
		model.ObjectType_file:       objRestrictEditAndDuplicate,
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
		model.ObjectType_relationOption: objRestrictEditAndTemplate,
		model.ObjectType_relationOptionsList: {
			model.Restrictions_Template,
		},
		model.ObjectType_participant: objRestrictAll,
		model.ObjectType_chat:        objRestrictEditAndDuplicate,
		model.ObjectType_chatDerived: objRestrictEditAndDuplicate,
		model.ObjectType_tag:         objRestrictEditAndTemplate,
	}

	objectRestrictionsBySBType = map[smartblock.SmartBlockType]ObjectRestrictions{
		smartblock.SmartBlockTypeIdentity: objRestrictAll,
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
			model.Restrictions_Publish,
		},
		smartblock.SmartBlockTypeWorkspace: {
			model.Restrictions_Blocks,
			model.Restrictions_Relations,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
			model.Restrictions_Publish,
		},
		smartblock.SmartBlockTypeFileObject:        objRestrictEditAndDuplicate,
		smartblock.SmartBlockTypeArchive:           objRestrictAll,
		smartblock.SmartBlockTypeBundledRelation:   objRestrictAll,
		smartblock.SmartBlockTypeSubObject:         objRestrictEditAndTemplate,
		smartblock.SmartBlockTypeObjectType:        objRestrictEditAndTemplate,
		smartblock.SmartBlockTypeRelation:          objRestrictEditAndTemplate,
		smartblock.SmartBlockTypeBundledObjectType: objRestrictAll,
		smartblock.SmartBlockTypeBundledTemplate:   objRestrictAll,
		smartblock.SmartBlockTypeTemplate: {
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Publish,
		},
		smartblock.SmartBlockTypeWidget: {
			model.Restrictions_Relations,
			model.Restrictions_Details,
			model.Restrictions_Delete,
			model.Restrictions_LayoutChange,
			model.Restrictions_TypeChange,
			model.Restrictions_Template,
			model.Restrictions_Duplicate,
			model.Restrictions_Publish,
		},
		smartblock.SmartBlockTypeMissingObject: objRestrictAll,
		smartblock.SmartBlockTypeDate:          objRestrictAll,
		smartblock.SmartBlockTypeAccountOld: {
			model.Restrictions_Template,
		},
		smartblock.SmartBlockTypeParticipant:       objRestrictAll,
		smartblock.SmartBlockTypeChatObject:        objRestrictEditAndDuplicate,
		smartblock.SmartBlockTypeChatDerivedObject: objRestrictEditAndDuplicate,
	}
)

var (
	editableSystemTypes = []domain.TypeKey{
		bundle.TypeKeyPage, bundle.TypeKeyBookmark, // pages
		bundle.TypeKeySet, bundle.TypeKeyCollection, // lists
		bundle.TypeKeyFile, bundle.TypeKeyAudio, bundle.TypeKeyVideo, bundle.TypeKeyImage, // files
	}
)

func GetRestrictionsBySBType(sbType smartblock.SmartBlockType) []int {
	restrictions := objectRestrictionsBySBType[sbType]
	result := make([]int, len(restrictions))
	for i, restriction := range restrictions {
		result[i] = int(restriction)
	}
	return result
}

type ObjectRestrictions []model.RestrictionsObjectRestriction

func NewObjectRestrictionsFromValue(v domain.Value) ObjectRestrictions {
	raw := v.Int64List()
	restrictions := make(ObjectRestrictions, len(raw))
	for i, restriction := range raw {
		// nolint:gosec
		restrictions[i] = model.RestrictionsObjectRestriction(restriction)
	}
	return restrictions
}

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

func (or ObjectRestrictions) ToValue() domain.Value {
	var ints = make([]int64, len(or))
	for i, v := range or {
		ints[i] = int64(v)
	}
	return domain.Int64List(ints)
}

func getObjectRestrictions(rh RestrictionHolder) (r ObjectRestrictions) {
	uk := rh.UniqueKey()
	if uk != nil && uk.InternalKey() != "" {
		return getRestrictionsForUniqueKey(uk)
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
	return
}

func getRestrictionsForUniqueKey(uk domain.UniqueKey) (r ObjectRestrictions) {
	r = objectRestrictionsBySBType[uk.SmartblockType()]
	switch uk.SmartblockType() {
	case smartblock.SmartBlockTypeObjectType:
		key := uk.InternalKey()
		if slices.Contains(bundle.SystemTypes, domain.TypeKey(key)) {
			if slices.Contains(editableSystemTypes, domain.TypeKey(key)) {
				r = sysTypesRestrictions
			} else {
				r = sysTypesRestrictionsEdit
			}
		}
		if t, _ := bundle.GetType(domain.TypeKey(key)); t != nil && t.RestrictObjectCreation {
			r = append(r, model.Restrictions_CreateObjectOfThisType)
		}
		return r
	case smartblock.SmartBlockTypeRelation:
		key := uk.InternalKey()
		if slices.Contains(bundle.SystemRelations, domain.RelationKey(key)) {
			r = sysRelationsRestrictions
		}
	}
	// we assume that all sb types exist in objectRestrictionsBySBType
	return r
}
