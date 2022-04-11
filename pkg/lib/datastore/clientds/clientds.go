package clientds

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/multids"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	dsbadgerv3 "github.com/anytypeio/go-ds-badger3"
	dgraphbadgerv1 "github.com/dgraph-io/badger"
	dgraphbadgerv1pb "github.com/dgraph-io/badger/pb"
	"github.com/hashicorp/go-multierror"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	dsbadgerv1 "github.com/ipfs/go-ds-badger"
	textileBadger "github.com/textileio/go-ds-badger"
	"github.com/textileio/go-threads/db/keytransform"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

const (
	CName            = "datastore"
	liteOldDSDir     = "ipfslite" // used as a fallback for the existing repos
	liteDSDir        = "ipfslite_v3"
	logstoreOldDSDir = "logstore" // used for migration to the localstoreDSDir and then removed
	localstoreDSDir  = "localstore"
	threadsDbDSDir   = "collection" + string(os.PathSeparator) + "eventstore"

	valueLogExtenderKey  = "_extend"
	valueLogExtenderSize = 1024
)

var log = logging.Logger("anytype-clientds")

type clientds struct {
	running             bool
	litestoreOldDS      *dsbadgerv1.Datastore
	litestoreDS         *dsbadgerv3.Datastore
	litestoreCombinedDS ds.Batching

	logstoreOldDS *dsbadgerv1.Datastore // logstore moved to localstoreDS
	localstoreDS  *dsbadgerv3.Datastore
	threadsDbDS   *textileBadger.Datastore
	cfg           Config
	repoPath      string
	migrations    []migration
}

type Config struct {
	Litestore    dsbadgerv3.Options
	LitestoreOld dsbadgerv1.Options
	LogstoreOld  dsbadgerv1.Options

	Localstore dsbadgerv3.Options
	TextileDb  dsbadgerv1.Options
}

var DefaultConfig = Config{
	Litestore:    dsbadgerv3.DefaultOptions,
	LitestoreOld: dsbadgerv1.DefaultOptions,
	LogstoreOld:  dsbadgerv1.DefaultOptions,
	TextileDb:    dsbadgerv1.DefaultOptions,
	Localstore:   dsbadgerv3.DefaultOptions,
}

type DSConfigGetter interface {
	DSConfig() Config
}

type migration struct {
	migrationFunc func() error
	migrationKey  ds.Key
}

func init() {

	// lets set badger options inside the init, otherwise we need to directly import the badger intp MW
	DefaultConfig.LogstoreOld.ValueLogFileSize = 64 * 1024 * 1024 // Badger will rotate value log files after 64MB. GC only works starting from the 2nd value log file
	DefaultConfig.LogstoreOld.GcDiscardRatio = 0.2                // allow up to 20% value log overhead
	DefaultConfig.LogstoreOld.GcInterval = time.Minute * 10       // run GC every 10 minutes
	DefaultConfig.LogstoreOld.GcSleep = time.Second * 5           // sleep between rounds of one GC cycle(it has multiple rounds within one cycle)
	DefaultConfig.LogstoreOld.ValueThreshold = 1024               // store up to 1KB of value within the LSM tree itself to speed-up details filter queries
	DefaultConfig.LogstoreOld.Logger = logging.Logger("badger-logstore-old")

	// used to store objects localstore + threads logs info
	DefaultConfig.Localstore.MemTableSize = 16 * 1024 * 1024     // Memtable saves all values below value threshold + write ahead log, actual file size is 2x the amount, the size is preallocated
	DefaultConfig.Localstore.ValueLogFileSize = 16 * 1024 * 1024 // Vlog has all values more than value threshold, actual file uses 2x the amount, the size is preallocated
	DefaultConfig.Localstore.GcDiscardRatio = 0.2                // allow up to 20% value log overhead
	DefaultConfig.Localstore.GcInterval = time.Minute * 10       // run GC every 10 minutes
	DefaultConfig.Localstore.GcSleep = time.Second * 5           // sleep between rounds of one GC cycle(it has multiple rounds within one cycle)
	DefaultConfig.Localstore.ValueThreshold = 1024               // store up to 1KB of value within the LSM tree itself to speed-up details filter queries
	DefaultConfig.Localstore.Logger = logging.Logger("badger-localstore")
	DefaultConfig.Localstore.SyncWrites = false

	DefaultConfig.Litestore.MemTableSize = 64 * 1024 * 1024     // Memtable saves all values below value threshold + write ahead log, actual file size is 2x the amount, the size is preallocated
	DefaultConfig.Litestore.ValueLogFileSize = 64 * 1024 * 1024 // Vlog has all values more than value threshold, actual file uses 2x the amount, the size is preallocated
	DefaultConfig.Litestore.GcDiscardRatio = 0.2                // allow up to 20% value log overhead
	DefaultConfig.Litestore.GcInterval = time.Minute * 10       // run GC every 10 minutes
	DefaultConfig.Litestore.GcSleep = time.Second * 5           // sleep between rounds of one GC cycle(it has multiple rounds within one cycle)
	DefaultConfig.Litestore.ValueThreshold = 1024
	DefaultConfig.Litestore.Logger = logging.Logger("badger-litestore")
	DefaultConfig.Litestore.SyncWrites = false

	DefaultConfig.LitestoreOld.Logger = logging.Logger("badger-litestore-old")
	DefaultConfig.LitestoreOld.ValueLogFileSize = 64 * 1024 * 1024
	DefaultConfig.LitestoreOld.GcDiscardRatio = 0.1

	DefaultConfig.TextileDb.Logger = logging.Logger("badger-textiledb")
	// we don't need to tune litestore&threadsDB badger instances because they should be fine with defaults for now
}

func (r *clientds) Init(a *app.App) (err error) {
	wl := a.Component(wallet.CName)
	if wl == nil {
		return fmt.Errorf("need wallet to be inited first")
	}

	if cfgGetter, ok := a.Component("config").(DSConfigGetter); ok {
		r.cfg = cfgGetter.DSConfig()
	} else {
		return fmt.Errorf("ds config is missing")
	}

	r.repoPath = wl.(wallet.Wallet).RepoPath()
	r.migrations = []migration{
		{
			migrationFunc: r.migrateLocalStoreBadger,
			migrationKey:  ds.NewKey("/migration/localstore/badgerv3"),
		},
		{
			migrationFunc: r.migrateFileStoreAndIndexesBadger,
			migrationKey:  ds.NewKey("/migration/localstore/badgerv3/filesindexes"),
		},
		{
			migrationFunc: r.migrateLogstore,
			migrationKey:  ds.NewKey("/migration/logstore/badgerv3"),
		},
	}
	return nil
}

func (r *clientds) Run() error {
	var err error

	r.litestoreDS, err = dsbadgerv3.NewDatastore(filepath.Join(r.repoPath, liteDSDir), &r.cfg.Litestore)
	if err != nil {
		return err
	}

	litestoreOldPath := filepath.Join(r.repoPath, liteOldDSDir)
	if _, err := os.Stat(litestoreOldPath); !os.IsNotExist(err) {
		r.litestoreOldDS, err = dsbadgerv1.NewDatastore(litestoreOldPath, &r.cfg.LitestoreOld)
		if err != nil {
			return err
		}

		r.litestoreCombinedDS = multids.New(r.litestoreDS, r.litestoreOldDS)
	}

	logstoreOldDSDirPath := filepath.Join(r.repoPath, logstoreOldDSDir)
	if _, err := os.Stat(logstoreOldDSDirPath); !os.IsNotExist(err) {
		r.logstoreOldDS, err = dsbadgerv1.NewDatastore(logstoreOldDSDirPath, &r.cfg.LogstoreOld)
		if err != nil {
			return err
		}
	}

	r.localstoreDS, err = dsbadgerv3.NewDatastore(filepath.Join(r.repoPath, localstoreDSDir), &r.cfg.Localstore)
	if err != nil {
		return err
	}

	err = r.migrateIfNeeded()
	if err != nil {
		return fmt.Errorf("migrateIfNeeded failed: %w", err)
	}

	threadsDbOpts := textileBadger.Options(r.cfg.TextileDb)
	tdbPath := filepath.Join(r.repoPath, threadsDbDSDir)
	err = os.MkdirAll(tdbPath, os.ModePerm)
	if err != nil {
		return err
	}

	r.threadsDbDS, err = textileBadger.NewDatastore(filepath.Join(r.repoPath, threadsDbDSDir), &threadsDbOpts)
	if err != nil {
		return err
	}
	r.running = true
	return nil
}

func (r *clientds) migrateIfNeeded() error {
	for _, m := range r.migrations {
		_, err := r.localstoreDS.Get(m.migrationKey)
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
		err = r.localstoreDS.Put(m.migrationKey, nil)
		if err != nil {
			return fmt.Errorf("failed to put %s migration key into db: %w", m.migrationKey.String(), err)
		}
	}
	return nil
}

func (r *clientds) migrateWithKey(from *dsbadgerv1.Datastore, to *dsbadgerv3.Datastore, chooseKey func(item *dgraphbadgerv1.Item) bool) error {
	if from == nil {
		return fmt.Errorf("from ds is nil")
	}
	s := from.DB.NewStream()
	s.ChooseKey = chooseKey
	s.Send = func(list *dgraphbadgerv1pb.KVList) error {
		batch, err := to.Batch()
		if err != nil {
			return err
		}
		for _, kv := range list.Kv {
			err := batch.Put(ds.NewKey(string(kv.Key)), kv.Value)
			if err != nil {
				return err
			}
		}
		return batch.Commit()
	}
	return s.Orchestrate(context.Background())
}

func (r *clientds) migrateLocalStoreBadger() error {
	if r.logstoreOldDS == nil {
		return nil
	}
	return r.migrateWithKey(r.logstoreOldDS, r.localstoreDS, func(item *dgraphbadgerv1.Item) bool {
		keyString := string(item.Key())
		res := strings.HasPrefix(keyString, "/pages") ||
			strings.HasPrefix(keyString, "/workspaces") ||
			strings.HasPrefix(keyString, "/relations")
		return res
	})
}

func (r *clientds) migrateFileStoreAndIndexesBadger() error {
	if r.logstoreOldDS == nil {
		return nil
	}
	return r.migrateWithKey(r.logstoreOldDS, r.localstoreDS, func(item *dgraphbadgerv1.Item) bool {
		keyString := string(item.Key())
		return strings.HasPrefix(keyString, "/files") || strings.HasPrefix(keyString, "/idx")
	})
}

func (r *clientds) migrateLogstore() error {
	if r.logstoreOldDS == nil {
		return nil
	}
	return r.migrateWithKey(r.logstoreOldDS, r.localstoreDS, func(item *dgraphbadgerv1.Item) bool {
		keyString := string(item.Key())
		return strings.HasPrefix(keyString, "/thread")
	})
}

type ValueLogInfo struct {
	Index int64
	Size  int64
}

func (r *clientds) RunBlockstoreGC() (freed int64, err error) {
	if r.litestoreOldDS != nil {
		freed1, err := runBlockstoreGC(filepath.Join(r.repoPath, liteOldDSDir), r.litestoreOldDS, DefaultConfig.LitestoreOld.ValueLogFileSize)
		if err != nil {
			return 0, err
		}
		freed += freed1
	}

	if r.litestoreDS != nil {
		freed2, err := runBlockstoreGC(filepath.Join(r.repoPath, liteDSDir), r.litestoreDS, DefaultConfig.Litestore.ValueLogFileSize)
		if err != nil {
			return 0, err
		}
		freed += freed2
	}

	return
}

func runBlockstoreGC(dsPath string, dsInstance ds.Datastore, valueLogSize int64) (freed int64, err error) {
	getValueLogsInfo := func() (totalSize int64, valLogs []*ValueLogInfo, err error) {
		err = filepath.Walk(dsPath, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			ext := filepath.Ext(info.Name())
			switch ext {
			case ".vlog":
				index, err := strconv.ParseInt(strings.TrimSuffix(info.Name(), ext), 10, 64)
				if err != nil {
					return nil
				}
				valLogs = append(valLogs, &ValueLogInfo{Index: index, Size: info.Size()})
				totalSize += info.Size()
			}
			return nil
		})
		if err != nil {
			return
		}

		sort.Slice(valLogs, func(i, j int) bool {
			return valLogs[i].Index < valLogs[j].Index
		})
		return totalSize, valLogs, nil
	}

	totalSizeBefore, valLogs, err := getValueLogsInfo()
	if err != nil {
		return 0, err
	}
	log.With("vlogs_count", len(valLogs)).With("vlogs", valLogs).Infof("GC: before the cycle")

	if len(valLogs) == 0 {
		return 0, nil
	}

	if valLogs[len(valLogs)-1].Size > valueLogSize {
		// in case we have the last value log exceeding the max value log size
		v := make([]byte, valueLogExtenderSize)
		_ = dsInstance.Put(ds.NewKey(valueLogExtenderKey), v)
	}

	var total int
	var maxErrors = 1
	for {
		if v1, ok := dsInstance.(*dsbadgerv1.Datastore); ok {
			err = v1.DB.RunValueLogGC(0.000000000001)
		} else if v3, ok := dsInstance.(*dsbadgerv3.Datastore); ok {
			err = v3.DB.RunValueLogGC(0.000000000001)
		} else {
			panic("badger version unsupported")
		}
		// set the discard ratio to the lowest value means we want to rewrite value log if we have any values removed
		if err != nil && err.Error() == "Value log GC attempt didn't result in any cleanup" {
			maxErrors--
			if maxErrors == 0 {
				log.Infof("badger gc exit on %d attempt", total)
				break
			}
			continue
		}
		total++
	}

	totalSizeAfter, vlogsAfter, err := getValueLogsInfo()

	results, err := dsInstance.Query(query.Query{Limit: 0, KeysOnly: true, ReturnsSizes: true})
	var (
		keysTotal     int64
		keysTotalSize int64
	)

	for result := range results.Next() {
		keysTotal++
		keysTotalSize += int64(result.Size)
	}

	freed = totalSizeBefore - totalSizeAfter
	if totalSizeAfter > keysTotalSize {
		log.With("vlogs_count", len(vlogsAfter)).With("vlogs_freed_b", freed).With("keys_size_b", keysTotalSize).With("vlog_overhead_b", totalSizeAfter-keysTotalSize).With("vlogs", vlogsAfter).Errorf("Badger GC: got badger value logs overhead after GC")
	}
	if freed < 0 {
		freed = 0
	}
	return freed, nil
}

func (r *clientds) PeerstoreDS() (ds.Batching, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}

	if r.litestoreCombinedDS != nil {
		return r.litestoreCombinedDS, nil
	}

	return r.litestoreDS, nil
}

func (r *clientds) BlockstoreDS() (ds.Batching, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}

	if r.litestoreCombinedDS != nil {
		return r.litestoreCombinedDS, nil
	}

	return r.litestoreDS, nil
}

func (r *clientds) LogstoreDS() (datastore.DSTxnBatching, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.localstoreDS, nil
}

func (r *clientds) ThreadsDbDS() (keytransform.TxnDatastoreExtended, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.threadsDbDS, nil
}

func (r *clientds) LocalstoreDS() (datastore.DSTxnBatching, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.localstoreDS, nil
}

func (r *clientds) Name() (name string) {
	return CName
}

func (r *clientds) Close() (err error) {
	if r.litestoreDS != nil {
		err2 := r.litestoreDS.Close()
		if err2 != nil {
			err = multierror.Append(err, err2)
		}
	}

	if r.localstoreDS != nil {
		err2 := r.localstoreDS.Close()
		if err2 != nil {
			err = multierror.Append(err, err2)
		}
	}

	if r.threadsDbDS != nil {
		err2 := r.threadsDbDS.Close()
		if err2 != nil {
			err = multierror.Append(err, err2)
		}
	}

	return err
}

func New() datastore.Datastore {
	return &clientds{}
}
