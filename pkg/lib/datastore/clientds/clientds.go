package clientds

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/hashicorp/go-multierror"
	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	textileBadger "github.com/textileio/go-ds-badger"
	"os"

	"github.com/textileio/go-threads/db/keytransform"
	"path/filepath"

	"github.com/anytypeio/go-anytype-middleware/app"
	datastore2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
)

const (
	CName          = "datastore"
	liteDSDir      = "ipfslite"
	logstoreDSDir  = "logstore"
	threadsDbDSDir = "collection" + string(os.PathSeparator) + "eventstore"
)

type datastore struct {
	litestoreDS *badger.Datastore
	logstoreDS  *badger.Datastore
	threadsDbDS *textileBadger.Datastore
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

func (r *datastore) Init(a *app.App) (err error) {
	wl := a.Component(wallet.CName)
	if wl == nil {
		return fmt.Errorf("need wallet to be inited first")
	}

	var cfg Config

	if cfgGetter, ok := a.Component("config").(DSConfigGetter); ok {
		cfg = cfgGetter.DSConfig()
	} else {
		return fmt.Errorf("ds config is missing")
	}

	repoPath := wl.(wallet.Wallet).RepoPath()
	r.litestoreDS, err = badger.NewDatastore(filepath.Join(repoPath, liteDSDir), &cfg.Litestore)
	if err != nil {
		return err
	}

	r.logstoreDS, err = badger.NewDatastore(filepath.Join(repoPath, logstoreDSDir), &cfg.Logstore)
	if err != nil {
		return err
	}

	threadsDbOpts := textileBadger.Options(cfg.TextileDb)
	tdbPath := filepath.Join(repoPath, threadsDbDSDir)
	err = os.MkdirAll(tdbPath, os.ModePerm)
	if err != nil {
		return err
	}

	r.threadsDbDS, err = textileBadger.NewDatastore(filepath.Join(repoPath, threadsDbDSDir), &threadsDbOpts)
	if err != nil {
		return err
	}

	return nil
}

func (r *datastore) Run() error {
	// we can't move badger init here, because it will require other pkgs to depend on getter iterfaces
	return nil
}

func (r *datastore) PeerstoreDS() ds.Batching {
	return r.litestoreDS
}

func (r *datastore) BlockstoreDS() ds.Batching {
	return r.litestoreDS
}

func (r *datastore) LogstoreDS() datastore2.DSTxnBatching {
	return r.logstoreDS
}

func (r *datastore) ThreadsDbDS() keytransform.TxnDatastoreExtended {
	return r.threadsDbDS
}

func (r *datastore) LocalstoreDS() ds.TxnDatastore {
	return r.logstoreDS
}

func (r *datastore) Name() (name string) {
	return CName
}

func (r *datastore) Close() (err error) {
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

func New() datastore2.Datastore {
	return &datastore{}
}
