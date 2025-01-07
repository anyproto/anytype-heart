package anystorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/app/ocache"
)

var log = logger.NewNamed(spacestorage.CName)

var (
	ErrClosed  = errors.New("space storage closed")
	ErrDeleted = errors.New("space storage deleted")
)

type configGetter interface {
	GetSpaceStorePath() string
	GetTempDirPath() string
}

type optKey int

const (
	createKeyVal optKey = 0
	doKeyVal     optKey = 1
)

type doFunc func() error

type storageContainer struct {
	db        anystore.DB
	mx        sync.Mutex
	path      string
	handlers  int
	isClosing bool
	onClose   func(path string) error
	closeCh   chan struct{}
}

func newStorageContainer(db anystore.DB, path string) *storageContainer {
	return &storageContainer{
		db:      db,
		closeCh: make(chan struct{}),
	}
}

func (s *storageContainer) Close() (err error) {
	return fmt.Errorf("should not be called directly")
}

func (s *storageContainer) Acquire() (anystore.DB, error) {
	s.mx.Lock()
	if s.isClosing {
		ch := s.closeCh
		s.mx.Unlock()
		<-ch
		return nil, ErrClosed
	}
	s.handlers++
	s.mx.Unlock()
	return s.db, nil
}

func (s *storageContainer) Release() {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.handlers++
}

func (s *storageContainer) TryClose(objectTTL time.Duration) (res bool, err error) {
	s.mx.Lock()
	if s.handlers > 0 {
		s.mx.Unlock()
		return false, nil
	}
	s.isClosing = true
	s.closeCh = make(chan struct{})
	ch := s.closeCh
	onClose := s.onClose
	db := s.db
	s.mx.Unlock()
	if db != nil {
		if err := db.Close(); err != nil {
			log.Warn("failed to close db", zap.Error(err))
		}
	}
	if onClose != nil {
		err := onClose(s.path)
		if err != nil {
			log.Warn("failed to close db", zap.Error(err))
		}
	}
	close(ch)
	return true, nil
}

func New(rootPath string) *storageService {
	return &storageService{
		rootPath: rootPath,
	}
}

type storageService struct {
	rootPath string
	cache    ocache.OCache
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	var files []string
	fileInfo, err := os.ReadDir(s.rootPath)
	if err != nil {
		return files, fmt.Errorf("can't read datadir '%v': %v", s.rootPath, err)
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
	// TODO: [storage] set anystore config from config
	dbPath := path.Join(s.rootPath, id)
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, spacestorage.ErrSpaceStorageMissing
		}
		return nil, err
	}
	return anystore.Open(ctx, dbPath, anyStoreConfig)
}

func (s *storageService) createDb(ctx context.Context, id string) (db anystore.DB, err error) {
	dbPath := path.Join(s.rootPath, id)
	return anystore.Open(ctx, dbPath, anyStoreConfig)
}

func (s *storageService) Close(ctx context.Context) (err error) {
	return unix.Rmdir(s.rootPath)
}

func (s *storageService) Init(a *app.App) (err error) {
	s.cache = ocache.New(s.loadFunc,
		ocache.WithLogger(log.Sugar()),
		ocache.WithGCPeriod(time.Minute),
		ocache.WithTTL(60*time.Second))
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

func (s *storageService) loadFunc(ctx context.Context, id string) (value ocache.Object, err error) {
	if fn, ok := ctx.Value(doKeyVal).(doFunc); ok {
		err := fn()
		if err != nil {
			return nil, err
		}
		// continue to open
	} else if ctx.Value(createKeyVal) != nil {
		if s.SpaceExists(id) {
			return nil, spacestorage.ErrSpaceStorageExists
		}
		db, err := s.createDb(ctx, id)
		if err != nil {
			return nil, err
		}
		cont := &storageContainer{
			path: path.Join(s.rootPath, id),
			db:   db,
		}
		return cont, nil
	}
	db, err := s.openDb(ctx, id)
	if err != nil {
		return nil, err
	}
	return newStorageContainer(db, path.Join(s.rootPath, id)), nil
}

func (s *storageService) get(ctx context.Context, id string) (container *storageContainer, err error) {
	cont, err := s.cache.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return cont.(*storageContainer), nil
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (spacestorage.SpaceStorage, error) {
	cont, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	db, err := cont.Acquire()
	if err != nil {
		return nil, err
	}
	st, err := spacestorage.New(ctx, id, db)
	if err != nil {
		return nil, err
	}
	return newClientStorage(ctx, cont, st)
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
	ctx = context.WithValue(ctx, createKeyVal, true)
	cont, err := s.get(ctx, payload.SpaceHeaderWithId.Id)
	if err != nil {
		return nil, err
	}
	db, err := cont.Acquire()
	if err != nil {
		return nil, err
	}
	st, err := spacestorage.Create(ctx, db, payload)
	if err != nil {
		return nil, err
	}
	return newClientStorage(ctx, cont, st)
}

func (s *storageService) TryLockAndDo(ctx context.Context, spaceId string, do func() error) (err error) {
	ctx = context.WithValue(ctx, doKeyVal, do)
	_, err = s.get(ctx, spaceId)
	return err
}

func (s *storageService) DeleteSpaceStorage(ctx context.Context, spaceId string) error {
	dbPath := path.Join(s.rootPath, spaceId)
	del := func(path string) error {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("can't delete datadir '%s': %w", path, err)
		}
		return os.RemoveAll(path)
	}
	ctx = context.WithValue(ctx, doKeyVal, func() error {
		err := del(dbPath)
		if err != nil {
			return err
		}
		return ErrDeleted
	})
	cont, err := s.get(ctx, spaceId)
	if err != nil {
		if errors.Is(err, ErrDeleted) || errors.Is(err, spacestorage.ErrSpaceStorageMissing) {
			return nil
		}
		return err
	}
	_, err = cont.Acquire()
	if err != nil {
		return s.DeleteSpaceStorage(ctx, spaceId)
	}
	cont.mx.Lock()
	cont.onClose = del
	ch := cont.closeCh
	cont.mx.Unlock()
	cont.Release()
	<-ch
	return nil
}

var anyStoreConfig *anystore.Config = &anystore.Config{
	ReadConnections: 4,
	SQLiteConnectionOptions: map[string]string{
		"synchronous": "off",
	},
}
