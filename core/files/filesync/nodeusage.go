package filesync

import (
	"context"
	"errors"
	"fmt"
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
}

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
	newUsageFromNode := int(info.SpaceUsageBytes)

	s.limit = newLimit
	s.usageFromNode = newUsageFromNode
	s.updatedAt = time.Now()

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

type allocatedFile struct {
	size int
}
