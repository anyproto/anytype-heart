package datastore

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v4"
)

const CName = "datastore"

type Datastore interface {
	app.ComponentRunnable
	SpaceStorage() (*badger.DB, error) // deprecated, should be moved to either LocalStorage or sqlite db
}

type inMemoryDatastore struct {
	db *badger.DB
}

// NewInMemory creates new in-memory store for testing purposes
func NewInMemory() (Datastore, error) {
	inm := &inMemoryDatastore{}
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	if err != nil {
		return nil, err
	}
	inm.db = db
	return inm, nil
}

func (i *inMemoryDatastore) Init(_ *app.App) error { return nil }

func (i *inMemoryDatastore) Name() string { return CName }

func (i *inMemoryDatastore) Run(ctx context.Context) error {
	return nil
}

func (i *inMemoryDatastore) Close(ctx context.Context) error {
	return i.db.Close()
}

func (i *inMemoryDatastore) SpaceStorage() (*badger.DB, error) {
	return i.db, nil
}
