package restriction

import (
	"fmt"
	"maps"
	"slices"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	objRestrictAll = ObjectRestrictions{
		model.Restrictions_Blocks:       {},
		model.Restrictions_Relations:    {},
		model.Restrictions_Details:      {},
		model.Restrictions_Delete:       {},
		model.Restrictions_LayoutChange: {},
		model.Restrictions_TypeChange:   {},
		model.Restrictions_Template:     {},
		model.Restrictions_Duplicate:    {},
		model.Restrictions_Publish:      {},
	}
	objRestrictEdit = ObjectRestrictions{
		model.Restrictions_Blocks:       {},
		model.Restrictions_LayoutChange: {},
		model.Restrictions_TypeChange:   {},
		model.Restrictions_Publish:      {},
	}
	objRestrictEditAndTemplate  = objRestrictEdit.Copy().Add(model.Restrictions_Template)
	objRestrictEditAndDuplicate = objRestrictEditAndTemplate.Copy().Add(model.Restrictions_Duplicate)

	objectRestrictionsByLayout = map[model.ObjectTypeLayout]ObjectRestrictions{
		model.ObjectType_basic:      {},
		model.ObjectType_profile:    {},
		model.ObjectType_todo:       {},
		model.ObjectType_note:       {},
		model.ObjectType_bookmark:   {},
		model.ObjectType_set:        objRestrictEdit,
		model.ObjectType_collection: objRestrictEdit.Copy().Remove(model.Restrictions_TypeChange),
		model.ObjectType_relationOptionsList: {
			model.Restrictions_Template: {},
		},
		model.ObjectType_space: {
			model.Restrictions_Template: {},
		},
	}

	objectRestrictionsBySBType = map[smartblock.SmartBlockType]ObjectRestrictions{
		smartblock.SmartBlockTypeIdentity:           objRestrictAll,
		smartblock.SmartBlockTypeAnytypeProfile:     objRestrictAll,
		smartblock.SmartBlockTypeArchive:            objRestrictAll,
		smartblock.SmartBlockTypeBundledRelation:    objRestrictAll,
		smartblock.SmartBlockTypeBundledObjectType:  objRestrictAll,
		smartblock.SmartBlockTypeBundledTemplate:    objRestrictAll,
		smartblock.SmartBlockTypeMissingObject:      objRestrictAll,
		smartblock.SmartBlockTypeDate:               objRestrictAll,
		smartblock.SmartBlockTypeParticipant:        objRestrictAll,
		smartblock.SmartBlockTypeAccountOld:         objRestrictAll, // new value
		smartblock.SmartBlockTypeAccountObject:      objRestrictAll, // new
		smartblock.SmartBlockTypeSpaceView:          objRestrictAll, // new
		smartblock.SmartBlockTypeNotificationObject: objRestrictAll, // new
		smartblock.SmartBlockTypeDevicesObject:      objRestrictAll, // new
		smartblock.SmartBlockTypeChatDerivedObject:  objRestrictEditAndDuplicate,
		smartblock.SmartBlockTypeChatObject:         objRestrictEditAndDuplicate,
		smartblock.SmartBlockTypeFileObject:         objRestrictEditAndDuplicate,
		smartblock.SmartBlockTypeSubObject:          objRestrictEditAndTemplate,
		smartblock.SmartBlockTypeObjectType:         objRestrictEditAndTemplate,
		smartblock.SmartBlockTypeRelation:           objRestrictEditAndTemplate,
		smartblock.SmartBlockTypeHome:               objRestrictAll.Copy().Remove(model.Restrictions_Blocks),
		smartblock.SmartBlockTypeWidget:             objRestrictAll.Copy().Remove(model.Restrictions_Blocks),
		smartblock.SmartBlockTypeWorkspace:          objRestrictAll.Copy().Remove(model.Restrictions_Details),
		smartblock.SmartBlockTypeProfilePage: {
			model.Restrictions_LayoutChange: {},
			model.Restrictions_TypeChange:   {},
			model.Restrictions_Delete:       {},
			model.Restrictions_Duplicate:    {},
		},
		smartblock.SmartBlockTypeTemplate: {
			model.Restrictions_TypeChange: {},
			model.Restrictions_Template:   {},
			model.Restrictions_Publish:    {},
		},
	}
)

var editableSystemTypes = map[domain.TypeKey]struct{}{
	bundle.TypeKeyPage: {}, bundle.TypeKeyBookmark: {}, // pages
	bundle.TypeKeySet: {}, bundle.TypeKeyCollection: {}, // lists
	bundle.TypeKeyFile: {}, bundle.TypeKeyAudio: {}, bundle.TypeKeyVideo: {}, bundle.TypeKeyImage: {}, // files
}

func GetRestrictionsBySBType(sbType smartblock.SmartBlockType) domain.Value {
	restrictions := objectRestrictionsBySBType[sbType]
	return restrictions.ToValue()
}

type ObjectRestrictions map[model.RestrictionsObjectRestriction]struct{}

func NewObjectRestrictionsFromValue(v domain.Value) ObjectRestrictions {
	raw := v.Int64List()
	restrictions := make(ObjectRestrictions, len(raw))
	for _, restriction := range raw {
		// nolint:gosec
		restrictions[model.RestrictionsObjectRestriction(restriction)] = struct{}{}
	}
	return restrictions
}

func (or ObjectRestrictions) Check(cr ...model.RestrictionsObjectRestriction) (err error) {
	for _, r := range cr {
		if _, found := or[r]; found {
			return fmt.Errorf("%w: %s", ErrRestricted, r.String())
		}
	}
	return
}

func (or ObjectRestrictions) Equal(or2 ObjectRestrictions) bool {
	if len(or) != len(or2) {
		return false
	}
	for r := range or {
		if or2.Check(r) == nil {
			return false
		}
	}
	return true
}

func (or ObjectRestrictions) Copy() ObjectRestrictions {
	obj := make(ObjectRestrictions, len(or))
	maps.Copy(obj, or)
	return obj
}

func (or ObjectRestrictions) ToValue() domain.Value {
	var ints = make([]int64, len(or))
	var i int
	for v := range or {
		ints[i] = int64(v)
		i++
	}
	return domain.Int64List(ints)
}

func (or ObjectRestrictions) ToProto() []model.RestrictionsObjectRestriction {
	return lo.Keys(or)
}

func (or ObjectRestrictions) Add(restrictions ...model.RestrictionsObjectRestriction) ObjectRestrictions {
	for _, r := range restrictions {
		or[r] = struct{}{}
	}
	return or
}

func (or ObjectRestrictions) Remove(restrictions ...model.RestrictionsObjectRestriction) ObjectRestrictions {
	for _, r := range restrictions {
		delete(or, r)
	}
	return or
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
	r = objectRestrictionsBySBType[uk.SmartblockType()].Copy()
	switch uk.SmartblockType() {
	case smartblock.SmartBlockTypeObjectType:
		key := uk.InternalKey()
		if slices.Contains(bundle.SystemTypes, domain.TypeKey(key)) {
			r.Add(model.Restrictions_Delete)
			if _, isEditable := editableSystemTypes[domain.TypeKey(key)]; !isEditable {
				r.Add(model.Restrictions_Details)
			}
		}
		if t, _ := bundle.GetType(domain.TypeKey(key)); t != nil && t.RestrictObjectCreation {
			r.Add(model.Restrictions_CreateObjectOfThisType)
		}
		return r
	case smartblock.SmartBlockTypeRelation:
		key := uk.InternalKey()
		if slices.Contains(bundle.SystemRelations, domain.RelationKey(key)) {
			r = objRestrictAll.Copy().Remove(model.Restrictions_Duplicate)
		}
	}
	// we assume that all sb types exist in objectRestrictionsBySBType
	return r
}
