package anystoreprovider

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/anystorehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "anystore-provider"

var log = logging.LoggerNotSugared(CName)

type systemKeys struct {
}

func (k systemKeys) PaymentCacheKey(ver int) string {
	return fmt.Sprintf("payments_subscription_v%d", ver)
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

	app.ComponentRunnable
}

type configProvider interface {
	GetAnyStoreConfig() *anystore.Config
}

type provider struct {
	objectStorePath string
	anyStoreConfig  *anystore.Config

	commonDb           *dbWithFileLock
	commonDbLockRemove func() error
	systemCollection   anystore.Collection

	crtdStoreLock sync.Mutex
	crdtDbs       map[string]*AnystoreGetter

	spaceIndexDbsLock sync.Mutex
	spaceIndexDbs     map[string]*dbWithFileLock

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

type dbWithFileLock struct {
	db         anystore.DB
	lockRemove func() error
}

func (db *dbWithFileLock) Close(ctx context.Context) error {
	return errors.Join(
		db.db.Checkpoint(context.Background(), true),
		db.db.Close(),
		db.lockRemove(),
	)
}

func New() Provider {
	return &provider{
		crdtDbs:        map[string]*AnystoreGetter{},
		spaceIndexDbs:  map[string]*dbWithFileLock{},
		anyStoreConfig: &anystore.Config{},
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

	s.commonDb, err = openDatabase(context.Background(), s.getAnyStoreConfig(), filepath.Join(s.objectStorePath, "objects.db"))
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	s.systemCollection, err = s.commonDb.db.Collection(s.componentCtx, "system")
	if err != nil {
		return fmt.Errorf("init system collection: %w", err)
	}

	return nil
}

func (s *provider) Run(ctx context.Context) error {
	return nil
}

func openDatabase(ctx context.Context, config *anystore.Config, path string) (*dbWithFileLock, error) {
	err := ensureDirExists(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("ensure dir exists: %w", err)
	}

	db, lockRemove, err := anystorehelper.OpenDatabaseWithLockCheck(ctx, path, config)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return &dbWithFileLock{
		db:         db,
		lockRemove: lockRemove,
	}, nil
}

func (s *provider) setDefaultConfig() {
	if s.anyStoreConfig == nil {
		s.anyStoreConfig = &anystore.Config{}
	}
	if s.anyStoreConfig.SQLiteConnectionOptions == nil {
		s.anyStoreConfig.SQLiteConnectionOptions = map[string]string{}
	}
	s.anyStoreConfig.SQLiteConnectionOptions = maps.Clone(s.anyStoreConfig.SQLiteConnectionOptions)
	s.anyStoreConfig.SQLiteConnectionOptions["synchronous"] = "off"
}

func (s *provider) GetCommonDb() anystore.DB {
	return s.commonDb.db
}

func (s *provider) GetSystemCollection() anystore.Collection {
	return s.systemCollection
}

func (s *provider) GetSpaceIndexDb(spaceId string) (anystore.DB, error) {
	s.spaceIndexDbsLock.Lock()
	defer s.spaceIndexDbsLock.Unlock()

	db, ok := s.spaceIndexDbs[spaceId]
	if ok {
		return db.db, nil
	}

	db, err := openDatabase(s.componentCtx, s.getAnyStoreConfig(), filepath.Join(s.objectStorePath, spaceId, "objects.db"))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	s.spaceIndexDbs[spaceId] = db

	return db.db, nil
}

type AnystoreGetter struct {
	ctx             context.Context
	config          *anystore.Config
	objectStorePath string
	spaceId         string

	lock sync.Mutex
	db   *dbWithFileLock
}

func (g *AnystoreGetter) get() *dbWithFileLock {
	g.lock.Lock()
	defer g.lock.Unlock()

	return g.db
}

func (g *AnystoreGetter) Wait() (anystore.DB, error) {
	g.lock.Lock()
	defer g.lock.Unlock()

	if g.db != nil {
		return g.db.db, nil
	}

	path := filepath.Join(g.objectStorePath, g.spaceId, "crdt.db")
	db, err := openDatabase(g.ctx, g.config, path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	g.db = db

	return db.db, nil
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
	}
}

func (s *provider) Close(ctx context.Context) error {
	var err error

	s.componentCtxCancel()
	if s.commonDb != nil {
		err = errors.Join(err, s.commonDb.Close(ctx))
	}

	s.spaceIndexDbsLock.Lock()
	// close in parallel
	closeChan := make(chan error, len(s.spaceIndexDbs))
	for spaceId, store := range s.spaceIndexDbs {
		go func(spaceId string, store *dbWithFileLock) {
			closeChan <- store.Close(ctx)
		}(spaceId, store)
	}
	for i := 0; i < len(s.spaceIndexDbs); i++ {
		err = errors.Join(err, <-closeChan)
	}
	s.spaceIndexDbs = map[string]*dbWithFileLock{}
	s.spaceIndexDbsLock.Unlock()

	s.crtdStoreLock.Lock()
	closeChan = make(chan error, len(s.crdtDbs))
	for spaceId, store := range s.crdtDbs {
		db := store.get()
		go func(spaceId string, db *dbWithFileLock) {
			if db != nil {
				closeChan <- db.Close(ctx)
			}
		}(spaceId, db)
	}
	for i := 0; i < len(s.crdtDbs); i++ {
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
