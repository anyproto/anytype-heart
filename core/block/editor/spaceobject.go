package editor

import (
	"errors"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

// SpaceObject is a wrapper around smartblock.SmartBlock that provides
// additional functionality for space creation/deletion/etc
type SpaceObject struct {
	smartblock.SmartBlock
}

// spaceObjectDeps is a set of dependencies for SpaceObject
type spaceObjectDeps struct {
}

// newSpaceObject creates a new SpaceObject with given deps
func newSpaceObject(sb smartblock.SmartBlock, deps spaceObjectDeps) *SpaceObject {
	return &SpaceObject{
		SmartBlock: sb,
	}
}

// Init initializes SpaceObject
func (p *SpaceObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.DisableLayouts()
	return
}

// targetSpaceID returns space id from the root of space object's tree
func (p *SpaceObject) targetSpaceID() (id string, err error) {
	changeInfo := p.Tree().ChangeInfo()
	if changeInfo == nil {
		return "", ErrIncorrectSpaceInfo
	}
	var (
		changePayload = &model.ObjectChangePayload{}
		spaceHeader   = &model.SpaceObjectHeader{}
	)
	err = proto.Unmarshal(changeInfo.ChangePayload, changePayload)
	if err != nil {
		return "", ErrIncorrectSpaceInfo
	}
	err = proto.Unmarshal(changePayload.Data, spaceHeader)
	if err != nil {
		return "", ErrIncorrectSpaceInfo
	}
	return spaceHeader.SpaceID, nil
}
