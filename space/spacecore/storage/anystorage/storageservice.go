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
	"github.com/anyproto/go-sqlite"
	"github.com/anyproto/go-sqlite/sqlitex"
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
	spaceLocks   [spaceLockShards]chan struct{}

	reporter debugreporter.Reporter
}

// spaceLockShards is how many locks space ids are spread over. Keying the locks
// by id instead would mean a map an untrusted peer can grow without bound:
// SpacePush hands a remote id to NewSpace, which reaches WaitSpaceStorage
// before the payload is validated, so a rejected push would still leave its
// entry behind.
//
// Two spaces that land on one shard queue behind each other for the length of
// one open, which is microseconds unless the db is dirty and any-store runs its
// quick check -- and a caller that will not wait that long has its ctx. Nothing
// takes a second space lock while holding one (openDb, createDb and
// handleStorageBuildError are the only service calls inside the critical
// section, and none re-enters), so a shared shard can never deadlock, only
// queue. The count is well past the handful of spaces that open at once --
// deferred loads run at preloadConcurrency, which is 2 -- because the array
// costs one pointer per shard and the channels are made on first use.
const spaceLockShards = 4096

// lockSpace serializes whoever opens, creates or deletes one space's store.db,
// and returns the func that releases it.
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
//
// The channel is a mutex a context can wait on, which sync.Mutex is not: a
// discovery-key derive queued behind a slow open has to give up when its
// budget runs out.
func (s *storageService) lockSpace(ctx context.Context, id string) (unlock func(), err error) {
	l := s.spaceLockFor(id)
	select {
	case l <- struct{}{}:
		// released once: a second call would hand the lock to nobody and let
		// the caller after that run alongside whoever holds it now
		var once sync.Once
		return func() { once.Do(func() { <-l }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *storageService) spaceLockFor(id string) chan struct{} {
	idx := spaceLockShard(id)
	s.spaceLocksMu.Lock()
	defer s.spaceLocksMu.Unlock()
	if s.spaceLocks[idx] == nil {
		s.spaceLocks[idx] = make(chan struct{}, 1)
	}
	return s.spaceLocks[idx]
}

// spaceLockShard is FNV-1a over the id, written out so it neither allocates nor
// pulls in a hash for something this small.
func spaceLockShard(id string) int {
	const (
		offsetBasis = 14695981039346656037
		prime       = 1099511628211
	)
	var h uint64 = offsetBasis
	for i := 0; i < len(id); i++ {
		h ^= uint64(id[i])
		h *= prime
	}
	return int(h % spaceLockShards)
}

// preserveOrphanWal moves a space dir aside when its store.db has been
// truncated away while the WAL beside it still holds data. SQLite drops the WAL
// of a zero-page main file as soon as the db is opened, so opening first would
// destroy what can be the only copy of changes no peer can give back; the
// rename keeps the pair together for recovery. A healthy store never looks like
// this -- its main file carries a page from the moment WAL mode is set.
func (s *storageService) preserveOrphanWal(id, dbPath string) (preserved bool) {
	main, err := os.Stat(dbPath)
	if err != nil || main.Size() > 0 {
		return false
	}
	wal, err := os.Stat(dbPath + "-wal")
	if err != nil || wal.Size() == 0 {
		return false
	}
	log.With(zap.String("spaceId", id), zap.Int64("walSize", wal.Size())).
		Error("space store is empty but its wal is not, backing up before the wal is dropped")
	if s.reporter != nil {
		s.reporter.Report("DB_ORPHAN_WAL", map[string]any{
			"db":      filepath.Join(id, "store.db"),
			"spaceId": id,
			"walSize": wal.Size(),
		}, debugreporter.Capture{Kind: debugreporter.KindNone})
	}
	if _, backupErr := s.backupCorruptedSpace(id); backupErr != nil {
		log.With(zap.String("spaceId", id), zap.Error(backupErr)).Error("failed to back up space store with an orphan wal")
		return false
	}
	return true
}

func (s *storageService) AllSpaceIds() (ids []string, err error) {
	var files []string
	fileInfo, err := os.ReadDir(s.rootPath)
	if err != nil {
		return files, fmt.Errorf("can't read datadir '%v': %w", s.rootPath, err)
	}
	for _, file := range fileInfo {
		if strings.HasPrefix(file.Name(), ".") {
			continue
		}
		// a backup is not a space: handing it to discovery makes it open the
		// dir, hit the very error that produced it, and back the backup up
		// again -- orphaning the recovery path already recorded for the space
		if strings.Contains(file.Name(), backupSuffix) {
			continue
		}
		files = append(files, file.Name())
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

	if s.preserveOrphanWal(id, dbPath) {
		return nil, spacestorage.ErrSpaceStorageMissing
	}

	start := time.Now()
	db, err = anystore.Open(ctx, dbPath, s.anyStoreConfig())
	if err != nil {
		reason, proven := provenUnusable(ctx, err, dbPath)
		if !proven {
			return nil, fmt.Errorf("open space store: %w", err)
		}
		code, _ := anystorehelper.IsCorruptedError(err)
		log.With(zap.Error(err), zap.String("spaceId", id), zap.String("reason", reason)).
			With(zap.String("code", code.String()), zap.String("desc", code.Message())).
			With(zap.Int64("tookMs", time.Since(start).Milliseconds())).
			Error("failed to open spacestore, backing up")
		if s.reporter != nil {
			s.reporter.Report(reason, map[string]any{
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
	return db, nil
}

// provenUnusable reports whether a failed open proves the store is no good to
// anyone, which is the only thing that justifies moving a space directory
// aside. IsCorruptedError alone does not: it takes ErrQuickCheckFailed, which
// any-store returns for every error the check hit -- the caller's own ctx
// running out included, and the discovery-key derive hands us a 10s one for
// every space on disk -- and ErrIncompatibleVersion, which is any version
// mismatch, a populated store written by a newer build included. Neither says
// anything about the store, so neither may cost the user their data.
func provenUnusable(ctx context.Context, err error, dbPath string) (reason string, proven bool) {
	// the caller gave up, or SQLite was interrupted on its way out: an
	// interrupted step surfaces with no context error anywhere in the chain
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		sqlite.ErrCode(err) == sqlite.ResultInterrupt {
		return "", false
	}
	if errors.Is(err, anystore.ErrIncompatibleVersion) {
		if storeIsUninitialized(dbPath) {
			return "DB_UNINITIALIZED", true
		}
		return "", false
	}
	if _, isCorrupted := anystorehelper.IsCorruptedError(err); isCorrupted {
		return "DB_CORRUPTION", true
	}
	return "", false
}

func (s *storageService) createDb(ctx context.Context, id string) (db anystore.DB, err error) {
	dirPath := path.Join(s.rootPath, id)
	err = os.MkdirAll(dirPath, 0755)
	if err != nil {
		return nil, err
	}
	dbPath := path.Join(dirPath, "store.db")
	if s.preserveOrphanWal(id, dbPath) {
		if err = os.MkdirAll(dirPath, 0755); err != nil {
			return nil, fmt.Errorf("recreate space dir: %w", err)
		}
	}

	start := time.Now()
	db, err = anystore.Open(ctx, dbPath, s.anyStoreConfig())
	if err == nil {
		return db, nil
	}
	// Only a store that was created and never initialized is moved aside. Any
	// other failure keeps the directory where it is: IsCorruptedError also
	// covers ErrQuickCheckFailed, which any-store returns for any error the
	// check hit including a cancelled context, and ErrIncompatibleVersion is
	// any version mismatch, not only the unstamped 0 -- renaming on those
	// would orphan a populated store whose changes no peer can give back.
	if reason, proven := provenUnusable(ctx, err, dbPath); !proven || reason != "DB_UNINITIALIZED" {
		return nil, fmt.Errorf("open space store: %w", err)
	}
	// A create killed before any-store stamped `user_version` leaves a store.db
	// every later open reads as version 0. openDb heals that by moving the dir
	// aside so the space is fetched again, but a space reached through Derive
	// (the tech space on account create, and the old-account tech-space
	// recovery) is created rather than fetched: without the same recovery here,
	// retrying a bootstrap that died mid-create could never get past it
	// (GO-7534).
	log.With(zap.Error(err), zap.String("spaceId", id)).
		With(zap.Int64("tookMs", time.Since(start).Milliseconds())).
		Error("space store was never initialized, backing up for a fresh create")
	if s.reporter != nil {
		s.reporter.Report("DB_UNINITIALIZED", map[string]any{
			"db":      filepath.Join(id, "store.db"),
			"spaceId": id,
			"error":   err.Error(),
			"tookMs":  time.Since(start).Milliseconds(),
		}, debugreporter.Capture{Kind: debugreporter.KindNone})
	}
	if _, backupErr := s.backupCorruptedSpace(id); backupErr != nil {
		return nil, fmt.Errorf("backup uninitialized space store: %w", backupErr)
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

// storeIsUninitialized reports whether dbPath holds a database that was created
// but never initialized -- a create killed before any-store stamped
// `user_version` and wrote its tables. Anything it cannot prove empty is
// false, so an unreadable or populated store is never moved aside.
func storeIsUninitialized(dbPath string) bool {
	conn, err := sqlite.OpenConn(dbPath, sqlite.OpenReadOnly|sqlite.OpenWAL|sqlite.OpenURI)
	if err != nil {
		return false
	}
	defer func() {
		_ = conn.Close()
	}()
	var version, objects int
	err = sqlitex.ExecuteTransient(conn, "PRAGMA user_version", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			version = stmt.ColumnInt(0)
			return nil
		},
	})
	if err != nil || version != 0 {
		return false
	}
	err = sqlitex.ExecuteTransient(conn, "SELECT count(*) FROM sqlite_master", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			objects = stmt.ColumnInt(0)
			return nil
		},
	})
	return err == nil && objects == 0
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
		// the db outlives this call otherwise: an unclosed handle keeps its
		// auto-flush goroutine and connections alive for the process lifetime,
		// and leaves the durability sentinel set so the next open quick-checks.
		// ErrSpaceStorageExists is the common way in -- Derive creates before
		// it loads, so an existing space takes this path on every attempt.
		_ = db.Close()
		return nil, fmt.Errorf("failed to create spacestorage: %w", err)
	}
	cs, err := NewClientStorage(ctx, st)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("new client storage: %w", err)
	}
	return cs, nil
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
