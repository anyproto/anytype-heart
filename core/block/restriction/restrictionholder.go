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
	UniqueKey() uniquekey.UniqueKey
}

type restrictionHolder struct {
	id     string
	tp     model.SmartBlockType
	uk     uniquekey.UniqueKey
	layout model.ObjectTypeLayout
}

func newRestrictionHolder(id string, sbType smartblock.SmartBlockType, layout model.ObjectTypeLayout, uk uniquekey.UniqueKey) RestrictionHolder {
	return &restrictionHolder{
		id:     id,
		tp:     sbType.ToProto(),
		layout: layout,
		uk:     uk,
	}
}

func (s *restrictionHolder) Id() string {
	return s.id
}

func (s *restrictionHolder) Type() model.SmartBlockType {
	return s.tp
}

func (s *restrictionHolder) UniqueKey() uniquekey.UniqueKey {
	return s.uk
}

func (s *restrictionHolder) Layout() (model.ObjectTypeLayout, bool) {
	return s.layout, s.layout != noLayout
}
