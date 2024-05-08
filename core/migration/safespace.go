package migration

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

type DoableSpace interface {
	Do(objectId string, apply func(sb smartblock.SmartBlock) error) error
	Id() string
}

type safeSpace struct {
	space       clientspace.Space
	CtxExceeded bool
}

func (s safeSpace) Do(objectId string, apply func(sb smartblock.SmartBlock) error) error {
	if s.CtxExceeded {
		return ErrCtxExceeded
	}
	return s.space.Do(objectId, apply)
}

func (s safeSpace) Id() string {
	if s.CtxExceeded {
		return ""
	}
	return s.space.Id()
}
