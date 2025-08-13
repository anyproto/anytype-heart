package filesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/util/futures"
)

const spaceUsageTTL = 60 * time.Second

// spaceUsage helps to track limits usage for parallel uploading
// TODO Descriptive commentary
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

func newSpaceUsage(ctx context.Context, spaceId string, rpcStore rpcstore.RpcStore) (*spaceUsage, error) {
	s := &spaceUsage{
		spaceId:  spaceId,
		rpcStore: rpcStore,
		files:    make(map[string]allocatedFile),
	}
	return s, s.update(ctx)
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

func (s *spaceUsage) removeFile(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	file, ok := s.files[key]
	if ok {
		s.allocatedUsage -= file.size
		delete(s.files, key)
	}

}

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

type uploadLimitManager struct {
	rpcStore rpcstore.RpcStore

	lock   sync.Mutex
	spaces map[string]*futures.Future[*spaceUsage]
}

func newLimitManager(rpcStore rpcstore.RpcStore) *uploadLimitManager {
	return &uploadLimitManager{
		rpcStore: rpcStore,
		spaces:   make(map[string]*futures.Future[*spaceUsage]),
	}
}

func (m *uploadLimitManager) getSpace(ctx context.Context, spaceId string) (*spaceUsage, error) {
	space, err := m.getSpaceFuture(ctx, spaceId).Wait()
	if err != nil {
		return nil, err
	}
	return space, nil
}

func (m *uploadLimitManager) getSpaceFuture(ctx context.Context, spaceId string) *futures.Future[*spaceUsage] {
	m.lock.Lock()
	space, ok := m.spaces[spaceId]
	if ok {
		m.lock.Unlock()
		return space
	} else {
		space = futures.New[*spaceUsage]()
		m.spaces[spaceId] = space
		m.lock.Unlock()

		space.Resolve(newSpaceUsage(ctx, spaceId, m.rpcStore))
		return space
	}
}
