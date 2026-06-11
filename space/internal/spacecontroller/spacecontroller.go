package spacecontroller

import (
	"context"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type SpaceController interface {
	SpaceId() string
	// Start demands the space and blocks until its lifecycle process is
	// running (loading/joining/offloading started).
	Start(ctx context.Context) error
	// Demand marks the space as wanted-loaded without blocking. The
	// controller loads it in the background unless its status dictates
	// offloading/joining.
	Demand()
	// WaitLoad demands the space and blocks until it is fully loaded,
	// returning the real load error on failure.
	WaitLoad(ctx context.Context) (clientspace.Space, error)
	// WaitMode blocks until the lifecycle process for mode m is running,
	// surfacing the real transition error on failure. It fails fast when m is
	// not the controller's current target.
	WaitMode(ctx context.Context, m mode.Mode) error
	Mode() mode.Mode
	Update() error
	SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error
	SetLocalInfo(ctx context.Context, status spaceinfo.SpaceLocalInfo) error
	Close(ctx context.Context) error
	GetStatus() spaceinfo.AccountStatus
	GetLocalStatus() spaceinfo.LocalStatus
}
