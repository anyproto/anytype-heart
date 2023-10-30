package filesync

import (
	"context"
	"fmt"
	"net/http"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/conc"
)

type NodeUsage struct {
	AccountBytesLimit int
	TotalBytesUsage   int
	TotalCidsCount    int
	BytesLeft         uint64
	Spaces            []SpaceStat
}

type SpaceStat struct {
	SpaceId           string
	FileCount         int
	CidsCount         int
	TotalBytesUsage   int // Per account
	SpaceBytesUsage   int // Per space
	AccountBytesLimit int
}

type FileStat struct {
	SpaceId             string
	FileId              string
	TotalChunksCount    int
	UploadedChunksCount int
	BytesUsage          int
}

func (s FileStat) IsPinned() bool {
	return s.UploadedChunksCount == s.TotalChunksCount
}

func (s *fileSync) NodeUsage(ctx context.Context) (usage *NodeUsage, err error) {
	info, err := s.rpcStore.AccountInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("get node usage info: %w", err)
	}
	spaces := make([]SpaceStat, 0, len(info.Spaces))
	for _, space := range info.Spaces {
		spaces = append(spaces, SpaceStat{
			SpaceId:           space.SpaceId,
			FileCount:         int(space.FilesCount),
			CidsCount:         int(space.CidsCount),
			TotalBytesUsage:   int(space.TotalUsageBytes),
			SpaceBytesUsage:   int(space.SpaceUsageBytes),
			AccountBytesLimit: int(space.LimitBytes),
		})
	}
	left := uint64(0)
	if info.LimitBytes > info.TotalUsageBytes {
		left = info.LimitBytes - info.TotalUsageBytes
	}
	return &NodeUsage{
		AccountBytesLimit: int(info.LimitBytes),
		TotalCidsCount:    int(info.TotalCidsCount),
		TotalBytesUsage:   int(info.TotalUsageBytes),
		BytesLeft:         left,
		Spaces:            spaces,
	}, nil
}

// SpaceStat returns cached space usage information
func (f *fileSync) SpaceStat(ctx context.Context, spaceID string) (SpaceStat, error) {
	stats, ok, err := f.queue.getSpaceStats(spaceID)
	if err != nil {
		return SpaceStat{}, fmt.Errorf("get space info from store: %w", err)
	}
	if !ok {
		return f.getAndUpdateSpaceStat(ctx, spaceID)
	}
	return stats, nil
}

func (f *fileSync) getAndUpdateSpaceStat(ctx context.Context, spaceID string) (ss SpaceStat, err error) {
	info, err := f.rpcStore.SpaceInfo(ctx, spaceID)
	if err != nil {
		return
	}
	newStats := SpaceStat{
		SpaceId:           spaceID,
		FileCount:         int(info.FilesCount),
		CidsCount:         int(info.CidsCount),
		TotalBytesUsage:   int(info.TotalUsageBytes),
		SpaceBytesUsage:   int(info.SpaceUsageBytes),
		AccountBytesLimit: int(info.LimitBytes),
	}
	prevStats, prevStatsFound, err := f.queue.getSpaceStats(spaceID)
	if err != nil {
		return SpaceStat{}, fmt.Errorf("get space info from store: %w", err)
	}
	if prevStats != newStats {
		err = f.queue.setSpaceStats(spaceID, newStats)
		if err != nil {
			return SpaceStat{}, fmt.Errorf("save space info to store: %w", err)
		}
		// Do not send event if it is first time we get stats
		if prevStatsFound {
			f.sendSpaceUsageEvent(spaceID, uint64(newStats.SpaceBytesUsage))
		}
	}

	return newStats, nil
}

func (f *fileSync) updateSpaceUsageInformation(spaceID string) {
	if _, err := f.getAndUpdateSpaceStat(context.Background(), spaceID); err != nil {
		log.Warn("can't get space usage information", zap.String("spaceID", spaceID), zap.Error(err))
	}
}

func (f *fileSync) sendSpaceUsageEvent(spaceID string, bytesUsage uint64) {
	f.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfFileSpaceUsage{
					FileSpaceUsage: &pb.EventFileSpaceUsage{
						BytesUsage: bytesUsage,
						SpaceId:    spaceID,
					},
				},
			},
		},
	})
}

func (f *fileSync) FileListStats(ctx context.Context, spaceID string, fileIDs []string) ([]FileStat, error) {
	filesInfo, err := f.fetchFilesInfo(ctx, spaceID, fileIDs)
	if err != nil {
		return nil, err
	}
	return conc.MapErr(filesInfo, func(fileInfo *fileproto.FileInfo) (FileStat, error) {
		return f.fileInfoToStat(ctx, spaceID, fileInfo)
	})
}

func (f *fileSync) fetchFilesInfo(ctx context.Context, spaceId string, fileIDs []string) ([]*fileproto.FileInfo, error) {
	requests := lo.Chunk(fileIDs, 50)
	responses, err := conc.MapErr(requests, func(chunk []string) ([]*fileproto.FileInfo, error) {
		return f.rpcStore.FilesInfo(ctx, spaceId, chunk...)
	})
	if err != nil {
		return nil, err
	}
	return lo.Flatten(responses), nil
}

func (f *fileSync) FileStat(ctx context.Context, spaceId, fileId string) (fs FileStat, err error) {
	fi, err := f.rpcStore.FilesInfo(ctx, spaceId, fileId)
	if err != nil {
		return
	}
	if len(fi) == 0 {
		return FileStat{}, domain.ErrFileNotFound
	}
	file := fi[0]

	return f.fileInfoToStat(ctx, spaceId, file)
}

func (f *fileSync) fileInfoToStat(ctx context.Context, spaceId string, file *fileproto.FileInfo) (FileStat, error) {
	totalChunks, err := f.countChunks(ctx, spaceId, file.FileId)
	if err != nil {
		return FileStat{}, fmt.Errorf("count chunks: %w", err)
	}

	return FileStat{
		SpaceId:             spaceId,
		FileId:              file.FileId,
		TotalChunksCount:    totalChunks,
		UploadedChunksCount: int(file.CidsCount),
		BytesUsage:          int(file.UsageBytes),
	}, nil
}

func (f *fileSync) countChunks(ctx context.Context, spaceID string, fileID string) (int, error) {
	chunksCount, err := f.fileStore.GetChunksCount(fileID)
	if err == nil {
		return chunksCount, nil
	}

	chunksCount, err = f.fetchChunksCount(ctx, spaceID, fileID)
	if err != nil {
		return -1, fmt.Errorf("count chunks in IPFS: %w", err)
	}

	err = f.fileStore.SetChunksCount(fileID, chunksCount)

	return chunksCount, err
}

func (f *fileSync) fetchChunksCount(ctx context.Context, spaceID string, fileID string) (int, error) {
	fileCid, err := cid.Parse(fileID)
	if err != nil {
		return -1, err
	}
	dagService := f.dagServiceForSpace(spaceID)
	node, err := dagService.Get(ctx, fileCid)
	if err != nil {
		return -1, err
	}

	var count int
	visited := map[string]struct{}{}
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(node, dagService))
	err = walker.Iterate(func(node ipld.NavigableNode) error {
		id := node.GetIPLDNode().Cid().String()
		if _, ok := visited[id]; !ok {
			visited[id] = struct{}{}
			count++
		}
		return nil
	})
	if err == ipld.EndOfDag {
		err = nil
	}
	return count, err
}

func (f *fileSync) DebugQueue(_ *http.Request) (*QueueInfo, error) {
	var (
		info QueueInfo
		err  error
	)

	info.UploadingQueue, err = f.queue.listItemsByPrefix(uploadKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("list items from uploading queue: %w", err)
	}
	info.DiscardedQueue, err = f.queue.listItemsByPrefix(discardedKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("list items from discarded queue: %w", err)
	}
	info.RemovingQueue, err = f.queue.listItemsByPrefix(removeKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("list items from removing queue: %w", err)
	}
	return &info, nil
}
