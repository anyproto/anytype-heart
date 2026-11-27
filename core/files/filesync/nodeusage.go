package filesync

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
)

const spaceUsageTTL = 30 * time.Second

// spaceUsage helps to track limits usage for parallel uploading. To do that we track sizes of currently uploading files
// and estimate free space using that information.
type spaceUsage struct {
	ctx     context.Context
	spaceId string
	lock    sync.Mutex

	updateCh chan<- updateMessage
	rpcStore rpcstore.RpcStore

	limit          int
	usageFromNode  int
	allocatedUsage int
	files          map[string]allocatedFile
	updatedAt      time.Time

	// report is the node's last answer and the uploads confirmed since, for
	// reported. It has a lock of its own: lock is held across node calls, and
	// a reader must not wait for the node. Writers take it after lock, briefly;
	// reported takes it alone and does the copying, so a burst of
	// confirmations costs the uploader nothing per reader. Its counts are
	// 64-bit, from the node's answer, so a limit above 2 GiB survives a 32-bit
	// build.
	reportLock    sync.Mutex
	reportedLimit int64
	reportedUsage int64
	reportedAt    time.Time
	confirmed     map[string]confirmedEntry
}

// confirmedEntry is a confirmed upload, kept for a while past the first
// report that includes it: the store can still list its file as waiting
// then, and a reader must not count it twice.
type confirmedEntry struct {
	bytes int64
	// inReportAt is when the first report including the upload came in;
	// zero until one has
	inReportAt time.Time
}

// confirmedRetention is how long a confirmed upload is kept past the first
// report that includes it.
const confirmedRetention = 10 * time.Minute

func newSpaceUsage(ctx context.Context, spaceId string, rpcStore rpcstore.RpcStore, updateCh chan<- updateMessage) *spaceUsage {
	s := &spaceUsage{
		ctx:      ctx,
		spaceId:  spaceId,
		rpcStore: rpcStore,
		updateCh: updateCh,
		files:    make(map[string]allocatedFile),
	}

	go func() {
		update := func() {
			err := s.Update(ctx)
			if err != nil && !errors.Is(err, fileprotoerr.ErrForbidden) {
				log.Error("update space usage in background", zap.Error(err), zap.String("spaceId", s.spaceId))
			}
		}

		update()
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				update()
			}
		}
	}()

	return s
}

func (s *spaceUsage) getLimit() int {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.limit
}

// reported returns the limit and usage as the node last reported them, and
// the uploads confirmed lately, in a copy of its own. ok is false until the
// node has answered once. It never waits for lock, which a node call can
// hold, nor for the node.
func (s *spaceUsage) reported() (limits SpaceLimits, ok bool) {
	s.reportLock.Lock()
	defer s.reportLock.Unlock()
	if s.reportedAt.IsZero() {
		return SpaceLimits{}, false
	}
	confirmed := make(map[string]ConfirmedUpload, len(s.confirmed))
	for key, entry := range s.confirmed {
		confirmed[key] = ConfirmedUpload{Bytes: entry.bytes, InReport: !entry.inReportAt.IsZero()}
	}
	return SpaceLimits{
		LimitBytes:     s.reportedLimit,
		NodeUsageBytes: s.reportedUsage,
		Confirmed:      confirmed,
		ReportedAt:     s.reportedAt,
	}, true
}

func (s *spaceUsage) getTotalUsage() int {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.usageFromNode + s.allocatedUsage
}

func (s *spaceUsage) getFreeSpace(ctx context.Context) (int, error) {
	err := s.update(ctx)
	if err != nil {
		return 0, err
	}
	return s.limit - s.usageFromNode - s.allocatedUsage, nil
}

// allocateFile tries to add size of given file to total usage and returns error if there is no free space
func (s *spaceUsage) allocateFile(ctx context.Context, key string, size int) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	_, ok := s.files[key]
	if ok {
		return nil
	}

	free, err := s.getFreeSpace(ctx)
	if err != nil {
		return fmt.Errorf("get free space: %w", err)
	}
	if free < size {
		return &errLimitReached{
			fileSize:        size,
			accountLimit:    s.limit,
			totalBytesUsage: s.usageFromNode + s.allocatedUsage,
		}
	}

	s.files[key] = allocatedFile{
		size: size,
	}

	s.allocatedUsage += size
	return nil
}

// deallocateFile removes size of given file from total usage
func (s *spaceUsage) deallocateFile(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	file, ok := s.files[key]
	if ok {
		s.allocatedUsage -= file.size
		delete(s.files, key)
	}

}

// markFileUploaded removes size of given file from total local usage and adds this size to estimated node usage counter.
// In order to keep node usage counter consistent we update it from remote node.
func (s *spaceUsage) markFileUploaded(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	file, ok := s.files[key]
	if ok {
		s.allocatedUsage -= file.size
		// Update usage from node locally as best-effort solution. When we receive real node usage
		// this value will be reconciled
		s.usageFromNode += file.size
		delete(s.files, key)
		s.reportLock.Lock()
		if s.confirmed == nil {
			s.confirmed = map[string]confirmedEntry{}
		}
		// a confirmation after a report is not in it, whatever an earlier
		// confirmation of the same key was
		s.confirmed[key] = confirmedEntry{bytes: s.confirmed[key].bytes + int64(file.size)}
		s.reportLock.Unlock()
	}
}

func (s *spaceUsage) Update(ctx context.Context) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.update(ctx)
}

func (s *spaceUsage) update(ctx context.Context) error {
	if time.Since(s.updatedAt) < spaceUsageTTL {
		return nil
	}

	info, err := s.rpcStore.SpaceInfo(ctx, s.spaceId)
	if err != nil {
		return fmt.Errorf("get space info: %w", err)
	}

	newLimit := int(info.LimitBytes)
	newUsageFromNode := int(info.TotalUsageBytes)

	s.limit = newLimit
	s.usageFromNode = newUsageFromNode
	s.updatedAt = time.Now()
	// the node's answer includes what was confirmed before it; those stay
	// listed a while past the first answer that does, marked, for a reader
	// that still sees their files waiting
	s.reportLock.Lock()
	s.reportedLimit = clampInt64(info.LimitBytes)
	s.reportedUsage = clampInt64(info.TotalUsageBytes)
	s.reportedAt = s.updatedAt
	for key, entry := range s.confirmed {
		switch {
		case entry.inReportAt.IsZero():
			entry.inReportAt = s.updatedAt
			s.confirmed[key] = entry
		case s.updatedAt.Sub(entry.inReportAt) > confirmedRetention:
			delete(s.confirmed, key)
		}
	}
	s.reportLock.Unlock()

	s.sendUpdate()

	return nil
}

func (s *spaceUsage) sendUpdate() {
	msg := updateMessage{
		spaceId: s.spaceId,
		limit:   s.limit,
		usage:   s.usageFromNode,
	}

	select {
	case s.updateCh <- msg:
	default:
	}
}

// clampInt64 converts a byte count from the node, saturating at the int64
// maximum.
func clampInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

type allocatedFile struct {
	size int
}
