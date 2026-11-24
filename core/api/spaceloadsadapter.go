package api

import (
	"context"
	"fmt"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
)

// spaceLoadsAdapter implements apicore.SpaceLoads over space.Service.Get:
// the wait every live-object read and every write of API v2 reaches anyway,
// exposed on its own so the v2 service can put a deadline on it before it
// calls an RPC that drops the request context.
type spaceLoadsAdapter struct {
	spaces spaceGetter
}

func newSpaceLoadsAdapter(spaces spaceGetter) apicore.SpaceLoads {
	return &spaceLoadsAdapter{spaces: spaces}
}

func (a *spaceLoadsAdapter) WaitLoaded(ctx context.Context, spaceId string) error {
	if _, err := a.spaces.Get(ctx, spaceId); err != nil {
		return fmt.Errorf("get space %s: %w", spaceId, err)
	}
	return nil
}
