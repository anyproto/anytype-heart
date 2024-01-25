package state

import (
	"strings"

	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/util/slice"
)

type ChangeApplier interface {
	ApplyChanges(ch *pb.ChangeContent)
}

type ChangeGetter interface {
	Diff() (ch []*pb.ChangeContent)
}

type ObjectType interface {
	ChangeApplier
	ChangeGetter
	ObjectTypeKeys() []domain.TypeKey
	ObjectTypeKey() domain.TypeKey
	SetObjectTypeKeys(objectTypeKeys []domain.TypeKey)
	SetParent(parent ObjectType)
	SetObjectTypeKey(objectTypeKey domain.TypeKey)
	SetNoObjectType(noObjectType bool)
	GetObjectTypeHistory() *undo.ObjectType
	SetParentObjectType()
}

type ObjectTypes struct {
	objectTypeKeys []domain.TypeKey
	parent         ObjectType
	noObjectType   bool
}

func NewObjectTypes(objectTypeKeys []domain.TypeKey, parent ObjectType) ObjectType {
	return &ObjectTypes{objectTypeKeys: objectTypeKeys, parent: parent}
}

func (o *ObjectTypes) GetObjectTypeHistory() *undo.ObjectType {
	var ot *undo.ObjectType
	if o.parent != nil && o.objectTypeKeys != nil {
		prev := o.parent.ObjectTypeKeys()
		if !slice.UnsortedEqual(prev, o.objectTypeKeys) {
			ot = &undo.ObjectType{Before: prev, After: o.objectTypeKeys}
			o.parent.SetObjectTypeKeys(o.objectTypeKeys)
		}
	}
	return ot
}

func (o *ObjectTypes) SetParentObjectType() {
	if len(o.objectTypeKeys) > 0 {
		o.parent.SetObjectTypeKeys(o.ObjectTypeKeys())
	}
}

func (o *ObjectTypes) SetObjectTypeKey(objectTypeKey domain.TypeKey) {
	o.SetObjectTypeKeys([]domain.TypeKey{objectTypeKey})
}

func (o *ObjectTypes) ObjectTypeKey() domain.TypeKey {
	objTypes := o.ObjectTypeKeys()
	if len(objTypes) == 0 && !o.noObjectType {
		return ""
	}

	if len(objTypes) > 0 {
		return objTypes[0]
	}
	return ""
}

func (o *ObjectTypes) ObjectTypeKeys() []domain.TypeKey {
	if o.objectTypeKeys == nil && o.parent != nil {
		return o.parent.ObjectTypeKeys()
	}
	return o.objectTypeKeys
}

func (o *ObjectTypes) SetObjectTypeKeys(objectTypeKeys []domain.TypeKey) {
	o.objectTypeKeys = objectTypeKeys
}

func (o *ObjectTypes) SetNoObjectType(noObjectType bool) {
	o.noObjectType = noObjectType
}

func (o *ObjectTypes) SetParent(parent ObjectType) {
	o.parent = parent
}

func (o *ObjectTypes) Diff() (ch []*pb.ChangeContent) {
	if o.objectTypeKeys == nil {
		return nil
	}
	var prev []domain.TypeKey
	if o.parent != nil {
		prev = o.parent.ObjectTypeKeys()
	}

	var prevMap = make(map[domain.TypeKey]struct{}, len(prev))
	var curMap = make(map[domain.TypeKey]struct{}, len(o.ObjectTypeKeys()))

	for _, v := range o.ObjectTypeKeys() {
		curMap[v] = struct{}{}
		_, ok := prevMap[v]
		if !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfObjectTypeAdd{
					ObjectTypeAdd: &pb.ChangeObjectTypeAdd{Url: v.URL()},
				},
			})
		}
	}
	for _, v := range prev {
		_, ok := curMap[v]
		if !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfObjectTypeRemove{
					ObjectTypeRemove: &pb.ChangeObjectTypeRemove{Url: v.URL()},
				},
			})
		}
	}
	return
}

func (o *ObjectTypes) ApplyChanges(ch *pb.ChangeContent) {
	switch {
	case ch.GetObjectTypeRemove() != nil:
		if err := o.changeObjectTypeRemove(ch.GetObjectTypeRemove()); err != nil {
			return
		}
	case ch.GetObjectTypeAdd() != nil:
		if err := o.changeObjectTypeAdd(ch.GetObjectTypeAdd()); err != nil {
			return
		}
	}
}

func (o *ObjectTypes) changeObjectTypeAdd(add *pb.ChangeObjectTypeAdd) error {
	if add.Url != "" {
		// migration of the old type changes
		// before we were storing the change ID instead of Key
		// but it's pretty easy to convert it
		add.Key, _ = migrateObjectTypeIDToKey(add.Url)
	}

	for _, ot := range o.ObjectTypeKeys() {
		if ot == domain.TypeKey(add.Key) {
			return nil
		}
	}
	objectTypes := append(o.ObjectTypeKeys(), domain.TypeKey(add.Key))
	o.SetObjectTypeKeys(objectTypes)
	return nil
}

func (o *ObjectTypes) changeObjectTypeRemove(remove *pb.ChangeObjectTypeRemove) error {
	var found bool
	if remove.Url != "" {
		remove.Key, _ = migrateObjectTypeIDToKey(remove.Url)
	}
	o.SetObjectTypeKeys(slice.Filter(o.ObjectTypeKeys(), func(key domain.TypeKey) bool {
		if key == domain.TypeKey(remove.Key) {
			found = true
			return false
		}
		return true
	}))
	if !found {
		log.Warnf("changeObjectTypeRemove: type to remove not found: '%s'", remove.Url)
	} else {
		o.SetObjectTypeKeys(o.ObjectTypeKeys())
	}
	return nil
}

func migrateObjectTypeIDToKey(old string) (new string, migrated bool) {
	if strings.HasPrefix(old, addr.ObjectTypeKeyToIdPrefix) {
		return strings.TrimPrefix(old, addr.ObjectTypeKeyToIdPrefix), true
	} else if strings.HasPrefix(old, addr.BundledObjectTypeURLPrefix) {
		return strings.TrimPrefix(old, addr.BundledObjectTypeURLPrefix), true
	}
	return old, false
}
