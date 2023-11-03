package filesync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/dgraph-io/badger/v3"
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

func (u NodeUsage) GetSpaceUsage(spaceId string) SpaceStat {
	for _, space := range u.Spaces {
		if space.SpaceId == spaceId {
			return space
		}
	}
	return SpaceStat{
		SpaceId:           spaceId,
		TotalBytesUsage:   u.TotalBytesUsage,
		AccountBytesLimit: u.AccountBytesLimit,
	}
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

func (f *fileSync) runNodeUsageUpdater() {
	f.precacheNodeUsage()

	ticker := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-ticker.C:
			_, err := f.getAndUpdateNodeUsage(context.Background())
			if err != nil {
				log.Error("updater: can't update node usage", zap.Error(err))
			}
		case <-f.loopCtx.Done():
			return
		}
	}
}

func (f *fileSync) precacheNodeUsage() {
	_, ok, err := f.getCachedNodeUsage()
	// Init cache with default limits
	if !ok || err != nil {
		err = f.queue.setNodeUsage(NodeUsage{
			AccountBytesLimit: 1024 * 1024 * 1024, // 1 GB
		})
		if err != nil {
			log.Error("can't set default limits", zap.Error(err))
		}
	}

	// Load actual node usage
	_, err = f.getAndUpdateNodeUsage(context.Background())
	if err != nil {
		log.Error("can't init node usage cache", zap.Error(err))

		// Don't confuse users with 0B limit in case of error, so set default 1GB limit
		err = f.queue.setNodeUsage(NodeUsage{
			AccountBytesLimit: 1024 * 1024 * 1024, // 1 GB
		})
		if err != nil {
			log.Error("can't set default limits", zap.Error(err))
		}
	}
}

func (s *fileSync) NodeUsage(ctx context.Context) (NodeUsage, error) {
	usage, ok, err := s.getCachedNodeUsage()
	if err != nil {
		return NodeUsage{}, fmt.Errorf("get cached node usage: %w", err)
	}
	if !ok {
		return s.getAndUpdateNodeUsage(ctx)
	}
	return usage, err
}

func (s *fileSync) getCachedNodeUsage() (NodeUsage, bool, error) {
	usage, err := s.queue.getNodeUsage()
	if errors.Is(err, badger.ErrKeyNotFound) {
		return NodeUsage{}, false, nil
	}
	if err != nil {
		return NodeUsage{}, false, err
	}
	return usage, true, nil
}

func (s *fileSync) getAndUpdateNodeUsage(ctx context.Context) (NodeUsage, error) {
	info, err := s.rpcStore.AccountInfo(ctx)
	if err != nil {
		return NodeUsage{}, fmt.Errorf("get node usage info: %w", err)
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
	usage := NodeUsage{
		AccountBytesLimit: int(info.LimitBytes),
		TotalCidsCount:    int(info.TotalCidsCount),
		TotalBytesUsage:   int(info.TotalUsageBytes),
		BytesLeft:         left,
		Spaces:            spaces,
	}
	err = s.queue.setNodeUsage(usage)
	if err != nil {
		return NodeUsage{}, fmt.Errorf("save node usage info to store: %w", err)
	}
	return usage, nil
}

// SpaceStat returns cached space usage information
func (f *fileSync) SpaceStat(ctx context.Context, spaceId string) (SpaceStat, error) {
	usage, err := f.NodeUsage(ctx)
	if err != nil {
		return SpaceStat{}, err
	}
	return usage.GetSpaceUsage(spaceId), nil
}

func (s *fileSync) getAndUpdateSpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error) {
	prevUsage, prevUsageFound, err := s.getCachedNodeUsage()
	if err != nil {
		return SpaceStat{}, fmt.Errorf("get cached node usage: %w", err)
	}

	curUsage, err := s.getAndUpdateNodeUsage(ctx)
	if err != nil {
		return SpaceStat{}, fmt.Errorf("get and update node usage: %w", err)
	}

	prevStats := prevUsage.GetSpaceUsage(spaceId)
	newStats := curUsage.GetSpaceUsage(spaceId)
	if prevStats != newStats {
		// Do not send event if it is first time we get stats
		if prevUsageFound {
			s.sendSpaceUsageEvent(spaceId, uint64(newStats.SpaceBytesUsage))
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
