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
	UpdateStatus(ctx context.Context, status spaceinfo.AccountStatus) error
	SetStatus(ctx context.Context, status spaceinfo.AccountStatus) error
	UpdateRemoteStatus(ctx context.Context, status spaceinfo.RemoteStatus) error
	Close(ctx context.Context) error
}

type DeleteController interface {
	Delete(ctx context.Context) error
}
