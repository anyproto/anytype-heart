package restriction

import (
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type RestrictionHolder interface {
	Id() string
	Type() model.SmartBlockType
	Layout() (model.ObjectTypeLayout, bool)
	ObjectType() string
	UniqueKey() uniquekey.UniqueKey
}

type restrictionHolder struct {
	id         string
	tp         model.SmartBlockType
	uk         uniquekey.UniqueKey
	layout     model.ObjectTypeLayout
	objectType string
}

func newRestrictionHolder(id string, sbType smartblock.SmartBlockType, layout model.ObjectTypeLayout, uk uniquekey.UniqueKey, ot string) RestrictionHolder {
	return &restrictionHolder{
		id:         id,
		tp:         sbType.ToProto(),
		layout:     layout,
		uk:         uk,
		objectType: ot,
	}
}

func (rh *restrictionHolder) Id() string {
	return rh.id
}

func (rh *restrictionHolder) Type() model.SmartBlockType {
	return rh.tp
}

func (rh *restrictionHolder) Layout() (model.ObjectTypeLayout, bool) {
	return rh.layout, rh.layout != noLayout
}

func (rh *restrictionHolder) ObjectType() string {
	return rh.objectType
}

func (s *restrictionHolder) UniqueKey() uniquekey.UniqueKey {
	return s.uk
}
