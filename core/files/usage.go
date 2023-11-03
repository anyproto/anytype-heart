package files

import (
	"context"
	"fmt"

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

type NodeUsageResponse struct {
	Usage           filesync.NodeUsage
	LocalUsageBytes uint64
}

func (s *service) GetNodeUsage(ctx context.Context) (*NodeUsageResponse, error) {
	usage, err := s.fileSync.NodeUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("node usage: %w", err)
	}

	localUsage, err := s.fileStorage.LocalDiskUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("local disk usage: %w", err)
	}

	return &NodeUsageResponse{
		Usage:           usage,
		LocalUsageBytes: localUsage,
	}, nil
}
