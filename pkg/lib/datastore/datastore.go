package datastore

import (
	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v3"
)

const CName = "datastore"

type Datastore interface {
	app.ComponentRunnable
	SpaceStorage() (*badger.DB, error)
	LocalStorage() (*badger.DB, error)
}
