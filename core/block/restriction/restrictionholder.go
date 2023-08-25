package restriction

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type RestrictionHolder interface {
	Type() smartblock.SmartBlockType
	Layout() (model.ObjectTypeLayout, bool)
	ObjectTypeID() string
	UniqueKey() domain.UniqueKey
}

type restrictionHolder struct {
	sbType       smartblock.SmartBlockType
	uniqueKey    domain.UniqueKey
	layout       model.ObjectTypeLayout
	objectTypeID string
}

func newRestrictionHolder(sbType smartblock.SmartBlockType, layout model.ObjectTypeLayout, uk domain.UniqueKey, objectTypeID string) RestrictionHolder {
	return &restrictionHolder{
		sbType:       sbType,
		layout:       layout,
		uniqueKey:    uk,
		objectTypeID: objectTypeID,
	}
}

func (rh *restrictionHolder) Type() smartblock.SmartBlockType {
	return rh.sbType
}

func (rh *restrictionHolder) Layout() (model.ObjectTypeLayout, bool) {
	return rh.layout, rh.layout != noLayout
}

func (rh *restrictionHolder) ObjectTypeID() string {
	return rh.objectTypeID
}

func (s *restrictionHolder) UniqueKey() domain.UniqueKey {
	return s.uniqueKey
}
