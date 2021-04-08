package datastore

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	ds "github.com/ipfs/go-datastore"
	"github.com/textileio/go-threads/db/keytransform"
)

const CName = "datastore"

type Datastore interface {
	app.ComponentRunnable
	PeerstoreDS() ds.Batching
	BlockstoreDS() ds.Batching
	LogstoreDS() DSTxnBatching
	LocalstoreDS() ds.TxnDatastore
	ThreadsDbDS() keytransform.TxnDatastoreExtended
}

type DSTxnBatching interface {
	ds.TxnDatastore
	Batch() (ds.Batch, error)
}
