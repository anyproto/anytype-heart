package badgerstorage

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/dgraph-io/badger/v4"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

var ErrLocked = errors.New("space storage locked")

type storageService struct {
	keys         storageServiceKeys
	provider     datastore.Datastore
	db           *badger.DB
	lockedSpaces map[string]*lockSpace

	mu sync.Mutex
}

type lockSpace struct {
	ch  chan struct{}
	err error
}

func New() *storageService {
	return &storageService{}
}

func (s *storageService) Init(a *app.App) (err error) {
	s.provider = a.MustComponent(datastore.CName).(datastore.Datastore)
	s.keys = newStorageServiceKeys()

	s.lockedSpaces = map[string]*lockSpace{}
	return
}

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) SpaceStorage(id string) (spacestorage.SpaceStorage, error) {
	return newSpaceStorage(s.db, id, s)
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (store spacestorage.SpaceStorage, err error) {
	var ls *lockSpace
	ls, err = s.checkLock(id, func() error {
		store, err = newSpaceStorage(s.db, id, s)
		return err
	})
	if errors.Is(err, ErrLocked) {
		select {
		case <-ls.ch:
			if ls.err != nil {
				return nil, err
			}
			return s.WaitSpaceStorage(ctx, id)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return
}

func (s *storageService) MarkSpaceCreated(id string) (err error) {
	return badgerhelper.SetValue(s.db, s.keys.SpaceCreatedKey(id), nil)
}

func (s *storageService) UnmarkSpaceCreated(id string) (err error) {
	return badgerhelper.DeleteValue(s.db, s.keys.SpaceCreatedKey(id))
}

func (s *storageService) IsSpaceCreated(id string) (created bool) {
	return hasDB(s.db, s.keys.SpaceCreatedKey(id))
}

func (s *storageService) SpaceExists(id string) bool {
	return s.db.View(func(txn *badger.Txn) error {
		_, err := getTxn(txn, newSpaceKeys(id).HeaderKey())
		if err != nil {
			return err
		}
		return nil
	}) == nil
}

func (s *storageService) checkLock(id string, openFunc func() error) (ls *lockSpace, err error) {
	s.mu.Lock()
	var ok bool
	if ls, ok = s.lockedSpaces[id]; ok {
		s.mu.Unlock()
		return ls, ErrLocked
	}
	ch := make(chan struct{})
	ls = &lockSpace{
		ch: ch,
	}
	s.lockedSpaces[id] = ls
	s.mu.Unlock()
	if err = openFunc(); err != nil {
		s.unlockSpaceStorage(id)
		return nil, err
	}
	return nil, nil
}

func (s *storageService) waitLock(ctx context.Context, id string, action func() error) (err error) {
	var ls *lockSpace
	ls, err = s.checkLock(id, action)
	if errors.Is(err, ErrLocked) {
		select {
		case <-ls.ch:
			if ls.err != nil {
				return ls.err
			}
			return s.waitLock(ctx, id, action)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return
}

func (s *storageService) DeleteSpaceStorage(ctx context.Context, spaceId string) error {
	err := s.waitLock(ctx, spaceId, func() error {
		return s.deleteSpace(spaceId)
	})
	if err == nil {
		s.unlockSpaceStorage(spaceId)
	}
	return err
}

func (s *storageService) deleteSpace(spaceId string) (err error) {
	return deleteSpace(spaceId, s.db)
}

func (s *storageService) unlockSpaceStorage(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ls, ok := s.lockedSpaces[id]; ok {
		close(ls.ch)
		delete(s.lockedSpaces, id)
	}
}

func (s *storageService) CreateSpaceStorage(payload spacestorage.SpaceStorageCreatePayload) (spacestorage.SpaceStorage, error) {
	return createSpaceStorage(s.db, payload, s)
}

func (s *storageService) GetSpaceID(objectID string) (spaceID string, err error) {
	return badgerhelper.GetValue(s.db, s.keys.BindObjectIDKey(objectID), func(bytes []byte) (string, error) {
		return string(bytes), nil
	})
}

func (s *storageService) BindSpaceID(spaceID, objectID string) (err error) {
	return badgerhelper.SetValue(s.db, s.keys.BindObjectIDKey(objectID), []byte(spaceID))
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	err = s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = s.keys.SpacePrefix()

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			id := item.Key()
			if len(id) <= len(s.keys.SpacePrefix())+1 {
				continue
			}
			id = id[len(s.keys.SpacePrefix())+1:]
			ids = append(ids, string(id))
		}
		return nil
	})
	return
}

func (s *storageService) Run(ctx context.Context) (err error) {
	s.db, err = s.provider.SpaceStorage()
	if err != nil {
		return
	}
	return
}

func (s *storageService) Close(ctx context.Context) (err error) {
	return
}
