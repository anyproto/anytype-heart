package space

import (
	"context"

	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

func (s *service) DerivedIDs(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error) {
	spc, err := s.Get(ctx, spaceID)
	if err != nil {
		return threads.DerivedSmartblockIds{}, err
	}
	return spc.DeriveObjectIDs(ctx)
}
