package restriction

import (
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type RestrictionHolder interface {
	Type() model.SmartBlockType
	Layout() (model.ObjectTypeLayout, bool)
	// ObjectType is id of object type
	ObjectType() string
	UniqueKey() uniquekey.UniqueKey
}

type restrictionHolder struct {
	tp           model.SmartBlockType
	uk           uniquekey.UniqueKey
	layout       model.ObjectTypeLayout
	objectTypeID string
}

func newRestrictionHolder(sbType smartblock.SmartBlockType, layout model.ObjectTypeLayout, uk uniquekey.UniqueKey, objectTypeID string) RestrictionHolder {
	return &restrictionHolder{
		tp:           sbType.ToProto(),
		layout:       layout,
		uk:           uk,
		objectTypeID: objectTypeID,
	}
}

func (rh *restrictionHolder) Type() model.SmartBlockType {
	return rh.tp
}

func (rh *restrictionHolder) Layout() (model.ObjectTypeLayout, bool) {
	return rh.layout, rh.layout != noLayout
}

func (rh *restrictionHolder) ObjectType() string {
	return rh.objectTypeID
}

func (s *restrictionHolder) UniqueKey() uniquekey.UniqueKey {
	return s.uk
}
