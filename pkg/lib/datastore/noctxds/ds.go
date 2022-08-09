package noctxds

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"io"
)

type noCtxRead struct {
	ds ds.Read
}

func (n noCtxRead) Get(key ds.Key) (value []byte, err error) {
	return n.ds.Get(context.Background(), key)
}

func (n noCtxRead) Has(key ds.Key) (exists bool, err error) {
	return n.ds.Has(context.Background(), key)
}

func (n noCtxRead) GetSize(key ds.Key) (size int, err error) {
	return n.ds.GetSize(context.Background(), key)
}

func (n noCtxRead) Query(q query.Query) (query.Results, error) {
	return n.ds.Query(context.Background(), q)
}

type noCtxWrite struct {
	ds ds.Write
}

func (n noCtxWrite) Put(key ds.Key, value []byte) error {
	return n.ds.Put(context.Background(), key, value)
}

func (n noCtxWrite) Delete(key ds.Key) error {
	return n.ds.Delete(context.Background(), key)
}

func (n noCtxDs) Sync(prefix ds.Key) error {
	return n.ds.Sync(context.Background(), prefix)
}

type noCtxTxn struct {
	noCtxWrite
	noCtxRead
	Txn ds.Txn
}

func (n noCtxTxn) Commit() error {
	return n.Txn.Commit(context.Background())
}

func (n noCtxTxn) Discard() {
	n.Txn.Discard(context.Background())
}

type noCtxTxnDs struct {
	noCtxDs
	ds ds.TxnDatastore
}

func (n noCtxTxnDs) NewTransaction(readOnly bool) (Txn, error) {
	tx, err := n.ds.NewTransaction(context.Background(), readOnly)
	if err != nil {
		return nil, err
	}

	return &noCtxTxn{
		noCtxRead:  noCtxRead{tx},
		noCtxWrite: noCtxWrite{tx},
		Txn:        tx,
	}, nil
}

type noCtxBatch struct {
	noCtxWrite
	ds ds.Batch
}

func (n noCtxBatch) Commit() error {
	return n.ds.Commit(context.Background())
}

type noCtxDs struct {
	noCtxRead
	noCtxWrite
	ds ds.Datastore
}

type noCtxTxnDsBatching struct {
	noCtxTxnDs
	ds datastore.DSTxnBatching
}

func (n noCtxTxnDsBatching) Batch() (Batch, error) {
	b, err := n.ds.Batch(context.Background())
	if err != nil {
		return nil, err
	}
	return &noCtxBatch{noCtxWrite{b}, b}, nil
}

func New(ds datastore.DSTxnBatching) DSTxnBatching {
	return &noCtxTxnDsBatching{
		noCtxTxnDs: noCtxTxnDs{
			noCtxDs: noCtxDs{noCtxRead{ds: ds}, noCtxWrite{ds: ds}, ds},
			ds:      ds,
		},
		ds: ds,
	}
}

func (n noCtxTxnDsBatching) Close() error {
	return n.ds.Close()
}

type Write interface {
	Put(key ds.Key, value []byte) error
	Delete(key ds.Key) error
}

type Read interface {
	Get(key ds.Key) (value []byte, err error)
	Has(key ds.Key) (exists bool, err error)
	GetSize(key ds.Key) (size int, err error)
	Query(q query.Query) (query.Results, error)
}

type Datastore interface {
	Read
	Write

	Sync(prefix ds.Key) error
	io.Closer
}

type TxnDatastore interface {
	Datastore
	NewTransaction(readOnly bool) (Txn, error)
}

type Txn interface {
	Read
	Write

	Commit() error
	Discard()
}

type Batch interface {
	Write
	Commit() error
}

type DSTxnBatching interface {
	TxnDatastore
	Batch() (Batch, error)
}
