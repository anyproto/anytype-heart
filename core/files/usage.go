package files

import (
	"context"

	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pb"
)

func (s *service) GetSpaceUsage(ctx context.Context, spaceID string) (*pb.RpcFileSpaceUsageResponseUsage, error) {
	stat, err := s.fileSync.SpaceStat(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	usage, err := s.fileStorage.LocalDiskUsage(ctx)
	if err != nil {
		return nil, err
	}

	left := stat.AccountBytesLimit - stat.TotalBytesUsage
	if left < 0 {
		left = 0
	}

	return &pb.RpcFileSpaceUsageResponseUsage{
		FilesCount:      uint64(stat.FileCount),
		CidsCount:       uint64(stat.CidsCount),
		BytesUsage:      uint64(stat.SpaceBytesUsage),
		BytesLeft:       uint64(left),
		BytesLimit:      uint64(stat.AccountBytesLimit),
		LocalBytesUsage: usage,
	}, nil
}

func (s *service) GetNodeUsage(ctx context.Context) (filesync.NodeUsage, error) {
	return s.fileSync.NodeUsage(ctx)
}
