package migration

import (
	"context"
	"sync"

	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

type QueryableStore interface {
	Query(q database.Query) (records []database.Record, err error)
	Lock()
}

type safeStore struct {
	store  objectstore.ObjectStore
	locked bool
	m      sync.RWMutex
}

func (s *safeStore) Query(q database.Query) (records []database.Record, err error) {
	s.m.RLock()
	if s.locked {
		s.m.RUnlock()
		return nil, context.Canceled
	}
	s.m.RUnlock()
	return s.store.Query(q)
}

func (s *safeStore) Lock() {
	s.m.Lock()
	s.locked = true
	s.m.Unlock()
}
