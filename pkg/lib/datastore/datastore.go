package datastore

import (
	"context"

	"github.com/anytypeio/any-sync/app"
	"github.com/dgraph-io/badger/v3"
	ds "github.com/ipfs/go-datastore"
)

const CName = "datastore"

type Datastore interface {
	app.ComponentRunnable
	LocalstoreDS() (DSTxnBatching, error)
	SpaceStorage() (*badger.DB, error)
}

type DSTxnBatching interface {
	ds.TxnDatastore
	Batch(ctx context.Context) (ds.Batch, error)
}
