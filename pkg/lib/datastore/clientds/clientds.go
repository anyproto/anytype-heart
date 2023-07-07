package clientds

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v3"
	"github.com/hashicorp/go-multierror"
	ds "github.com/ipfs/go-datastore"
	dsbadgerv3 "github.com/textileio/go-ds-badger3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName           = "datastore"
	oldLitestoreDir = "ipfslite_v3"
	localstoreDSDir = "localstore"
	SpaceDSDir      = "spacestore"
)

var log = logging.Logger("anytype-clientds")

type clientds struct {
	running bool

	spaceDS                                    *dsbadgerv3.Datastore
	localstoreDS                               *dsbadgerv3.Datastore
	cfg                                        Config
	repoPath                                   string
	migrations                                 []migration
	spaceStoreWasMissing, localStoreWasMissing bool
	spentOnInit                                time.Duration
}

type Config struct {
	Spacestore dsbadgerv3.Options
	Localstore dsbadgerv3.Options
}

var DefaultConfig = Config{
	Spacestore: dsbadgerv3.DefaultOptions,
	Localstore: dsbadgerv3.DefaultOptions,
}

type DSConfigGetter interface {
	DSConfig() Config
}

type migration struct {
	migrationFunc func() error
	migrationKey  ds.Key
}

func init() {

	// used to store all objects tree changes + some metadata
	DefaultConfig.Spacestore.MemTableSize = 16 * 1024 * 1024     // Memtable saves all values below value threshold + write ahead log, actual file size is 2x the amount, the size is preallocated
	DefaultConfig.Spacestore.ValueLogFileSize = 64 * 1024 * 1024 // Vlog has all values more than value threshold, actual file uses 2x the amount, the size is preallocated
	DefaultConfig.Spacestore.GcInterval = 0
	DefaultConfig.Spacestore.GcSleep = 0
	DefaultConfig.Spacestore.ValueThreshold = 1024 * 128 // Object details should be small enough, e.g. under 10KB. 512KB here is just a precaution.
	DefaultConfig.Spacestore.Logger = logging.LWrapper{logging.Logger("store.spacestore")}
	DefaultConfig.Spacestore.SyncWrites = false
	DefaultConfig.Spacestore.WithCompression(0) // disable compression

	// used to store objects localstore + threads logs info
	DefaultConfig.Localstore.MemTableSize = 64 * 1024 * 1024
	DefaultConfig.Localstore.ValueLogFileSize = 16 * 1024 * 1024 // Vlog has all values more than value threshold, actual file uses 2x the amount, the size is preallocated
	DefaultConfig.Localstore.GcInterval = 0                      // we don't need to have value GC here, because all the values should fit in the ValueThreshold. So GC will be done by the live LSM compactions
	DefaultConfig.Localstore.GcSleep = 0
	DefaultConfig.Localstore.ValueThreshold = 1024 * 1024 // Object details should be small enough, e.g. under 10KB. 512KB here is just a precaution.
	DefaultConfig.Localstore.Logger = logging.LWrapper{logging.Logger("store.localstore")}
	DefaultConfig.Localstore.SyncWrites = false
	DefaultConfig.Localstore.WithCompression(0) // disable compression

}

func (r *clientds) Init(a *app.App) (err error) {
	// TODO: looks like we do a lot of stuff on Init here. We should consider moving it to the Run
	start := time.Now()
	wl := a.Component(wallet.CName)
	if wl == nil {
		return fmt.Errorf("need wallet to be inited first")
	}
	r.repoPath = wl.(wallet.Wallet).RepoPath()

	if cfgGetter, ok := a.Component("config").(DSConfigGetter); ok {
		r.cfg = cfgGetter.DSConfig()
	} else {
		return fmt.Errorf("ds config is missing")
	}

	r.migrations = []migration{}

	if _, err := os.Stat(filepath.Join(r.getRepoPath(oldLitestoreDir))); !os.IsNotExist(err) {
		return fmt.Errorf("old repo found")
	}

	if _, err := os.Stat(r.getRepoPath(localstoreDSDir)); os.IsNotExist(err) {
		r.localStoreWasMissing = true
	}

	if _, err := os.Stat(r.getRepoPath(SpaceDSDir)); os.IsNotExist(err) {
		r.spaceStoreWasMissing = true
	}

	RemoveExpiredLocks(r.repoPath)

	r.localstoreDS, err = dsbadgerv3.NewDatastore(r.getRepoPath(localstoreDSDir), &r.cfg.Localstore)
	if err != nil {
		return fmt.Errorf("failed to init local datastore")
	}

	r.spaceDS, err = dsbadgerv3.NewDatastore(r.getRepoPath(SpaceDSDir), &r.cfg.Spacestore)
	if err != nil {
		return fmt.Errorf("failed to init space datastore")
	}

	err = r.migrateIfNeeded()
	if err != nil {
		return fmt.Errorf("migrateIfNeeded failed: %w", err)
	}

	r.running = true
	r.spentOnInit = time.Since(start)
	return nil
}

func (r *clientds) Run(context.Context) error {
	return nil
}

func (r *clientds) migrateIfNeeded() error {
	for _, m := range r.migrations {
		_, err := r.localstoreDS.Get(context.Background(), m.migrationKey)
		if err == nil {
			continue
		}
		if err != nil && err != ds.ErrNotFound {
			return err
		}
		err = m.migrationFunc()
		if err != nil {
			return fmt.Errorf(
				"migration with key %s failed: failed to migrate the keys from old db: %w",
				m.migrationKey.String(),
				err)
		}
		err = r.localstoreDS.Put(context.Background(), m.migrationKey, nil)
		if err != nil {
			return fmt.Errorf("failed to put %s migration key into db: %w", m.migrationKey.String(), err)
		}
	}
	return nil
}

func (r *clientds) SpaceStorage() (*badger.DB, error) {
	// TODO: [MR] Change after testing
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.spaceDS.DB, nil
}

func (r *clientds) LocalstoreDS() (datastore.DSTxnBatching, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.localstoreDS, nil
}

func (r *clientds) LocalstoreBadger() (*badger.DB, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.localstoreDS.DB, nil
}

func (r *clientds) Name() (name string) {
	return CName
}

func (r *clientds) Close(ctx context.Context) (err error) {
	if r.localstoreDS != nil {
		err2 := r.localstoreDS.Close()
		if err2 != nil {
			err = multierror.Append(err, err2)
		}
	}

	if r.spaceDS != nil {
		err2 := r.spaceDS.Close()
		if err2 != nil {
			err = multierror.Append(err, err2)
		}
	}

	return err
}

func New() datastore.Datastore {
	return &clientds{}
}

func (r *clientds) getRepoPath(dir string) string {
	return filepath.Join(r.repoPath, dir)
}

func (r *clientds) GetLogFields() []zap.Field {
	return []zap.Field{
		zap.Bool("spaceStoreWasMissing", r.spaceStoreWasMissing),
		zap.Bool("localStoreWasMissing", r.localStoreWasMissing),
		zap.Int64("spentOnInit", r.spentOnInit.Milliseconds()),
	}
}
