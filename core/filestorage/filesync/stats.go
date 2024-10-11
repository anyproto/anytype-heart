package filesync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/dgraph-io/badger/v4"
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

func (s *fileSync) runNodeUsageUpdater() {
	defer s.closeWg.Done()

	s.precacheNodeUsage()

	ticker := time.NewTicker(time.Second * 10)
	slowMode := false
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cachedUsage, cachedUsageExists, _ := s.getCachedNodeUsage()
			ctx, cancel := context.WithCancel(s.loopCtx)
			_, err := s.getAndUpdateNodeUsage(ctx)
			cancel()
			if err != nil {
				log.Warn("updater: can't update node usage", zap.Error(err))
			} else {
				updatedUsage, updatedUsageExists, _ := s.getCachedNodeUsage()
				if cachedUsageExists && updatedUsageExists && cachedUsage.BytesLeft == updatedUsage.BytesLeft {
					// looks like we don't have active uploads we should actively follow
					// let's slow down the updates
					if !slowMode {
						ticker.Reset(time.Minute)
						slowMode = true
					}
				} else {
					// we have activity, or updated BytesLeft for the first time
					// let's keep the updates frequent
					if slowMode {
						ticker.Reset(time.Second * 10)
						slowMode = false
					}
				}
			}
		case <-s.loopCtx.Done():
			return
		}
	}
}

func (s *fileSync) precacheNodeUsage() {
	_, ok, err := s.getCachedNodeUsage()
	// Init cache with default limits
	if !ok || err != nil {
		err = s.store.setNodeUsage(NodeUsage{
			AccountBytesLimit: 1024 * 1024 * 1024, // 1 GB
		})
		if err != nil {
			log.Error("can't set default limits", zap.Error(err))
		}
	}

	// Load actual node usage
	ctx, cancel := context.WithCancel(s.loopCtx)
	defer cancel()
	_, err = s.getAndUpdateNodeUsage(ctx)
	if err != nil {
		log.Error("can't init node usage cache", zap.Error(err))
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

func (s *fileSync) UpdateNodeUsage(ctx context.Context) error {
	_, err := s.getAndUpdateNodeUsage(ctx)
	return err
}

func (s *fileSync) getCachedNodeUsage() (NodeUsage, bool, error) {
	usage, err := s.store.getNodeUsage()
	if errors.Is(err, badger.ErrKeyNotFound) {
		return NodeUsage{}, false, nil
	}
	if err != nil {
		return NodeUsage{}, false, err
	}
	return usage, true, nil
}

func (s *fileSync) getAndUpdateNodeUsage(ctx context.Context) (NodeUsage, error) {
	prevUsage, prevUsageFound, err := s.getCachedNodeUsage()
	if err != nil {
		return NodeUsage{}, fmt.Errorf("get cached node usage: %w", err)
	}

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
	err = s.store.setNodeUsage(usage)
	if err != nil {
		return NodeUsage{}, fmt.Errorf("save node usage info to store: %w", err)
	}

	if !prevUsageFound || prevUsage.AccountBytesLimit != usage.AccountBytesLimit {
		s.sendLimitUpdatedEvent(uint64(usage.AccountBytesLimit))
	}

	for _, space := range spaces {
		if !prevUsageFound || prevUsage.GetSpaceUsage(space.SpaceId).SpaceBytesUsage != space.SpaceBytesUsage {
			s.sendSpaceUsageEvent(space.SpaceId, uint64(space.SpaceBytesUsage))
		}
	}

	return usage, nil
}

// SpaceStat returns cached space usage information
func (s *fileSync) SpaceStat(ctx context.Context, spaceId string) (SpaceStat, error) {
	usage, err := s.NodeUsage(ctx)
	if err != nil {
		return SpaceStat{}, err
	}
	return usage.GetSpaceUsage(spaceId), nil
}

func (s *fileSync) getAndUpdateSpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error) {
	curUsage, err := s.getAndUpdateNodeUsage(ctx)
	if err != nil {
		return SpaceStat{}, fmt.Errorf("get and update node usage: %w", err)
	}

	return curUsage.GetSpaceUsage(spaceId), nil
}

func (s *fileSync) updateSpaceUsageInformation(spaceID string) {
	if _, err := s.getAndUpdateSpaceStat(context.Background(), spaceID); err != nil {
		log.Warn("can't get space usage information", zap.String("spaceID", spaceID), zap.Error(err))
	}
}

func (s *fileSync) sendSpaceUsageEvent(spaceId string, bytesUsage uint64) {
	s.eventSender.Broadcast(makeSpaceUsageEvent(spaceId, bytesUsage))
}

func makeSpaceUsageEvent(spaceId string, bytesUsage uint64) *pb.Event {
	return &pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfFileSpaceUsage{
					FileSpaceUsage: &pb.EventFileSpaceUsage{
						BytesUsage: bytesUsage,
						SpaceId:    spaceId,
					},
				},
			},
		},
	}
}

func (s *fileSync) sendLimitUpdatedEvent(limit uint64) {
	s.eventSender.Broadcast(makeLimitUpdatedEvent(limit))
}

func makeLimitUpdatedEvent(limit uint64) *pb.Event {
	return &pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfFileLimitUpdated{
					FileLimitUpdated: &pb.EventFileLimitUpdated{
						BytesLimit: limit,
					},
				},
			},
		},
	}
}

func (s *fileSync) FileListStats(ctx context.Context, spaceID string, hashes []domain.FileId) ([]FileStat, error) {
	filesInfo, err := s.fetchFilesInfo(ctx, spaceID, hashes)
	if err != nil {
		return nil, err
	}
	return conc.MapErr(filesInfo, func(fileInfo *fileproto.FileInfo) (FileStat, error) {
		return s.fileInfoToStat(ctx, spaceID, fileInfo)
	})
}

func (s *fileSync) fetchFilesInfo(ctx context.Context, spaceId string, hashes []domain.FileId) ([]*fileproto.FileInfo, error) {
	requests := lo.Chunk(hashes, 50)
	responses, err := conc.MapErr(requests, func(chunk []domain.FileId) ([]*fileproto.FileInfo, error) {
		return s.rpcStore.FilesInfo(ctx, spaceId, chunk...)
	})
	if err != nil {
		return nil, err
	}
	return lo.Flatten(responses), nil
}

func (s *fileSync) fileInfoToStat(ctx context.Context, spaceId string, file *fileproto.FileInfo) (FileStat, error) {
	totalChunks, err := s.countChunks(ctx, spaceId, domain.FileId(file.FileId))
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

func (s *fileSync) countChunks(ctx context.Context, spaceID string, fileId domain.FileId) (int, error) {
	chunksCount, err := s.fileStore.GetChunksCount(fileId)
	if err == nil {
		return chunksCount, nil
	}

	chunksCount, err = s.fetchChunksCount(ctx, spaceID, fileId)
	if err != nil {
		return -1, fmt.Errorf("count chunks in IPFS: %w", err)
	}

	err = s.fileStore.SetChunksCount(fileId, chunksCount)

	return chunksCount, err
}

func (s *fileSync) fetchChunksCount(ctx context.Context, spaceID string, fileId domain.FileId) (int, error) {
	fileCid, err := cid.Parse(fileId.String())
	if err != nil {
		return -1, err
	}
	dagService := s.dagServiceForSpace(spaceID)
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

func (s *fileSync) DebugQueue(_ *http.Request) (*QueueInfo, error) {
	var info QueueInfo
	info.UploadingQueue = s.uploadingQueue.ListKeys()
	info.RetryUploadingQueue = s.retryUploadingQueue.ListKeys()
	info.DeletionQueue = s.deletionQueue.ListKeys()
	info.RetryDeletionQueue = s.retryDeletionQueue.ListKeys()
	return &info, nil
}
