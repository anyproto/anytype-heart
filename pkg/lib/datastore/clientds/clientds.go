package clientds

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

const (
	CName      = "datastore"
	SpaceDSDir = "spacestore"
)

var log = logging.Logger("anytype-clientds")

var ErrSpaceStoreNotAvailable = fmt.Errorf("space store badger db is not available")

type clientds struct {
	running bool

	spaceStorageMode     storage.SpaceStorageMode
	spaceDS              *badger.DB
	cfg                  Config
	repoPath             string
	spaceStoreWasMissing bool
	spentOnInit          time.Duration
	closing              chan struct{}
	syncerFinished       chan struct{}
}

type Config struct {
	Spacestore badger.Options
}

var DefaultConfig = Config{
	Spacestore: badger.DefaultOptions(""),
}

type DSConfigGetter interface {
	DSConfig() Config
	GetSpaceStorageMode() storage.SpaceStorageMode
}

type loggerWrapper struct {
	*logging.Sugared
}

func (l loggerWrapper) Warningf(template string, args ...interface{}) {
	l.Warnf(template, args...)
}

func init() {

	// used to store all objects tree changes + some metadata
	DefaultConfig.Spacestore.MemTableSize = 16 * 1024 * 1024     // Memtable saves all values below value threshold + write ahead log, actual file size is 2x the amount, the size is preallocated
	DefaultConfig.Spacestore.ValueLogFileSize = 64 * 1024 * 1024 // Vlog has all values more than value threshold, actual file uses 2x the amount, the size is preallocated
	DefaultConfig.Spacestore.ValueThreshold = 1024 * 128         // Object details should be small enough, e.g. under 10KB. 512KB here is just a precaution.
	DefaultConfig.Spacestore.Logger = loggerWrapper{logging.Logger("store.spacestore")}
	DefaultConfig.Spacestore.SyncWrites = false
	DefaultConfig.Spacestore.BlockCacheSize = 0
	DefaultConfig.Spacestore.Compression = options.None
}

func openBadgerWithRecover(opts badger.Options) (db *badger.DB, err error) {
	defer func() {
		// recover in case we have badger panic on open but not recovered by badger
		if r := recover(); r != nil {
			err = fmt.Errorf("badger panic: %v", r)
			if db != nil {
				db.Close()
			}
		}
	}()
	db, err = badger.Open(opts)
	return db, err
}

func isBadgerCorrupted(err error) bool {
	if strings.Contains(err.Error(), "checksum mismatch") {
		return true
	}
	if strings.Contains(err.Error(), "checksum is empty") {
		return true
	}
	if strings.Contains(err.Error(), "EOF") {
		return true
	}
	if strings.Contains(err.Error(), "file does not exist") {
		return true
	}
	if strings.Contains(err.Error(), "Unable to parse log") {
		return true
	}
	if strings.Contains(err.Error(), "Level validation err") {
		return true
	}
	if strings.Contains(err.Error(), "failed to read index") {
		return true
	}
	return false
}

func (r *clientds) Init(a *app.App) (err error) {
	// TODO: looks like we do a lot of stuff on Init here. We should consider moving it to the Run
	r.closing = make(chan struct{})
	r.syncerFinished = make(chan struct{})
	start := time.Now()
	wl := a.Component(wallet.CName)
	if wl == nil {
		return fmt.Errorf("need wallet to be inited first")
	}
	r.repoPath = wl.(wallet.Wallet).RepoPath()

	if cfgGetter, ok := a.Component("config").(DSConfigGetter); ok {
		r.cfg = cfgGetter.DSConfig()
		r.spaceStorageMode = cfgGetter.GetSpaceStorageMode()
	} else {
		return fmt.Errorf("" +
			"ds config is missing")
	}

	if _, err := os.Stat(r.getRepoPath(SpaceDSDir)); os.IsNotExist(err) {
		r.spaceStoreWasMissing = true
	}

	RemoveExpiredLocks(r.repoPath)

	if r.spaceStorageMode == storage.SpaceStorageModeBadger {
		opts := r.cfg.Spacestore
		opts.Dir = r.getRepoPath(SpaceDSDir)
		opts.ValueDir = opts.Dir
		r.spaceDS, err = openBadgerWithRecover(opts)
		if err != nil {
			err = anyerror.CleanupError(err)
			log.With("db", "spacestore").With("reset", false).With("err", err.Error()).Errorf("failed to open db")
			return err
		}
	}
	r.running = true
	r.spentOnInit = time.Since(start)
	return nil
}

func (r *clientds) Run(context.Context) error {
	go r.syncer()
	return nil
}

func (r *clientds) SpaceStorage() (*badger.DB, error) {
	// TODO: [MR] Change after testing
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	if r.spaceDS == nil {
		return nil, ErrSpaceStoreNotAvailable
	}
	return r.spaceDS, nil
}

func (r *clientds) Name() (name string) {
	return CName
}

func (r *clientds) Close(ctx context.Context) (err error) {
	close(r.closing)
	timeout := time.After(time.Minute)
	select {
	case <-r.syncerFinished:
	case <-timeout:
		return fmt.Errorf("sync time out")
	}
	// wait syncer goroutine to finish to make sure we don't have in-progress requests, because it may cause panics
	<-r.syncerFinished

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
		zap.Int("spaceStoreMode", int(r.spaceStorageMode)),
		zap.Int64("spentOnInit", r.spentOnInit.Milliseconds()),
	}
}
