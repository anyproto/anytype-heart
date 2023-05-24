package restriction

import (
	"fmt"
	"github.com/samber/lo"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
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
		model.ObjectType_space: {},

		model.ObjectType_bookmark:       {},
		model.ObjectType_relationOption: objRestrictEdit,
	}

	objectRestrictionsBySBType = map[model.SmartBlockType]ObjectRestrictions{
		model.SmartBlockType_ProfilePage:    {model.Restrictions_LayoutChange, model.Restrictions_TypeChange, model.Restrictions_Delete},
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
		model.SmartBlockType_Template:          {},
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

func (s *service) getObjectRestrictions(rh RestrictionHolder) (r ObjectRestrictions) {

	layout, hasLayout := rh.Layout()
	if hasLayout {
		switch layout {
		case model.ObjectType_objectType:
			return s.getObjectRestrictionsForObjectType(rh.Id())
		case model.ObjectType_relation:
			return s.getObjectRestrictionsForRelation(rh.Id())
		}
	}

	var ok bool
	if r, ok = objectRestrictionsBySBType[rh.Type()]; ok {
		return
	}

	if l, has := rh.Layout(); has {
		if r, ok = objectRestrictionsByLayout[l]; ok {
			return
		}
	}
	l, has := rh.Layout()
	log.Warnf("restrctions not found for object: id='%s' type='%v' layout='%v'(%v); fallback to empty", rh.Id(), rh.Type(), l, has)
	return ObjectRestrictions{}
}

func (s *service) getObjectRestrictionsForObjectType(id string) (r ObjectRestrictions) {
	r, _ = objectRestrictionsBySBType[model.SmartBlockType_SubObject]
	if strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) {
		return objRestrictAll
	}
	if !lo.Contains(bundle.SystemTypes, bundle.TypeKey(strings.TrimPrefix(id, addr.ObjectTypeKeyToIdPrefix))) {
		return
	}
	return sysTypesRestrictions
}

func (s *service) getObjectRestrictionsForRelation(id string) (r ObjectRestrictions) {
	r, _ = objectRestrictionsBySBType[model.SmartBlockType_SubObject]
	if strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return objRestrictAll
	}
	if !lo.Contains(bundle.SystemRelations, bundle.RelationKey(strings.TrimPrefix(id, addr.RelationKeyToIdPrefix))) {
		return
	}
	return sysRelationsRestrictions
}
