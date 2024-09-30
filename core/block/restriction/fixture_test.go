package restriction

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const noLayout = -1

type restrictionHolder struct {
	sbType    smartblock.SmartBlockType
	uniqueKey domain.UniqueKey
	layout    model.ObjectTypeLayout
}

func (rh *restrictionHolder) Type() smartblock.SmartBlockType {
	return rh.sbType
}

func (rh *restrictionHolder) Layout() (model.ObjectTypeLayout, bool) {
	return rh.layout, rh.layout != noLayout
}

func (rh *restrictionHolder) UniqueKey() domain.UniqueKey {
	return rh.uniqueKey
}

func givenObjectType(typeKey domain.TypeKey) RestrictionHolder {
	return &restrictionHolder{
		sbType:    smartblock.SmartBlockTypeObjectType,
		layout:    model.ObjectType_objectType,
		uniqueKey: domain.MustUniqueKey(smartblock.SmartBlockTypeObjectType, typeKey.String()),
	}
}

func givenRelation(relationKey domain.RelationKey) RestrictionHolder {
	return &restrictionHolder{
		sbType:    smartblock.SmartBlockTypeRelation,
		layout:    model.ObjectType_relation,
		uniqueKey: domain.MustUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String()),
	}
}

func givenRestrictionHolder(sbType smartblock.SmartBlockType, typeKey domain.TypeKey) RestrictionHolder {
	layout := model.ObjectType_basic
	t, err := bundle.GetType(typeKey)
	if err == nil {
		layout = t.Layout
	}
	uk, _ := domain.NewUniqueKey(sbType, "")
	return &restrictionHolder{
		sbType:    sbType,
		layout:    layout,
		uniqueKey: uk,
	}
}
