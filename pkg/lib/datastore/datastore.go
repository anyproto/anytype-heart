package datastore

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	ds "github.com/ipfs/go-datastore"
	"github.com/textileio/go-threads/db/keytransform"
)

const CName = "datastore"

type Datastore interface {
	app.ComponentRunnable
	PeerstoreDS() (ds.Batching, error)
	BlockstoreDS() (ds.Batching, error)
	LogstoreDS() (DSTxnBatching, error)
	LocalstoreDS() (ds.TxnDatastore, error)
	ThreadsDbDS() (keytransform.TxnDatastoreExtended, error)
}

type DSTxnBatching interface {
	ds.TxnDatastore
	Batch() (ds.Batch, error)
}
