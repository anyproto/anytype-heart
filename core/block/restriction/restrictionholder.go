package restriction

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type RestrictionHolder interface {
	Id() string
	Type() model.SmartBlockType
	Layout() (model.ObjectTypeLayout, bool)
}

type restrictionHolder struct {
	id     string
	tp     model.SmartBlockType
	layout model.ObjectTypeLayout
}

func newRestrictionHolder(id string, sbType smartblock.SmartBlockType, layout model.ObjectTypeLayout) RestrictionHolder {
	return &restrictionHolder{
		id:     id,
		tp:     sbType.ToProto(),
		layout: layout,
	}
}

func (s *restrictionHolder) Id() string {
	return s.id
}

func (s *restrictionHolder) Type() model.SmartBlockType {
	return s.tp
}

func (s *restrictionHolder) Layout() (model.ObjectTypeLayout, bool) {
	return s.layout, s.layout != -1
}
