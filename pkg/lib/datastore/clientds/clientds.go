package clientds

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/ipfs/go-datastore/query"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/hashicorp/go-multierror"
	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	textileBadger "github.com/textileio/go-ds-badger"
	"github.com/textileio/go-threads/db/keytransform"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
)

const (
	CName          = "datastore"
	liteDSDir      = "ipfslite"
	logstoreDSDir  = "logstore"
	threadsDbDSDir = "collection" + string(os.PathSeparator) + "eventstore"

	valueLogExtenderKey  = "_extend"
	valueLogExtenderSize = 1024
)

var log = logging.Logger("anytype-clientds")

type clientds struct {
	running     bool
	litestoreDS *badger.Datastore
	logstoreDS  *badger.Datastore
	threadsDbDS *textileBadger.Datastore
	cfg         Config
	repoPath    string
}

type Config struct {
	Litestore badger.Options
	Logstore  badger.Options
	TextileDb badger.Options
}

var DefaultConfig = Config{
	Litestore: badger.DefaultOptions,
	Logstore:  badger.DefaultOptions,
	TextileDb: badger.DefaultOptions,
}

type DSConfigGetter interface {
	DSConfig() Config
}

func init() {
	// lets set badger options inside the init, otherwise we need to directly import the badger intp MW
	DefaultConfig.Logstore.ValueLogFileSize = 64 * 1024 * 1024 // Badger will rotate value log files after 64MB. GC only works starting from the 2nd value log file
	DefaultConfig.Logstore.GcDiscardRatio = 0.2                // allow up to 20% value log overhead
	DefaultConfig.Logstore.GcInterval = time.Minute * 10       // run GC every 10 minutes
	DefaultConfig.Logstore.GcSleep = time.Second * 5           // sleep between rounds of one GC cycle(it has multiple rounds within one cycle)
	DefaultConfig.Logstore.ValueThreshold = 1024               // store up to 1KB of value within the LSM tree itself to speed-up details filter queries
	DefaultConfig.Logstore.Logger = logging.Logger("badger-logstore")
	DefaultConfig.Litestore.Logger = logging.Logger("badger-litestore")
	DefaultConfig.Litestore.ValueLogFileSize = 64 * 1024 * 1024
	DefaultConfig.Litestore.GcDiscardRatio = 0.1
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
	return nil
}

func (r *clientds) Run() error {
	var err error
	r.litestoreDS, err = badger.NewDatastore(filepath.Join(r.repoPath, liteDSDir), &r.cfg.Litestore)
	if err != nil {
		return err
	}

	r.logstoreDS, err = badger.NewDatastore(filepath.Join(r.repoPath, logstoreDSDir), &r.cfg.Logstore)
	if err != nil {
		return err
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

type ValueLogInfo struct {
	Index int64
	Size  int64
}

func (r *clientds) RunBlockstoreGC() (freed int64, err error) {
	getValueLogsInfo := func() (totalSize int64, valLogs []*ValueLogInfo, err error) {
		err = filepath.Walk(filepath.Join(r.repoPath, liteDSDir), func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			ext := filepath.Ext(info.Name())
			switch ext {
			case ".vlog":
				index, err := strconv.ParseInt(info.Name(), 10, 64)
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

	if valLogs[len(valLogs)-1].Size > DefaultConfig.Litestore.ValueLogFileSize {
		// in case we have the last value log exceeding the max value log size
		v := make([]byte, valueLogExtenderSize)
		r.litestoreDS.Put(ds.NewKey(valueLogExtenderKey), v)
	}

	var total int
	var maxErrors = 1
	for {
		// set the discard ratio to the lowest value means we want to rewrite value log if we have any values removed
		err = r.litestoreDS.DB.RunValueLogGC(0.000000000001)
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

	results, err := r.litestoreDS.Query(query.Query{Limit: 0, KeysOnly: true, ReturnsSizes: true})
	var (
		keysTotal     int64
		keysTotalSize int64
	)

	for result := range results.Next() {
		keysTotal++
		keysTotalSize += int64(result.Size)
	}

	if keysTotalSize > totalSizeAfter /* + DefaultConfig.Litestore.ValueLogFileSize*/ {
		log.With("vlogs_count", len(vlogsAfter)).With("keys_size_b", keysTotalSize).With("vlog_overhead_b", totalSizeAfter-keysTotalSize).With("vlogs", vlogsAfter).Errorf("Badger GC: got badger value logs overhead after GC")
	}
	return totalSizeAfter - totalSizeBefore, nil
}

func (r *clientds) PeerstoreDS() (ds.Batching, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.litestoreDS, nil
}

func (r *clientds) BlockstoreDS() (ds.Batching, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.litestoreDS, nil
}

func (r *clientds) LogstoreDS() (datastore.DSTxnBatching, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.logstoreDS, nil
}

func (r *clientds) ThreadsDbDS() (keytransform.TxnDatastoreExtended, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.threadsDbDS, nil
}

func (r *clientds) LocalstoreDS() (ds.TxnDatastore, error) {
	if !r.running {
		return nil, fmt.Errorf("exact ds may be requested only after Run")
	}
	return r.logstoreDS, nil
}

func (r *clientds) Name() (name string) {
	return CName
}

func (r *clientds) Close() (err error) {
	if r.logstoreDS != nil {
		err2 := r.logstoreDS.Close()
		if err2 != nil {
			err = multierror.Append(err, err2)
		}
	}

	if r.litestoreDS != nil {
		err2 := r.litestoreDS.Close()
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
