package migration

import (
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

type QueryableStore interface {
	Query(q database.Query) (records []database.Record, err error)
}

type safeStore struct {
	store       objectstore.ObjectStore
	CtxExceeded bool
}

func (s safeStore) Query(q database.Query) (records []database.Record, err error) {
	if s.CtxExceeded {
		return nil, ErrCtxExceeded
	}
	return s.store.Query(q)
}
