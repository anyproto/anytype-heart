package restriction

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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

	objectRestrictionsByPbType = map[model.SmartBlockType]ObjectRestrictions{
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
		model.SmartBlockType_File:            objRestrictAll,
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

func (s *service) ObjectRestrictionsByObj(obj Object) (r ObjectRestrictions) {
	var ok bool
	// todo: get rid of this
	if obj.Type() == model.SmartBlockType_ProfilePage && s.anytype.PredefinedBlocks().Profile != obj.Id() {
		if r, ok = objectRestrictionsByPbType[model.SmartBlockType_Page]; ok {
			return
		}
	}

	if r, ok = objectRestrictionsByPbType[obj.Type()]; ok {
		return
	}

	if l, has := obj.Layout(); has {
		if r, ok = objectRestrictionsByLayout[l]; ok {
			return
		}
	}
	log.Warnf("restrctions not found for object: id='%s' type='%v'", obj.Id(), obj.Type())
	return objRestrictAll
}
