package filesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
)

const spaceUsageTTL = 60 * time.Second

// spaceUsage helps to track limits usage for parallel uploading. To do that we track sizes of currently uploading files
// and estimate free space using that information.
type spaceUsage struct {
	spaceId string
	lock    sync.Mutex

	rpcStore rpcstore.RpcStore

	limit          int
	usageFromNode  int
	allocatedUsage int
	files          map[string]allocatedFile
	updatedAt      time.Time
}

func newSpaceUsage(spaceId string, rpcStore rpcstore.RpcStore) *spaceUsage {
	s := &spaceUsage{
		spaceId:  spaceId,
		rpcStore: rpcStore,
		files:    make(map[string]allocatedFile),
	}
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
	if time.Since(s.updatedAt) > spaceUsageTTL {
		err := s.update(ctx)
		if err != nil {
			return 0, err
		}
		s.updatedAt = time.Now()
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

// removeFile removes size of given file from total usage
func (s *spaceUsage) removeFile(key string) {
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
	info, err := s.rpcStore.SpaceInfo(ctx, s.spaceId)
	if err != nil {
		return fmt.Errorf("get space info: %w", err)
	}

	s.limit = int(info.LimitBytes)
	s.usageFromNode = int(info.SpaceUsageBytes)

	return nil
}

type allocatedFile struct {
	size int
}
