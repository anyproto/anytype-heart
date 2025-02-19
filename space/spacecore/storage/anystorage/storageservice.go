package anystorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// nolint: unused
var log = logger.NewNamed(spacestorage.CName)

func New(rootPath string) *storageService {
	return &storageService{
		rootPath: rootPath,
	}
}

type checkpointStore struct {
	anystore.DB
	checkpointAfterWrite time.Duration
	checkpointForce      time.Duration
	lastWrite            atomic.Time
	lastCheckpoint       atomic.Time
	ctx                  context.Context
	cancel               context.CancelFunc
}

func newCheckpointStore(db anystore.DB) *checkpointStore {
	ctx, cancel := context.WithCancel(context.Background())
	s := &checkpointStore{
		DB:                   db,
		checkpointAfterWrite: time.Second,
		checkpointForce:      time.Second * 10,
		ctx:                  ctx,
		cancel:               cancel,
	}
	go s.run()
	return s
}

func (s *checkpointStore) needCheckpoint() bool {
	now := time.Now()
	lastWrite := s.lastWrite.Load()
	lastCheckpoint := s.lastCheckpoint.Load()

	if lastCheckpoint.Before(lastWrite) && now.Sub(lastWrite) > s.checkpointAfterWrite {
		return true
	}

	if now.Sub(lastCheckpoint) > s.checkpointForce {
		return true
	}
	return false
}

func (s *checkpointStore) SetLastWrite() {
	s.lastWrite.Store(time.Now())
}

func (s *checkpointStore) Close() error {
	s.cancel()
	return s.DB.Close()
}

func (s *checkpointStore) run() {
	for {
		select {
		case <-time.After(s.checkpointAfterWrite):
		case <-s.ctx.Done():
			return
		}
		if s.needCheckpoint() {
			st := time.Now()
			if err := s.DB.Checkpoint(s.ctx, false); err != nil {
				log.Warn("checkpoint error", zap.Error(err))
			}
			log.Debug("checkpoint", zap.Duration("dur", time.Since(st)))
		}
	}
}

type storageService struct {
	rootPath string
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	var files []string
	fileInfo, err := os.ReadDir(s.rootPath)
	if err != nil {
		return files, fmt.Errorf("can't read datadir '%v': %w", s.rootPath, err)
	}
	for _, file := range fileInfo {
		if !strings.HasPrefix(file.Name(), ".") {
			files = append(files, file.Name())
		}
	}
	return files, nil
}

func (s *storageService) Run(ctx context.Context) (err error) {
	return nil
}

func (s *storageService) openDb(ctx context.Context, id string) (db anystore.DB, err error) {
	dbPath := path.Join(s.rootPath, id, "store.db")
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, spacestorage.ErrSpaceStorageMissing
		}
		return nil, err
	}
	db, err = anystore.Open(ctx, dbPath, anyStoreConfig())
	if err != nil {
		return
	}
	return newCheckpointStore(db), nil
}

func (s *storageService) createDb(ctx context.Context, id string) (db anystore.DB, err error) {
	dirPath := path.Join(s.rootPath, id)
	err = os.MkdirAll(dirPath, 0755)
	if err != nil {
		return nil, err
	}
	dbPath := path.Join(dirPath, "store.db")
	db, err = anystore.Open(ctx, dbPath, anyStoreConfig())
	if err != nil {
		return
	}
	return newCheckpointStore(db), nil
}

func (s *storageService) Close(ctx context.Context) (err error) {
	return nil
}

func (s *storageService) Init(a *app.App) (err error) {
	if _, err = os.Stat(s.rootPath); err != nil {
		err = os.MkdirAll(s.rootPath, 0755)
		if err != nil {
			return err
		}
	}
	return nil
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
	dbPath := path.Join(s.rootPath, spaceId)
	return os.RemoveAll(dbPath)
}

func anyStoreConfig() *anystore.Config {
	return &anystore.Config{
		ReadConnections: 4,
		SQLiteConnectionOptions: map[string]string{
			"synchronous": "off",
		},
	}
}
