package files

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (s *Service) GetSpaceUsage(ctx context.Context) (*pb.RpcFileSpaceUsageResponseUsage, error) {
	stat, err := s.fileSync.SpaceStat(ctx, s.spaceService.AccountId())
	if err != nil {
		return nil, err
	}

	usage, err := s.fileStorage.LocalDiskUsage(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.RpcFileSpaceUsageResponseUsage{
		FilesCount:      uint64(stat.FileCount),
		CidsCount:       uint64(stat.CidsCount),
		BytesUsage:      uint64(stat.BytesUsage),
		BytesLeft:       uint64(stat.BytesLimit - stat.BytesUsage),
		BytesLimit:      uint64(stat.BytesLimit),
		LocalBytesUsage: usage,
	}, nil
}
