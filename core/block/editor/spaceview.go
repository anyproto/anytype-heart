package editor

import (
	"context"
	"errors"
	"time"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

type spaceService interface {
	OnViewCreated(ctx context.Context, spaceID string) (info spaceinfo.SpaceInfo, err error)
}

// SpaceView is a wrapper around smartblock.SmartBlock that indicates the current space state
type SpaceView struct {
	smartblock.SmartBlock
	spaceService spaceService
}

// newSpaceObject creates a new SpaceView with given deps
func newSpaceObject(sb smartblock.SmartBlock, spaceService spaceService) *SpaceView {
	return &SpaceView{
		SmartBlock:   sb,
		spaceService: spaceService,
	}
}

// Init initializes SpaceView
func (s *SpaceView) Init(ctx *smartblock.InitContext) (err error) {
	if err = s.SmartBlock.Init(ctx); err != nil {
		return
	}
	spaceID, err := s.targetSpaceID()
	if err != nil {
		return
	}

	s.DisableLayouts()
	info, err := s.spaceService.OnViewCreated(ctx.Ctx, spaceID)
	if err != nil {
		return
	}
	return s.setSpaceInfo(ctx.State, info)
}

func (s *SpaceView) TryClose(objectTTL time.Duration) (res bool, err error) {
	return false, nil
}

func (s *SpaceView) SetSpaceInfo(info spaceinfo.SpaceInfo) (err error) {
	st := s.NewState()
	if err = s.setSpaceInfo(st, info); err != nil {
		return
	}
	return s.Apply(st)
}

func (s *SpaceView) setSpaceInfo(st *state.State, info spaceinfo.SpaceInfo) (err error) {
	// TODO: create relations and values emum mapping
	st.SetLocalDetail("spaceStatusLocal", pbtypes.Int64(int64(info.LocalStatus)))
	st.SetLocalDetail("spaceStatusRemote", pbtypes.Int64(int64(info.RemoteStatus)))
	return
}

// targetSpaceID returns space id from the root of space object's tree
func (s *SpaceView) targetSpaceID() (id string, err error) {
	changeInfo := s.Tree().ChangeInfo()
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
