package nocloserds

import (
	"github.com/ipfs/go-datastore"
	"github.com/textileio/go-threads/db/keytransform"
)

type NoCloser struct{}

type NoCloserDatastoreBatching struct {
	datastore.Batching
}

type NoCloserDatastoreTxnBatching struct {
	DatastoreTxnBatching
}

type NoCloserDatastoreExtended struct {
	keytransform.TxnDatastoreExtended
}

type DatastoreTxnBatching interface {
	datastore.TxnDatastore
	Batch() (datastore.Batch, error)
}

func NewBatch(ds datastore.Batching) datastore.Batching {
	return &NoCloserDatastoreBatching{ds}
}

func NewTxnBatch(ds DatastoreTxnBatching) DatastoreTxnBatching {
	return &NoCloserDatastoreTxnBatching{ds}
}

func NewTxnExtended(ds keytransform.TxnDatastoreExtended) keytransform.TxnDatastoreExtended {
	return &NoCloserDatastoreExtended{ds}
}

func (ncd NoCloserDatastoreBatching) Close() error {
	return nil
}

func (ncd NoCloserDatastoreTxnBatching) Close() error {
	return nil
}

func (ncd NoCloserDatastoreExtended) Close() error {
	return nil
}
