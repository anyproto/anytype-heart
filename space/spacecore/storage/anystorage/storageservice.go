package anystorage

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
)

// nolint: unused
var log = logger.NewNamed(spacestorage.CName)

func New(rootPath string) *storageService {
	return &storageService{
		rootPath: rootPath,
	}
}

type storageService struct {
	rootPath string
	store    anystore.DB
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	collNames, err := s.store.GetCollectionNames(context.Background())
	if err != nil {
		return nil, err
	}
	for _, collName := range collNames {
		if strings.Contains(collName, objecttree.CollName) {
			split := strings.Split(collName, "-")
			ids = append(ids, split[0])
		}
	}
	return
}

func (s *storageService) checkpointLoop() {
	for {
		select {
		case <-time.After(time.Second):
		}
		if s.store != nil {
			s.store.Checkpoint(context.Background(), false)
		}
	}
}

func (s *storageService) Run(ctx context.Context) (err error) {
	go s.checkpointLoop()
	return nil
}

func (s *storageService) openDb(ctx context.Context, id string) (db anystore.DB, err error) {
	return s.store, nil
}

func (s *storageService) createDb(ctx context.Context, id string) (db anystore.DB, err error) {
	return s.store, nil
}

func (s *storageService) Close(ctx context.Context) (err error) {
	if s.store == nil {
		return nil
	}
	return s.store.Close()
}

func (s *storageService) Init(a *app.App) (err error) {
	if _, err = os.Stat(s.rootPath); err != nil {
		err = os.MkdirAll(s.rootPath, 0755)
		if err != nil {
			return err
		}
	}
	path := path.Join(s.rootPath, "store.db")
	s.store, err = anystore.Open(context.Background(), path, anyStoreConfig())
	return
}

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (spacestorage.SpaceStorage, error) {
	db, err := s.openDb(ctx, id)
	if err != nil {
		return nil, err
	}
	st, err := spacestorage.New(ctx, id, db)
	if err != nil {
		if errors.Is(err, anystore.ErrCollectionNotFound) {
			return nil, spacestorage.ErrSpaceStorageMissing
		}
		return nil, err
	}
	return NewClientStorage(ctx, st)
}

func (s *storageService) SpaceExists(id string) bool {
	if id == "" {
		return false
	}
	dbPath := path.Join(s.rootPath, id)
	if _, err := os.Stat(dbPath); err != nil {
		return false
	}
	return true
}

func (s *storageService) CreateSpaceStorage(ctx context.Context, payload spacestorage.SpaceStorageCreatePayload) (spacestorage.SpaceStorage, error) {
	db, err := s.createDb(ctx, payload.SpaceHeaderWithId.Id)
	if err != nil {
		return nil, err
	}
	st, err := spacestorage.Create(ctx, db, payload)
	if err != nil {
		return nil, err
	}
	return NewClientStorage(ctx, st)
}

func (s *storageService) DeleteSpaceStorage(ctx context.Context, spaceId string) error {
	collNames, err := s.store.GetCollectionNames(context.Background())
	if err != nil {
		return err
	}
	for _, collName := range collNames {
		if strings.Contains(collName, spaceId) {
			coll, err := s.store.OpenCollection(context.Background(), collName)
			if err == nil {
				err := coll.Drop(ctx)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func anyStoreConfig() *anystore.Config {
	return &anystore.Config{
		ReadConnections: 4,
		SQLiteConnectionOptions: map[string]string{
			"synchronous": "off",
			"temp_store":  "1",
			"cache_size":  "-1024",
		},
	}
}
