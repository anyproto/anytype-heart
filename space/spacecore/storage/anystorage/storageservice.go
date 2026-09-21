package anystorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/debug/debugreporter"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/space/spacedomain"
)

const backupSuffix = "_backup"

// CorruptedBackup represents a backup of a corrupted space storage
type CorruptedBackup struct {
	SpaceId    string
	BackupPath string
}

// nolint: unused
var log = logger.NewNamed(spacestorage.CName)

func New(rootPath string, anyStoreConfig *anystore.Config) *storageService {
	return &storageService{
		rootPath: rootPath,
		config:   anyStoreConfig,
	}
}

type storageService struct {
	rootPath string
	config   *anystore.Config
	configMu sync.Mutex

	backupsMu sync.RWMutex
	backups   []CorruptedBackup

	spaceLocksMu sync.Mutex
	spaceLocks   map[string]*spaceLock

	reporter debugreporter.Reporter
}

// spaceLock serializes whoever opens, creates or deletes one space's store.db.
//
// any-store stamps `PRAGMA user_version` only after SQLite has already created
// the file, so for the few milliseconds a create spends inside anystore.Open
// the store on disk reads back as version 0. IsCorruptedError reports that as
// corruption and openDb answers corruption by renaming the space directory --
// which was being done to a store another goroutine was still creating (a LAN
// handshake derives discovery keys over every id AllSpaceIds returns, whether
// or not it is finished). The create then died on its vanished file and the
// account came up with no personal space (GO-7534). Openers of one space now
// queue behind its creator and always see a stamped db.
type spaceLock struct {
	ch   chan struct{}
	refs int
}

// lockSpace waits until no other goroutine holds this space's db, and returns
// the func that releases it. It gives up if ctx is cancelled first.
func (s *storageService) lockSpace(ctx context.Context, id string) (unlock func(), err error) {
	s.spaceLocksMu.Lock()
	if s.spaceLocks == nil {
		s.spaceLocks = make(map[string]*spaceLock)
	}
	l, ok := s.spaceLocks[id]
	if !ok {
		l = &spaceLock{ch: make(chan struct{}, 1)}
		s.spaceLocks[id] = l
	}
	// held while waiting too, so the entry survives until the last waiter is gone
	l.refs++
	s.spaceLocksMu.Unlock()

	release := func() {
		s.spaceLocksMu.Lock()
		defer s.spaceLocksMu.Unlock()
		l.refs--
		if l.refs == 0 {
			delete(s.spaceLocks, id)
		}
	}

	select {
	case l.ch <- struct{}{}:
		return func() {
			<-l.ch
			release()
		}, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
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

	start := time.Now()
	db, err = anystore.Open(ctx, dbPath, s.anyStoreConfig())
	if err != nil {
		code, isCorrupted := anystorehelper.IsCorruptedError(err)
		if isCorrupted {
			log.With(zap.Error(err), zap.String("code", code.String()), zap.String("desc", code.Message())).
				With(zap.Bool("isCorrupted", isCorrupted)).
				With(zap.Int64("tookMs", time.Since(start).Milliseconds())).
				Error("failed to open spacestore, backing up")
			if s.reporter != nil {
				s.reporter.Report("DB_CORRUPTION", map[string]any{
					"db":      filepath.Join(filepath.Base(filepath.Dir(dbPath)), filepath.Base(dbPath)),
					"spaceId": id,
					"code":    code.String(),
					"desc":    code.Message(),
					"error":   err.Error(),
					"tookMs":  time.Since(start).Milliseconds(),
				}, debugreporter.Capture{Kind: debugreporter.KindNone})
			}
			if _, backupErr := s.backupCorruptedSpace(id); backupErr != nil {
				log.With(zap.Error(backupErr)).Error("failed to backup corrupted space")
			}
			return nil, spacestorage.ErrSpaceStorageMissing
		}
		return nil, err
	}
	return db, nil
}

func (s *storageService) createDb(ctx context.Context, id string) (db anystore.DB, err error) {
	dirPath := path.Join(s.rootPath, id)
	err = os.MkdirAll(dirPath, 0755)
	if err != nil {
		return nil, err
	}
	dbPath := path.Join(dirPath, "store.db")
	start := time.Now()
	db, err = anystore.Open(ctx, dbPath, s.anyStoreConfig())
	if err == nil {
		return db, nil
	}
	code, isCorrupted := anystorehelper.IsCorruptedError(err)
	if !isCorrupted {
		return nil, err
	}
	// A create killed before any-store stamped `user_version` leaves a store.db
	// every later open reads as version 0. openDb heals that by moving the dir
	// aside so the space is fetched again, but a derived space (tech, personal)
	// is created rather than fetched: without the same recovery here, one
	// interrupted create left the account unable to bootstrap for good
	// (GO-7534).
	log.With(zap.Error(err), zap.String("spaceId", id), zap.String("code", code.String()), zap.String("desc", code.Message())).
		With(zap.Int64("tookMs", time.Since(start).Milliseconds())).
		Error("failed to open spacestore for create, backing up")
	if s.reporter != nil {
		s.reporter.Report("DB_CORRUPTION", map[string]any{
			"db":      filepath.Join(id, "store.db"),
			"spaceId": id,
			"code":    code.String(),
			"desc":    code.Message(),
			"error":   err.Error(),
			"tookMs":  time.Since(start).Milliseconds(),
		}, debugreporter.Capture{Kind: debugreporter.KindNone})
	}
	if _, backupErr := s.backupCorruptedSpace(id); backupErr != nil {
		return nil, fmt.Errorf("backup unusable space store: %w", backupErr)
	}
	if err = os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("recreate space dir: %w", err)
	}
	db, err = anystore.Open(ctx, dbPath, s.anyStoreConfig())
	if err != nil {
		return nil, fmt.Errorf("open space store after backup: %w", err)
	}
	return db, nil
}

func (s *storageService) Close(ctx context.Context) (err error) {
	return nil
}

func (s *storageService) Init(a *app.App) (err error) {
	// Reporter is optional — see anystoreprovider.Init comment.
	if r, err := app.GetComponent[debugreporter.Reporter](a); err == nil {
		s.reporter = r
	}
	if _, err = os.Stat(s.rootPath); err != nil {
		err = os.MkdirAll(s.rootPath, 0755)
		if err != nil {
			return err
		}
	}
	return s.loadBackups()
}

// loadBackups scans the storage root for existing backup folders
func (s *storageService) loadBackups() error {
	entries, err := os.ReadDir(s.rootPath)
	if err != nil {
		return fmt.Errorf("failed to read storage root: %w", err)
	}

	s.backupsMu.Lock()
	defer s.backupsMu.Unlock()

	s.backups = nil
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		idx := strings.Index(entry.Name(), backupSuffix)
		if idx == -1 {
			continue
		}
		spaceId := entry.Name()[:idx]
		if spaceId == "" {
			continue
		}
		s.backups = append(s.backups, CorruptedBackup{
			SpaceId:    spaceId,
			BackupPath: filepath.Join(s.rootPath, entry.Name()),
		})
	}
	return nil
}

func (s *storageService) Name() (name string) {
	return spacestorage.CName
}

func (s *storageService) WaitSpaceStorage(ctx context.Context, id string) (spacestorage.SpaceStorage, error) {
	unlock, err := s.lockSpace(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("lock space: %w", err)
	}
	defer unlock()

	start := time.Now()
	db, err := s.openDb(ctx, id)
	if err != nil {
		code, isCorrupted := anystorehelper.IsCorruptedError(err)
		log.With(zap.String("spaceId", id), zap.Bool("isCorrupted", isCorrupted), zap.String("code", code.String()), zap.Error(err)).Error("failed to open spacestore")
		return nil, err
	}
	if time.Since(start) > time.Second {
		ctxStat, cancel := context.WithTimeout(ctx, time.Second*2)
		defer cancel()

		logger := log.With(zap.String("spaceId", id)).With(zap.Int64("tookMs", time.Since(start).Milliseconds()))
		stat, err := db.Stats(ctxStat)
		if err != nil {
			logger = logger.With(zap.Error(err))
		} else {
			logger = logger.With(anystorehelper.DbStatToZapFields(stat)...)
		}
		logger.Warn("spacestore db open took too long")
	}
	st, err := spacestorage.New(ctx, id, db)
	if err != nil {
		return nil, s.handleStorageBuildError(id, db, err)
	}
	cs, err := NewClientStorage(ctx, st)
	if err != nil {
		return nil, s.handleStorageBuildError(id, db, err)
	}
	return cs, nil
}

// handleStorageBuildError converts a build failure on an openable store.db into
// ErrSpaceStorageMissing when the db is valid but uninitialized (its mandatory
// collections were never durably created, e.g. a create interrupted by process
// kill or power loss). The db passes every corruption check, so without this the
// space can never load and never self-heal (GO-7393). The db dir is backed up
// and the caller re-downloads the space from peers.
func (s *storageService) handleStorageBuildError(id string, db anystore.DB, err error) error {
	if !errors.Is(err, anystore.ErrCollectionNotFound) {
		_ = db.Close()
		return err
	}
	log.With(zap.String("spaceId", id), zap.Error(err)).Error("space store is uninitialized, backing up for re-download")
	if s.reporter != nil {
		s.reporter.Report("DB_UNINITIALIZED", map[string]any{
			"db":      filepath.Join(id, "store.db"),
			"spaceId": id,
			"error":   err.Error(),
		}, debugreporter.Capture{Kind: debugreporter.KindNone})
	}
	_ = db.Close()
	if _, backupErr := s.backupCorruptedSpace(id); backupErr != nil {
		log.With(zap.String("spaceId", id), zap.Error(backupErr)).Error("failed to backup uninitialized space")
	}
	return spacestorage.ErrSpaceStorageMissing
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
	if err := validateSpaceType(payload.SpaceHeaderWithId); err != nil {
		return nil, err
	}
	unlock, err := s.lockSpace(ctx, payload.SpaceHeaderWithId.Id)
	if err != nil {
		return nil, fmt.Errorf("lock space: %w", err)
	}
	defer unlock()

	db, err := s.createDb(ctx, payload.SpaceHeaderWithId.Id)
	if err != nil {
		err = fmt.Errorf("failed to create db: %w", err)
		return nil, err
	}
	st, err := spacestorage.Create(ctx, db, payload)
	if err != nil {
		err = fmt.Errorf("failed to create spacestorage: %w", err)
		return nil, err
	}
	return NewClientStorage(ctx, st)
}

func (s *storageService) DeleteSpaceStorage(ctx context.Context, spaceId string) error {
	unlock, err := s.lockSpace(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("lock space: %w", err)
	}
	defer unlock()

	dbPath := path.Join(s.rootPath, spaceId)
	return os.RemoveAll(dbPath)
}

func (s *storageService) anyStoreConfig() *anystore.Config {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	opts := maps.Clone(s.config.SQLiteConnectionOptions)
	if opts == nil {
		opts = make(map[string]string)
	}
	opts["synchronous"] = "normal"
	opts["wal_autocheckpoint"] = "10000"
	anystorehelper.ApplyPlatformPragmas(opts)

	return &anystore.Config{
		ReadConnections:                           4,
		SQLiteConnectionOptions:                   opts,
		SQLiteGlobalPageCachePreallocateSizeBytes: 1 << 26,

		StalledConnectionsPanicOnClose:    time.Second * 45,
		StalledConnectionsDetectorEnabled: true,
		Durability: anystore.DurabilityConfig{
			AutoFlush: true,
			IdleAfter: time.Second * 20,
			FlushMode: anystore.FlushModeCheckpointPassive,
			Sentinel:  true,
		},
	}
}

func validateSpaceType(headerWithId *spacesyncproto.RawSpaceHeaderWithId) error {
	var rawHeader = &spacesyncproto.RawSpaceHeader{}
	if err := rawHeader.UnmarshalVT(headerWithId.RawHeader); err != nil {
		return err
	}

	var header = &spacesyncproto.SpaceHeader{}
	if err := header.UnmarshalVT(rawHeader.SpaceHeader); err != nil {
		return err
	}

	switch spacedomain.SpaceType(header.SpaceType) {
	case "":
	case spacedomain.SpaceTypeTech:
	case spacedomain.SpaceTypeRegular:
	case spacedomain.SpaceTypeChat:
	case spacedomain.SpaceTypeOneToOne:
	default:
		return fmt.Errorf("%w: type: %v", spacedomain.ErrUnexpectedSpaceType, header.SpaceType)
	}
	return nil
}

// backupCorruptedSpace renames the corrupted space folder to {spaceId}_backup_{timestamp}
func (s *storageService) backupCorruptedSpace(spaceId string) (string, error) {
	srcPath := filepath.Join(s.rootPath, spaceId)
	timestamp := time.Now().Unix()
	backupPath := filepath.Join(s.rootPath, fmt.Sprintf("%s%s%d", spaceId, backupSuffix, timestamp))

	if err := os.Rename(srcPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to rename corrupted space: %w", err)
	}

	// Add to cached list
	s.backupsMu.Lock()
	s.backups = append(s.backups, CorruptedBackup{
		SpaceId:    spaceId,
		BackupPath: backupPath,
	})
	s.backupsMu.Unlock()

	return backupPath, nil
}

// ListCorruptedBackups returns the cached list of corrupted backup folders
func (s *storageService) ListCorruptedBackups() []CorruptedBackup {
	s.backupsMu.RLock()
	defer s.backupsMu.RUnlock()
	return s.backups
}

// DeleteBackup removes a corrupted backup folder after validating the path
func (s *storageService) DeleteBackup(backupPath string) error {
	absRoot, err := filepath.Abs(s.rootPath)
	if err != nil {
		return fmt.Errorf("failed to resolve root path: %w", err)
	}
	absBackup, err := filepath.Abs(backupPath)
	if err != nil {
		return fmt.Errorf("failed to resolve backup path: %w", err)
	}

	// Security: ensure backup path is within storage root
	if !strings.HasPrefix(absBackup, absRoot+string(filepath.Separator)) {
		return fmt.Errorf("invalid backup path: outside storage root")
	}

	// Ensure this is actually a backup folder (contains _backup_ pattern)
	if !strings.Contains(filepath.Base(backupPath), backupSuffix) {
		return fmt.Errorf("invalid backup path: not a backup folder")
	}

	if err := os.RemoveAll(backupPath); err != nil {
		return err
	}

	// Remove from cached list
	s.backupsMu.Lock()
	for i, b := range s.backups {
		if b.BackupPath == backupPath {
			s.backups = append(s.backups[:i], s.backups[i+1:]...)
			break
		}
	}
	s.backupsMu.Unlock()

	return nil
}
