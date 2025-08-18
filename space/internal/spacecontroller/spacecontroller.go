package spacecontroller

import (
	"context"

	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type SpaceController interface {
	SpaceId() string
	Start(ctx context.Context) error
	Mode() mode.Mode
	Current() any
	Update() error
	SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error
	SetLocalInfo(ctx context.Context, status spaceinfo.SpaceLocalInfo) error
	Close(ctx context.Context) error
	GetStatus() spaceinfo.AccountStatus
	GetLocalStatus() spaceinfo.LocalStatus
}
