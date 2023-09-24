package editor

import (
	"errors"
	"time"

	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceobject"
)

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

// SpaceObject is a wrapper around smartblock.SmartBlock that provides
// additional functionality for space creation/deletion/etc
type SpaceObject struct {
	smartblock.SmartBlock
	spaceobject.SpaceObject
}

// spaceObjectDeps is a set of dependencies for SpaceObject
type spaceObjectDeps struct {
	installer bundledObjectsInstaller
	cache     objectcache.Cache
	spaceCore spacecore.SpaceCoreService
	provider  personalIDProvider
}

// newSpaceObject creates a new SpaceObject with given deps
func newSpaceObject(sb smartblock.SmartBlock, deps spaceObjectDeps) *SpaceObject {
	spaceID := mustTargetSpaceID(sb)
	return &SpaceObject{
		SmartBlock: sb,
		SpaceObject: spaceobject.NewSpaceObject(sb.Id(), spaceobject.Deps{
			Installer:  deps.installer,
			Cache:      deps.cache,
			SpaceCore:  deps.spaceCore,
			SpaceID:    spaceID,
			IsPersonal: spaceID == deps.provider.PersonalSpaceID(),
		}),
	}
}

// Init initializes SpaceObject
func (s *SpaceObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = s.SmartBlock.Init(ctx); err != nil {
		return
	}
	err = s.SpaceObject.Run(ctx.Ctx)
	if err != nil {
		return
	}
	s.DisableLayouts()
	return
}

func (s *SpaceObject) TryClose(objectTTL time.Duration) (res bool, err error) {
	return false, nil
}

func (s *SpaceObject) Close() (err error) {
	if err := s.SpaceObject.Close(); err != nil {
		log.Error("failed to close space object", zap.Error(err), zap.String("id", s.SmartBlock.Id()))
	}
	return s.SmartBlock.Close()
}

func mustTargetSpaceID(s smartblock.SmartBlock) string {
	id, err := targetSpaceID(s)
	if err != nil {
		panic("space object cannot be without space ID in header")
	}
	return id
}

// targetSpaceID returns space id from the root of space object's tree
func targetSpaceID(s smartblock.SmartBlock) (id string, err error) {
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
