package clientds

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/hashicorp/go-multierror"
	ds "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	textileBadger "github.com/textileio/go-ds-badger"
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/textileio/go-threads/db/keytransform"
)

const (
	CName          = "datastore"
	liteDSDir      = "ipfslite"
	logstoreDSDir  = "logstore"
	threadsDbDSDir = "collection" + string(os.PathSeparator) + "eventstore"
)

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
