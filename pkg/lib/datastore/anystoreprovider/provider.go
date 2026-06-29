package anystoreprovider

/*
AI generated

Name: Anystore Database Provider
Scope: global

## Responsibility
- Provides common anystore database shared across all spaces
- Provides per-space index and CRDT databases with lazy initialization
- Auto-reinitializes corrupted databases by removing and recreating files

## External State
- objectstore/objects.db - common database with system collection
- objectstore/{spaceId}/objects.db - per-space index databases
- objectstore/{spaceId}/crdt.db - per-space CRDT databases
*/

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"
	"zombiezen.com/go/sqlite"

	"github.com/anyproto/anytype-heart/core/debug/debugreporter"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "anystore-provider"

var log = logging.LoggerNotSugared(CName)

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
	GetAnyStoreConfig() *anystore.Config
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
	s.anyStoreConfig = cfg.GetAnyStoreConfig()
	// Reporter is optional — tests build smaller app graphs without the
	// profiler component. Corruption reports in those contexts become no-ops.
	if r, err := app.GetComponent[debugreporter.Reporter](a); err == nil {
		s.reporter = r
	}

	return s.initInPath(repoPath)
}

func (s *provider) initInPath(repoPath string) error {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	s.objectStorePath = filepath.Join(repoPath, "objectstore")

	s.setDefaultConfig()

	err := ensureDirExists(s.objectStorePath)
	if err != nil {
		return err
	}

	s.commonDb, err = openDatabaseWithReinit(context.Background(), s.getAnyStoreConfig(), filepath.Join(s.objectStorePath, "objects.db"), s.reporter)
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

func getLogger(err error, code sqlite.ResultCode) *zap.Logger {
	return log.With(zap.Error(err), zap.String("code", code.String()), zap.String("desc", code.Message()))
}

// openDatabaseWithReinit tries to open anystore database, if it fails with corruption error it removes the files and tries to open again
func openDatabaseWithReinit(ctx context.Context, config *anystore.Config, path string, reporter debugreporter.Reporter) (anystore.DB, error) {
	err := ensureDirExists(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("ensure dir exists: %w", err)
	}

	start := time.Now()
	db, err := anystore.Open(ctx, path, config)
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

func (s *provider) setDefaultConfig() {
	if s.anyStoreConfig == nil {
		s.anyStoreConfig = &anystore.Config{}
	}
	if s.anyStoreConfig.SQLiteConnectionOptions == nil {
		s.anyStoreConfig.SQLiteConnectionOptions = map[string]string{}
	}
	s.anyStoreConfig.SQLiteConnectionOptions = maps.Clone(s.anyStoreConfig.SQLiteConnectionOptions)
	s.anyStoreConfig.SQLiteConnectionOptions["synchronous"] = "normal"
	s.anyStoreConfig.SQLiteConnectionOptions["wal_autocheckpoint"] = "10000"

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
	s.spaceIndexDbsLock.Lock()
	defer s.spaceIndexDbsLock.Unlock()

	db, ok := s.spaceIndexDbs[spaceId]
	if ok {
		return db, nil
	}

	// Refuse to recreate the DB/dir for a space that is being deleted.
	if s.isSpaceDeleted(spaceId) {
		return nil, ErrSpaceDeleted
	}

	db, err := openDatabaseWithReinit(s.componentCtx, s.getAnyStoreConfig(), filepath.Join(s.objectStorePath, spaceId, "objects.db"), s.reporter)
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

	lock sync.Mutex
	db   anystore.DB
	// deleted is set by DeleteSpaceData; once set, Wait refuses to (re)open the
	// crdt.db so a getter handed out before deletion cannot resurrect the
	// just-removed directory and leak an untracked DB handle.
	deleted bool
}

func (g *AnystoreGetter) get() anystore.DB {
	g.lock.Lock()
	defer g.lock.Unlock()

	return g.db
}

func (g *AnystoreGetter) Wait() (anystore.DB, error) {
	g.lock.Lock()
	defer g.lock.Unlock()

	if g.deleted {
		return nil, ErrSpaceDeleted
	}

	if g.db != nil {
		return g.db, nil
	}

	path := filepath.Join(g.objectStorePath, g.spaceId, "crdt.db")
	db, err := openDatabaseWithReinit(g.ctx, g.config, path, g.reporter)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	g.db = db

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
		// A getter created for a space currently being deleted must not reopen
		// the crdt.db; mark it deleted so Wait short-circuits.
		deleted: s.isSpaceDeleted(spaceId),
	}
	s.crdtDbs[spaceId] = db
	return db
}

func (s *provider) getAnyStoreConfig() *anystore.Config {
	return &anystore.Config{
		Namespace:               s.anyStoreConfig.Namespace,
		ReadConnections:         s.anyStoreConfig.ReadConnections,
		SQLiteConnectionOptions: maps.Clone(s.anyStoreConfig.SQLiteConnectionOptions),
		SyncPoolElementMaxSize:  s.anyStoreConfig.SyncPoolElementMaxSize,
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
		if db := store.get(); db != nil {
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
		crdtGetter.deleted = true
		crdtDb = crdtGetter.db
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

func (s *provider) Flush(timeout time.Duration, waitPending bool) {
	if !s.dbsAreFlushing.CompareAndSwap(false, true) {
		return
	}
	var idleDuration time.Duration
	if waitPending {
		idleDuration = time.Millisecond * 30
	}
	defer s.dbsAreFlushing.Store(false)
	s.spaceIndexDbsLock.Lock()
	s.crtdStoreLock.Lock()
	var dbs = make([]anystore.DB, 0, len(s.spaceIndexDbs)+len(s.crdtDbs)+1)
	for _, db := range s.spaceIndexDbs {
		dbs = append(dbs, db)
	}
	for _, getter := range s.crdtDbs {
		db := getter.get()
		if db != nil {
			dbs = append(dbs, db)
		}
	}
	s.spaceIndexDbsLock.Unlock()
	s.crtdStoreLock.Unlock()
	wg := sync.WaitGroup{}

	dbs = append(dbs, s.commonDb)
	for _, db := range dbs {
		wg.Add(1)
		go func(db anystore.DB) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(s.componentCtx, timeout)
			defer cancel()
			err := db.Flush(ctx, idleDuration, anystore.FlushModeCheckpointPassive)
			if err != nil {
				log.With(zap.Error(err)).Error("failed to flush db")
			}
		}(db)
	}
	wg.Wait()
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
