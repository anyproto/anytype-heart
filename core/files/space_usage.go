package files

import (
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
)

func (s *service) GetSpaceUsage(ctx session.Context) (*pb.RpcFileSpaceUsageResponseUsage, error) {
	stat, err := s.fileSync.SpaceStat(ctx.Context(), ctx.SpaceID())
	if err != nil {
		return nil, err
	}

	usage, err := s.fileStorage.LocalDiskUsage(ctx.Context())
	if err != nil {
		return nil, err
	}

	left := stat.BytesLimit - stat.BytesUsage
	if left < 0 {
		left = 0
	}

	return &pb.RpcFileSpaceUsageResponseUsage{
		FilesCount:      uint64(stat.FileCount),
		CidsCount:       uint64(stat.CidsCount),
		BytesUsage:      uint64(stat.BytesUsage),
		BytesLeft:       uint64(left),
		BytesLimit:      uint64(stat.BytesLimit),
		LocalBytesUsage: usage,
	}, nil
}
