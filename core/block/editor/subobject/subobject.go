package subobject

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
)

func NewSubObject(subId string, parent ParentObject) *SubObject {
	return &SubObject{
		id:           subId,
		ParentObject: parent,
		SmartBlock:   smartblock.New(),
	}
}

type ParentObject interface {
	Id() string
	SubState(subId string) (s *state.State, err error)
	SubStateApply(subId string, s *state.State) (err error)
}

type SubObject struct {
	id           string
	ParentObject ParentObject
	smartblock.SmartBlock
}

func (s *SubObject) Id() string {
	return s.ParentObject.Id() + "/" + s.id
}

func (s *SubObject) SubId() string {
	return s.id
}

func (s *SubObject) SubInit() {
	s.AddHook(func(st *state.State) (err error) {
		return s.ParentObject.SubStateApply(s.id, st)
	}, smartblock.HookAfterApply)
}
