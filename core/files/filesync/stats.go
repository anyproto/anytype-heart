package filesync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	anystore "github.com/anyproto/any-store"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
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

// Equal assume that Spaces slice is sorted by SpaceId
func (a NodeUsage) Equal(b NodeUsage) bool {
	if a.AccountBytesLimit != b.AccountBytesLimit ||
		a.TotalBytesUsage != b.TotalBytesUsage ||
		a.TotalCidsCount != b.TotalCidsCount ||
		a.BytesLeft != b.BytesLeft {
		return false
	}

	if len(a.Spaces) != len(b.Spaces) {
		return false
	}

	for i := range a.Spaces {
		if !a.Spaces[i].Equal(b.Spaces[i]) {
			return false
		}
	}

	return true
}

func (a SpaceStat) Equal(b SpaceStat) bool {
	return a.SpaceId == b.SpaceId &&
		a.FileCount == b.FileCount &&
		a.CidsCount == b.CidsCount &&
		a.TotalBytesUsage == b.TotalBytesUsage &&
		a.SpaceBytesUsage == b.SpaceBytesUsage &&
		a.AccountBytesLimit == b.AccountBytesLimit
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

	ticker := time.NewTicker(nodeUsageInterval(false))
	slowMode := false
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-s.nodeUsageUpdateCh:
		case <-s.loopCtx.Done():
			return
		}
		slowMode = switchNodeUsageCadence(ticker, slowMode, s.updateNodeUsageTick())
	}
}

// switchNodeUsageCadence resets the ticker when the cadence changes
func switchNodeUsageCadence(ticker interface{ Reset(time.Duration) }, slowMode, slow bool) bool {
	if slow != slowMode {
		ticker.Reset(nodeUsageInterval(slow))
	}
	return slow
}

// nodeUsageInterval is the updater cadence: fast while uploads move the
// usage, slow when idle or the node is unreachable
func nodeUsageInterval(slow bool) time.Duration {
	if slow {
		return time.Minute
	}
	return time.Second * 10
}

// updateNodeUsageTick refreshes the node usage and reports whether the
// updater should run slowly: the node is unreachable (e.g. device offline
// mid-upload) or the usage didn't move (no active uploads to follow)
func (s *fileSync) updateNodeUsageTick() (slow bool) {
	cachedUsage, cachedUsageExists, cachedErr := s.getCachedNodeUsage()
	ctx, cancel := context.WithCancel(s.loopCtx)
	_, err := s.getAndUpdateNodeUsage(ctx)
	cancel()
	if err != nil {
		log.Warn("updater: can't update node usage", zap.Error(err))
		return true
	}
	updatedUsage, updatedUsageExists, updatedErr := s.getCachedNodeUsage()
	return cachedErr == nil && updatedErr == nil && cachedUsageExists && updatedUsageExists &&
		cachedUsage.BytesLeft == updatedUsage.BytesLeft
}

// RequestNodeUsageUpdate wakes runNodeUsageUpdater for an early refresh, e.g.
// after a membership change. It never blocks the caller, and requests made
// while one is pending (or while an update is running) coalesce into one.
// In local-only mode the updater doesn't run and the request is a no-op.
func (s *fileSync) RequestNodeUsageUpdate() {
	select {
	case s.nodeUsageUpdateCh <- struct{}{}:
	default:
	}
}

func (s *fileSync) precacheNodeUsage() {
	_, ok, err := s.getCachedNodeUsage()
	// Init cache with default limits
	if !ok || err != nil {
		defaultUsage := NodeUsage{
			AccountBytesLimit: 100 * 1024 * 1024, // 100 MB
		}
		err = s.nodeUsageStore.Set(context.Background(), anystoreprovider.SystemKeys.NodeUsage(), defaultUsage)
		if err != nil {
			log.Error("can't set default limits", zap.Error(err))
		}
		s.setNodeUsageInMemory(defaultUsage)
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

func (s *fileSync) getCachedNodeUsage() (NodeUsage, bool, error) {
	s.nodeUsageLock.RLock()
	if s.nodeUsage != nil {
		usage := *s.nodeUsage
		s.nodeUsageLock.RUnlock()
		return usage, true, nil
	}
	s.nodeUsageLock.RUnlock()

	usage, err := s.nodeUsageStore.Get(context.Background(), anystoreprovider.SystemKeys.NodeUsage())
	if errors.Is(err, anystore.ErrDocNotFound) {
		return NodeUsage{}, false, nil
	}
	if err != nil {
		return NodeUsage{}, false, err
	}
	s.setNodeUsageInMemory(usage)
	return usage, true, nil
}

func (s *fileSync) setNodeUsageInMemory(usage NodeUsage) {
	s.nodeUsageLock.Lock()
	s.nodeUsage = &usage
	s.nodeUsageLock.Unlock()
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
	slices.SortFunc(spaces, func(a, b SpaceStat) int {
		return strings.Compare(a.SpaceId, b.SpaceId)
	})
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

	if prevUsage.Equal(usage) {
		return usage, nil
	}
	err = s.nodeUsageStore.Set(context.Background(), anystoreprovider.SystemKeys.NodeUsage(), usage)
	if err != nil {
		return NodeUsage{}, fmt.Errorf("save node usage info to store: %w", err)
	}
	s.setNodeUsageInMemory(usage)

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
	return event.NewEventSingleMessage("", &pb.EventMessageValueOfFileSpaceUsage{
		FileSpaceUsage: &pb.EventFileSpaceUsage{
			BytesUsage: bytesUsage,
			SpaceId:    spaceId,
		},
	})
}

func (s *fileSync) sendLimitUpdatedEvent(limit uint64) {
	s.eventSender.Broadcast(makeLimitUpdatedEvent(limit))
}

func makeLimitUpdatedEvent(limit uint64) *pb.Event {
	return event.NewEventSingleMessage("", &pb.EventMessageValueOfFileLimitUpdated{
		FileLimitUpdated: &pb.EventFileLimitUpdated{
			BytesLimit: limit,
		},
	})
}

func (s *fileSync) DebugQueue(_ *http.Request) (*QueueInfo, error) {
	var info QueueInfo
	return &info, nil
}
