package multids

import (
	"github.com/hashicorp/go-multierror"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
)

type multiDs struct {
	oldDs ds.Batching
	newDs ds.Batching
}

type multiBatch struct {
	oldDs ds.Batch
	newDs ds.Batch
}

func (d multiDs) Get(key ds.Key) (value []byte, err error) {
	s, err := d.newDs.Get(key)
	if err == ds.ErrNotFound {
		return d.oldDs.Get(key)
	}

	return s, err

}

func (d multiDs) Has(key ds.Key) (exists bool, err error) {
	if exists, err = d.newDs.Has(key); err != nil || exists {
		return exists, err
	} else {
		return d.oldDs.Has(key)
	}
}

func (d multiDs) GetSize(key ds.Key) (size int, err error) {
	if s, err := d.newDs.GetSize(key); err == ds.ErrNotFound {
		return d.oldDs.GetSize(key)
	} else {
		return s, err
	}
}

func (d multiDs) Query(q query.Query) (query.Results, error) {
	res1, err := d.newDs.Query(q)
	if err != nil {
		return nil, err
	}
	res2, err := d.oldDs.Query(q)
	if err != nil {
		return nil, err
	}
	return newResultsCombiner(res1, res2), nil
}

func (d multiDs) Put(key ds.Key, value []byte) error {
	return d.newDs.Put(key, value)
}

func (d multiDs) Delete(key ds.Key) error {
	if err := d.newDs.Delete(key); err == ds.ErrNotFound {
		return d.oldDs.Delete(key)
	} else {
		return err
	}
}

func (d multiDs) Sync(prefix ds.Key) error {
	err := d.newDs.Sync(prefix)
	if err != nil {
		return err
	}
	return d.oldDs.Sync(prefix)
}

func (d multiDs) Close() error {
	err1 := d.oldDs.Close()
	err := d.newDs.Close()
	if err != nil {
		return err
	}

	return err1
}

func (d multiDs) Batch() (ds.Batch, error) {
	oldBatch, err := d.oldDs.Batch()
	if err != nil {
		return nil, err
	}
	newBatch, err := d.newDs.Batch()
	if err != nil {
		return nil, err
	}
	return newMultiBatch(newBatch, oldBatch), nil
}

func New(newDs ds.Batching, oldDs ds.Batching) ds.Batching {
	return &multiDs{oldDs: oldDs, newDs: newDs}
}

func newMultiBatch(newDs ds.Batch, oldDs ds.Batch) ds.Batch {
	return &multiBatch{oldDs: oldDs, newDs: newDs}
}

func (m multiBatch) Put(key ds.Key, value []byte) error {
	// put only to the new ds
	return m.newDs.Put(key, value)
}

func (m multiBatch) Delete(key ds.Key) error {
	var err error

	err = m.oldDs.Delete(key)
	if err != nil {
		return err
	}

	err = m.newDs.Delete(key)
	if err != nil {
		return err
	}

	return nil
}

func (m multiBatch) Commit() error {
	var err1, err2 error
	err1 = m.oldDs.Commit()
	err2 = m.newDs.Commit()

	if err1 != nil || err2 != nil {
		merr := multierror.Error{Errors: []error{err1, err2}}
		return merr.ErrorOrNil()
	}

	return nil
}
