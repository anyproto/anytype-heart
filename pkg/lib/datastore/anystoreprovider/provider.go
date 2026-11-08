package anystoreprovider

/*
AI generated

Name: Anystore Database Provider
Scope: global

## Responsibility
- Provides common anystore database shared across all spaces
- Provides per-space index and CRDT databases with lazy initialization
- Auto-reinitializes corrupted databases by removing and recreating files

- Migrates each any-store v1 (SQLite) database to v2 at its first open (anystoremigrate), one at
  a time; the account start fails up front when the volume can't hold the migrations

## External State
- objectstore/objects.db - common database with system collection
- objectstore/{spaceId}/objects.db - per-space index databases
- objectstore/{spaceId}/crdt.db - per-space CRDT databases
- objectstore/[{spaceId}/]{objects,crdt}.db.v1 - the v1 file of a migrated database, kept (removeV1AfterMigration)
- objectstore/.anystore-v2 - marks a store with no v1 database left
*/

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	anystorev1 "github.com/anyproto/any-store"
	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"
	"zombiezen.com/go/sqlite"

	"github.com/anyproto/anytype-heart/core/debug/debugreporter"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoremigrate"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/freespace"
)

const CName = "anystore-provider"

var log = logging.LoggerNotSugared(CName)

// initPageBufferOnce pre-allocates the global page-buffer slab (64 MiB at 4096 B/page)
// before the first database is opened. Mirrors the old SQLiteGlobalPageCachePreallocateSizeBytes.
var initPageBufferOnce sync.Once

func ensurePageBuffer() {
	initPageBufferOnce.Do(func() {
		anystore.InitPageBuffer(4096, 32768) // 32768 * 4096 = 128 MiB
	})
}

const (
	// migrationConcurrency is how many v1 databases migrate at once. Measured on a real store (163
	// spaces, 238 databases, 444 MB, 3 runs each, cmd/anystoremigrate): one at a time took 17.1 /
	// 18.1 / 19.1 s, two at a time 11.8 / 12.0 / 12.1 s, with the same peak WAL (37.5 MB, one
	// database's) and 3% more peak extra disk (360 MB against 350 MB); peak RSS went from ~284 MB
	// to ~306-399 MB.
	migrationConcurrency = 2
	// removeV1AfterMigration removes a database's v1 file once its verified v2 file is in place.
	// Off: the v1 file stays beside it as <name>.v1, so a broken migration can still be recovered
	// from; it costs the size of the old store.
	removeV1AfterMigration = false
	// v2Marker marks an object store with no v1 database left, so later starts skip the scan.
	v2Marker = ".anystore-v2"
)

// freeDiskSpace is the free space of a volume; tests replace it.
var freeDiskSpace = freespace.GetFreeDiskSpace

// MigrationOptions are the options the provider migrates v1 databases with; cfg is the v2
// configuration the databases are opened with (nil: the provider's default). It initializes the
// global page buffer the configuration uses.
func MigrationOptions(cfg *anystore.Config) anystoremigrate.Options {
	ensurePageBuffer()
	if cfg == nil {
		cfg = (&provider{anyStoreConfig: &anystore.Config{}}).getAnyStoreConfig()
	}
	return anystoremigrate.Options{
		Concurrency:         migrationConcurrency,
		RemoveV1AfterVerify: removeV1AfterMigration,
		DstConfig:           cfg,
		FreeSpace:           freeDiskSpace,
		SrcConfig: &anystorev1.Config{
			ReadConnections: 1,
			// the SQLite page cache is preallocated once per process, by whoever opens a v1
			// database first: the same size the v1 space store asks for
			SQLiteGlobalPageCachePreallocateSizeBytes: 1 << 26,
		},
	}
}

// DatabasePaths lists the databases of an object store: objects.db and, for every space,
// <spaceId>/objects.db and <spaceId>/crdt.db, those that exist.
func DatabasePaths(objectStorePath string) ([]string, error) {
	paths := []string{filepath.Join(objectStorePath, "objects.db")}
	entries, err := os.ReadDir(objectStorePath)
	if err != nil {
		return nil, fmt.Errorf("read object store dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, name := range []string{"objects.db", "crdt.db"} {
			p := filepath.Join(objectStorePath, entry.Name(), name)
			if _, err := os.Stat(p); err == nil {
				paths = append(paths, p)
			}
		}
	}
	return paths, nil
}

// removeMigrationLeftovers removes, for every database name of the store, the temp database an
// interrupted migration left (anystoremigrate.RemoveLeftovers). Only that family of files goes:
// never a database, live or kept as <name>.v1.
func removeMigrationLeftovers(objectStorePath string) error {
	bases := []string{filepath.Join(objectStorePath, "objects.db")}
	entries, err := os.ReadDir(objectStorePath)
	if err != nil {
		return fmt.Errorf("read object store dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			bases = append(bases,
				filepath.Join(objectStorePath, entry.Name(), "objects.db"),
				filepath.Join(objectStorePath, entry.Name(), "crdt.db"))
		}
	}
	for _, base := range bases {
		if err = anystoremigrate.RemoveLeftovers(base); err != nil {
			return fmt.Errorf("remove leftovers of %s: %w", base, err)
		}
	}
	return nil
}

// ErrSpaceDeleted is returned when a space's databases are being removed via
// DeleteSpaceData and a caller tries to (re)open them, which would resurrect
// the just-removed on-disk directory.
var ErrSpaceDeleted = errors.New("space data is deleted")

type systemKeys struct {
}

func (k systemKeys) PaymentCacheKey(ver int) string {
	return fmt.Sprintf("payments_subscription_v%d", ver)
}

func (k systemKeys) PaymentCacheV2Key(ver int) string {
	return fmt.Sprintf("payments_subscription_v2_v%d", ver)
}

func (k systemKeys) PortKey() string {
	return "drpc_server_port"
}

func (k systemKeys) NodeUsage() string {
	return "node_usage"
}

func (k systemKeys) FileReconcilerStarted() string {
	return "file_reconciler_started"
}

func (k systemKeys) AccountStatus() string {
	return "account_status"
}

var SystemKeys = systemKeys{}

type Provider interface {
	// GetCommonDb returns an instance of anystore common across spaces
	GetCommonDb() anystore.DB

	// GetSystemCollection returns a collection for various system thing. It should be used with
	// static keys like:
	//   const accountStatusKey = "account_status"
	GetSystemCollection() anystore.Collection

	GetSpaceIndexDb(spaceId string) (anystore.DB, error)
	GetCrdtDb(spaceId string) *AnystoreGetter

	ListSpaceIdsFromFilesystem() ([]string, error)

	// DeleteSpaceData closes all DBs (spaceIndex + CRDT) and removes the space directory from filesystem
	DeleteSpaceData(spaceId string) error

	app.ComponentRunnable
}

type configProvider interface {
	GetAnyStoreV2Config() *anystore.Config
}

type provider struct {
	objectStorePath string
	anyStoreConfig  *anystore.Config

	commonDb           anystore.DB
	commonDbLockRemove func() error
	systemCollection   anystore.Collection

	crtdStoreLock sync.Mutex
	crdtDbs       map[string]*AnystoreGetter

	spaceIndexDbsLock sync.Mutex
	spaceIndexDbs     map[string]anystore.DB

	// deletedSpaceIds tombstones spaces while DeleteSpaceData is closing their
	// DBs and removing their files, so GetSpaceIndexDb / GetCrdtDb / a
	// previously handed-out AnystoreGetter.Wait refuse to reopen (and recreate
	// the on-disk dir for) a space that is being deleted. Guarded by its own
	// lock to avoid ordering constraints with the per-map locks above.
	deletedSpaceIdsLock sync.Mutex
	deletedSpaceIds     map[string]struct{}

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	dbsAreFlushing atomic.Bool

	migrator *anystoremigrate.Migrator

	// reporter is looked up in Init; may be nil when the provider is
	// constructed outside an app container (NewInPath, tests).
	reporter debugreporter.Reporter
}

func New() Provider {
	return &provider{
		crdtDbs:         map[string]*AnystoreGetter{},
		spaceIndexDbs:   map[string]anystore.DB{},
		deletedSpaceIds: map[string]struct{}{},
		anyStoreConfig:  &anystore.Config{},
	}
}

func NewInPath(rootPath string) (Provider, error) {
	p := New().(*provider)
	err := p.initInPath(rootPath)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *provider) Name() string {
	return CName
}

func (s *provider) Init(a *app.App) error {
	// For tests: don't run init code if the provider is initialized via NewInPath
	if s.commonDb != nil {
		return nil
	}

	cfg := app.MustComponent[configProvider](a)
	repoPath := app.MustComponent[wallet.Wallet](a).RepoPath()
	s.anyStoreConfig = cfg.GetAnyStoreV2Config()
	// Reporter is optional — tests build smaller app graphs without the
	// profiler component. Corruption reports in those contexts become no-ops.
	if r, err := app.GetComponent[debugreporter.Reporter](a); err == nil {
		s.reporter = r
	}

	return s.initInPath(repoPath)
}

func (s *provider) initInPath(repoPath string) error {
	ensurePageBuffer()
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	s.objectStorePath = filepath.Join(repoPath, "objectstore")

	s.setDefaultConfig()

	err := ensureDirExists(s.objectStorePath)
	if err != nil {
		return err
	}

	s.migrator = anystoremigrate.New(MigrationOptions(s.getAnyStoreConfig()))
	if err = s.checkMigrationSpace(); err != nil {
		return err
	}

	s.commonDb, err = openDatabaseWithReinit(context.Background(), s.getAnyStoreConfig(), filepath.Join(s.objectStorePath, "objects.db"), s.reporter, s.migrator)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	s.systemCollection, err = s.commonDb.Collection(s.componentCtx, "system")
	if err != nil {
		return fmt.Errorf("init system collection: %w", err)
	}

	return nil
}

func (s *provider) Run(ctx context.Context) error {
	return nil
}

// checkMigrationSpace fails the start, before any database is written, when the store holds v1
// databases and the volume can't hold their migration (anystoremigrate.ErrNotEnoughSpace). The
// databases themselves migrate one by one as they are opened. A store found with no v1 database
// is marked, so later starts skip the scan.
//
// What an interrupted migration left is removed first: an abandoned temp database holds space the
// check would otherwise count against the store, and every start after a crash on a tight disk
// would be refused.
func (s *provider) checkMigrationSpace() error {
	marker := filepath.Join(s.objectStorePath, v2Marker)
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	if err := removeMigrationLeftovers(s.objectStorePath); err != nil {
		return fmt.Errorf("remove leftovers of interrupted v1 migrations: %w", err)
	}
	paths, err := DatabasePaths(s.objectStorePath)
	if err != nil {
		return fmt.Errorf("list databases: %w", err)
	}
	var v1 []string
	for _, p := range paths {
		isV1, err := anystoremigrate.IsV1File(p)
		if err != nil {
			return fmt.Errorf("check v1 database: %w", err)
		}
		if isV1 {
			v1 = append(v1, p)
		}
	}
	if len(v1) == 0 {
		if err = os.WriteFile(marker, nil, 0o600); err != nil {
			log.Warn("write v2 store marker", zap.Error(err))
		}
		return nil
	}
	log.Info("object store holds any-store v1 databases, migrating each at its first open", zap.Int("databases", len(v1)))
	if err = s.migrator.CheckSpace(v1); err != nil {
		return fmt.Errorf("check space for the v1 database migration: %w", err)
	}
	return nil
}

func getLogger(err error, code sqlite.ResultCode) *zap.Logger {
	return log.With(zap.Error(err), zap.String("code", code.String()), zap.String("desc", code.Message()))
}

// openDatabaseWithReinit tries to open anystore database, if it fails with corruption error it removes the files and tries to open again.
// A v1 database is migrated to v2 first; a failed migration leaves it as it was and is returned as an error.
func openDatabaseWithReinit(ctx context.Context, config *anystore.Config, path string, reporter debugreporter.Reporter, migrator *anystoremigrate.Migrator) (anystore.DB, error) {
	err := ensureDirExists(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("ensure dir exists: %w", err)
	}
	if err = migrateIfV1(ctx, migrator, path); err != nil {
		return nil, err
	}

	start := time.Now()
	db, err := anystore.Open(ctx, path, config)
	if errors.Is(err, anystore.ErrV1Database) {
		// never a corruption: the v1 file must not be removed
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err != nil {
		code, isCorrupted := anystorehelper.IsCorruptedError(err)
		getLogger(err, code).With(zap.Bool("isCorrupted", isCorrupted)).With(zap.Int64("tookMs", time.Since(start).Milliseconds())).Error("failed to open anystore")
		if isCorrupted {
			if reporter != nil {
				reporter.Report("DB_CORRUPTION", map[string]any{
					"db":     filepath.Join(filepath.Base(filepath.Dir(path)), filepath.Base(path)),
					"code":   code.String(),
					"desc":   code.Message(),
					"error":  err.Error(),
					"tookMs": time.Since(start).Milliseconds(),
				}, debugreporter.Capture{Kind: debugreporter.KindNone})
			}
			removeErr := anystorehelper.RemoveSqliteFiles(path)
			if removeErr != nil {
				log.Error("failed to remove sqlite files", zap.Error(removeErr))
				return nil, removeErr
			}
			db, err = anystore.Open(ctx, path, config)
			if err != nil {
				code, _ = anystorehelper.IsCorruptedError(err)
				getLogger(err, code).Error("failed to open anystore again")
				return nil, err
			}
			return db, nil
		}
		return nil, err
	} else if time.Since(start) > time.Second {
		// only log for not-corrupted opens
		ctxStat, cancel := context.WithTimeout(ctx, time.Second*2)
		defer cancel()

		logger := log.With(zap.String("db", filepath.Base(path))).With(zap.Int64("tookMs", time.Since(start).Milliseconds()))
		stat, err := db.Stats(ctxStat)
		if err != nil {
			logger = logger.With(zap.Error(err))
		} else {
			logger = logger.With(anystorehelper.DbStatToZapFields(stat)...)
		}
		logger.Warn("objectstore db open took too long")
	}

	return db, nil
}

// migrateIfV1 migrates the database at path to v2 when it is a v1 one, waiting for its turn, and
// removes what an interrupted migration of it left.
func migrateIfV1(ctx context.Context, migrator *anystoremigrate.Migrator, path string) error {
	isV1, err := anystoremigrate.IsV1File(path)
	if err != nil {
		return fmt.Errorf("check v1 database: %w", err)
	}
	if !isV1 {
		if err = anystoremigrate.RemoveLeftovers(path); err != nil {
			return fmt.Errorf("remove leftovers of a v1 migration: %w", err)
		}
		return nil
	}
	if _, err = migrator.MigrateFile(ctx, path); err != nil {
		return fmt.Errorf("migrate v1 database %s: %w", path, err)
	}
	return nil
}

func (s *provider) setDefaultConfig() {
	if s.anyStoreConfig == nil {
		s.anyStoreConfig = &anystore.Config{}
	}
}

func (s *provider) GetCommonDb() anystore.DB {
	return s.commonDb
}

func (s *provider) GetSystemCollection() anystore.Collection {
	return s.systemCollection
}

func (s *provider) isSpaceDeleted(spaceId string) bool {
	s.deletedSpaceIdsLock.Lock()
	defer s.deletedSpaceIdsLock.Unlock()
	_, ok := s.deletedSpaceIds[spaceId]
	return ok
}

func (s *provider) GetSpaceIndexDb(spaceId string) (anystore.DB, error) {
	path := filepath.Join(s.objectStorePath, spaceId, "objects.db")
	s.spaceIndexDbsLock.Lock()
	db, ok := s.spaceIndexDbs[spaceId]
	s.spaceIndexDbsLock.Unlock()
	if ok {
		return db, nil
	}
	if s.isSpaceDeleted(spaceId) {
		return nil, ErrSpaceDeleted
	}
	// A v1 database migrates outside the lock: every open of a space index and every flush takes
	// it, and a migration takes long. Two openers of one space wait for one migration.
	if err := migrateIfV1(s.componentCtx, s.migrator, path); err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	s.spaceIndexDbsLock.Lock()
	defer s.spaceIndexDbsLock.Unlock()

	db, ok = s.spaceIndexDbs[spaceId]
	if ok {
		return db, nil
	}

	// Refuse to recreate the DB/dir for a space that is being deleted.
	if s.isSpaceDeleted(spaceId) {
		return nil, ErrSpaceDeleted
	}

	db, err := openDatabaseWithReinit(s.componentCtx, s.getAnyStoreConfig(), path, s.reporter, s.migrator)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	s.spaceIndexDbs[spaceId] = db

	return db, nil
}

type AnystoreGetter struct {
	ctx             context.Context
	config          *anystore.Config
	objectStorePath string
	spaceId         string
	reporter        debugreporter.Reporter
	migrator        *anystoremigrate.Migrator

	// lock serializes the opens of the database; an open can take long (a v1 database migrates).
	lock sync.Mutex
	// opened publishes the database once it is open. It is read without the lock, so a flush or
	// a caller sees an open database whoever holds the lock; nil while it is still opening.
	opened atomic.Pointer[openedDb]
	// deleted is set by DeleteSpaceData; once set, Wait refuses to (re)open the
	// crdt.db so a getter handed out before deletion cannot resurrect the
	// just-removed directory and leak an untracked DB handle.
	deleted atomic.Bool
}

type openedDb struct {
	db anystore.DB
}

// get returns the database if it is open, nil if it is not open yet (or still opening).
func (g *AnystoreGetter) get() anystore.DB {
	if h := g.opened.Load(); h != nil {
		return h.db
	}
	return nil
}

// getWaiting is get after waiting out an open in progress.
func (g *AnystoreGetter) getWaiting() anystore.DB {
	g.lock.Lock()
	defer g.lock.Unlock()

	return g.get()
}

func (g *AnystoreGetter) Wait() (anystore.DB, error) {
	if g.deleted.Load() {
		return nil, ErrSpaceDeleted
	}
	if db := g.get(); db != nil {
		return db, nil
	}

	g.lock.Lock()
	defer g.lock.Unlock()

	if g.deleted.Load() {
		return nil, ErrSpaceDeleted
	}
	if db := g.get(); db != nil {
		return db, nil
	}

	path := filepath.Join(g.objectStorePath, g.spaceId, "crdt.db")
	db, err := openDatabaseWithReinit(g.ctx, g.config, path, g.reporter, g.migrator)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	g.opened.Store(&openedDb{db: db})

	return db, nil
}

func (s *provider) GetCrdtDb(spaceId string) *AnystoreGetter {
	s.crtdStoreLock.Lock()
	defer s.crtdStoreLock.Unlock()

	db, ok := s.crdtDbs[spaceId]
	if ok {
		return db
	}

	db = &AnystoreGetter{
		spaceId:         spaceId,
		ctx:             s.componentCtx,
		config:          s.getAnyStoreConfig(),
		objectStorePath: s.objectStorePath,
		reporter:        s.reporter,
		migrator:        s.migrator,
	}
	// A getter created for a space currently being deleted must not reopen
	// the crdt.db; mark it deleted so Wait short-circuits.
	db.deleted.Store(s.isSpaceDeleted(spaceId))
	s.crdtDbs[spaceId] = db
	return db
}

func (s *provider) getAnyStoreConfig() *anystore.Config {
	return &anystore.Config{
		SyncPoolElementMaxSize: s.anyStoreConfig.SyncPoolElementMaxSize,
		AutoCheckpointAfter:    10000,
		UseGlobalPageBuffer:    true,
		Durability: anystore.DurabilityConfig{
			AutoFlush: true,
			IdleAfter: time.Second * 20,
			FlushMode: anystore.FlushModeCheckpointPassive,
			Sentinel:  true,
		},
	}
}

func (s *provider) Close(ctx context.Context) error {
	var err error

	s.componentCtxCancel()
	if s.commonDb != nil {
		err = errors.Join(err, s.commonDb.Close())
	}

	s.spaceIndexDbsLock.Lock()
	// close in parallel
	closeChan := make(chan error, len(s.spaceIndexDbs))
	for spaceId, store := range s.spaceIndexDbs {
		go func(spaceId string, store anystore.DB) {
			closeChan <- store.Close()
		}(spaceId, store)
	}
	for i := 0; i < len(s.spaceIndexDbs); i++ {
		err = errors.Join(err, <-closeChan)
	}
	s.spaceIndexDbs = map[string]anystore.DB{}
	s.spaceIndexDbsLock.Unlock()

	s.crtdStoreLock.Lock()
	// Only count getters whose DB was actually opened: an AnystoreGetter
	// registered via GetCrdtDb but never Wait()-ed has a nil db, so its
	// goroutine would never send and the receive loop below would deadlock if
	// we counted it.
	var openCrdtDbs []anystore.DB
	for _, store := range s.crdtDbs {
		if db := store.getWaiting(); db != nil {
			openCrdtDbs = append(openCrdtDbs, db)
		}
	}
	closeChan = make(chan error, len(openCrdtDbs))
	for _, db := range openCrdtDbs {
		go func(db anystore.DB) {
			closeChan <- db.Close()
		}(db)
	}
	for i := 0; i < len(openCrdtDbs); i++ {
		err = errors.Join(err, <-closeChan)
	}
	s.crdtDbs = map[string]*AnystoreGetter{}
	s.crtdStoreLock.Unlock()

	return err
}

func (s *provider) ListSpaceIdsFromFilesystem() ([]string, error) {
	entries, err := os.ReadDir(s.objectStorePath)
	if err != nil {
		return nil, err
	}
	var spaceIds []string
	for _, entry := range entries {
		if entry.IsDir() {
			spaceIds = append(spaceIds, entry.Name())
		}
	}
	return spaceIds, err
}

// DeleteSpaceData closes the space index and CRDT databases for the given space
// and removes the whole space directory from the filesystem. It always attempts
// the directory removal even if closing one of the DBs fails, joining all errors.
//
// The space is tombstoned for the duration so that a concurrent GetSpaceIndexDb
// / GetCrdtDb / a previously handed-out AnystoreGetter.Wait cannot reopen (and
// recreate the just-removed directory of) the space mid-deletion. DBs are
// captured under the per-map locks and Closed outside them: db.Close blocks
// unbounded acquiring the write connection, so closing under the lock would
// stall every concurrent GetSpaceIndexDb/GetCrdtDb/Flush.
func (s *provider) DeleteSpaceData(spaceId string) error {
	var errs error

	s.deletedSpaceIdsLock.Lock()
	s.deletedSpaceIds[spaceId] = struct{}{}
	s.deletedSpaceIdsLock.Unlock()

	// Capture and forget the spaceIndex DB under the lock, close it outside.
	s.spaceIndexDbsLock.Lock()
	indexDb, hasIndexDb := s.spaceIndexDbs[spaceId]
	if hasIndexDb {
		delete(s.spaceIndexDbs, spaceId)
	}
	s.spaceIndexDbsLock.Unlock()
	if hasIndexDb {
		if err := indexDb.Close(); err != nil {
			errs = errors.Join(errs, fmt.Errorf("close space index db: %w", err))
		}
	}

	// Capture and forget the CRDT DB under the lock, mark the getter deleted so
	// a concurrent holder's Wait short-circuits, then close it outside the lock.
	s.crtdStoreLock.Lock()
	var crdtDb anystore.DB
	if crdtGetter, ok := s.crdtDbs[spaceId]; ok {
		crdtGetter.lock.Lock()
		crdtGetter.deleted.Store(true)
		if h := crdtGetter.opened.Swap(nil); h != nil {
			crdtDb = h.db
		}
		crdtGetter.lock.Unlock()
		delete(s.crdtDbs, spaceId)
	}
	s.crtdStoreLock.Unlock()
	if crdtDb != nil {
		if err := crdtDb.Close(); err != nil {
			errs = errors.Join(errs, fmt.Errorf("close crdt db: %w", err))
		}
	}

	// Always attempt to remove the space directory from filesystem
	spacePath := filepath.Join(s.objectStorePath, spaceId)
	if err := os.RemoveAll(spacePath); err != nil {
		errs = errors.Join(errs, fmt.Errorf("remove space directory: %w", err))
	}

	// Lift the tombstone once removal succeeded so the same space can be
	// re-joined later in the session. Keep it on error so a retried delete
	// still refuses re-opens until removal actually succeeds.
	if errs == nil {
		s.deletedSpaceIdsLock.Lock()
		delete(s.deletedSpaceIds, spaceId)
		s.deletedSpaceIdsLock.Unlock()
	}

	return errs
}

// flushWaitSlack bounds how long Flush may wait past the per-DB timeout. SQLite does not
// promptly honor context cancellation inside a checkpoint (a goroutine blocked in a read
// syscall ignores sqlite3_interrupt until it returns), so a plain wg.Wait() can overrun by
// minutes on throttled mobile background I/O (GO-7393).
const flushWaitSlack = time.Second * 2

func (s *provider) Flush(timeout time.Duration, waitPending bool, mode anystore.FlushMode) {
	if !s.dbsAreFlushing.CompareAndSwap(false, true) {
		return
	}
	var idleDuration time.Duration
	if waitPending {
		idleDuration = time.Millisecond * 30
	}
	dbs := s.flushTargets()
	wg := &sync.WaitGroup{}
	for _, db := range dbs {
		wg.Add(1)
		go func(db anystore.DB) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(s.componentCtx, timeout)
			defer cancel()
			err := db.Flush(ctx, idleDuration, mode)
			if err != nil {
				log.With(zap.Error(err)).Error("failed to flush db")
			}
		}(db)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		// unlock the next flush only after the goroutines actually finished
		s.dbsAreFlushing.Store(false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout + flushWaitSlack):
		log.With(zap.Int("dbs", len(dbs)), zap.Duration("timeout", timeout)).Warn("flush overran its budget, returning early")
	}
}

// flushTargets are the open databases. A crdt database still opening (a v1 one migrating can take
// minutes) has nothing to flush yet: it is left out without waiting for it, but an open one is in
// whoever holds its getter's lock.
func (s *provider) flushTargets() []anystore.DB {
	s.spaceIndexDbsLock.Lock()
	s.crtdStoreLock.Lock()
	defer s.spaceIndexDbsLock.Unlock()
	defer s.crtdStoreLock.Unlock()
	dbs := make([]anystore.DB, 0, len(s.spaceIndexDbs)+len(s.crdtDbs)+1)
	for _, db := range s.spaceIndexDbs {
		dbs = append(dbs, db)
	}
	for _, getter := range s.crdtDbs {
		if db := getter.get(); db != nil {
			dbs = append(dbs, db)
		}
	}
	return append(dbs, s.commonDb)
}

func ensureDirExists(dir string) error {
	_, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(dir, 0700)
		if err != nil {
			return fmt.Errorf("create db dir: %w", err)
		}
	}
	return nil
}
