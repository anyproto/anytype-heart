package filestorage

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anytypeio/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	flatfs "github.com/ipfs/go-ds-flatfs"
	format "github.com/ipfs/go-ipld-format"
)

type flatStore struct {
	ds *flatfs.Datastore
}

func newFlatStore(path string) (*flatStore, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf("mkdir: %w", err)
		}
	}
	ds, err := flatfs.CreateOrOpen(path, flatfs.IPFS_DEF_SHARD, false)
	if err != nil {
		return nil, err
	}
	return &flatStore{ds: ds}, nil
}

func (f *flatStore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	raw, err := f.ds.Get(ctx, dskey(k))
	if err == datastore.ErrNotFound {
		return nil, &format.ErrNotFound{Cid: k}
	}
	if err != nil {
		return nil, err
	}

	return blocks.NewBlockWithCid(raw, k)
}

func (f *flatStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	ch := make(chan blocks.Block)
	go func() {
		defer close(ch)
		for _, k := range ks {
			b, err := f.Get(ctx, k)
			if err != nil {
				// TODO proper logging
				fmt.Println("GetMany: ", k, err)
				continue
			}
			ch <- b
		}
	}()
	return ch
}

func dskey(c cid.Cid) datastore.Key {
	return datastore.NewKey(strings.ToUpper(c.String()))
}

func (f *flatStore) Add(ctx context.Context, bs []blocks.Block) error {
	for _, b := range bs {
		if err := f.ds.Put(ctx, dskey(b.Cid()), b.RawData()); err != nil {
			return fmt.Errorf("put %s: %w", dskey(b.Cid()), err)
		}
	}
	return nil
}

func (f *flatStore) Delete(ctx context.Context, c cid.Cid) error {
	return f.ds.Delete(ctx, dskey(c))
}

func (f *flatStore) ExistsCids(ctx context.Context, ks []cid.Cid) (exist []cid.Cid, err error) {
	for _, k := range ks {
		ok, err := f.ds.Has(ctx, dskey(k))
		if err != nil {
			return nil, err
		}
		if ok {
			exist = append(exist, k)
		}
	}
	return
}

func (f *flatStore) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExist []blocks.Block, err error) {
	for _, b := range bs {
		ok, err := f.ds.Has(ctx, dskey(b.Cid()))
		if err != nil {
			return nil, err
		}
		if !ok {
			notExist = append(notExist, b)
		}
	}
	return
}

func (f *flatStore) BlockAvailability(ctx context.Context, ks []cid.Cid) (availability []*fileproto.BlockAvailability, err error) {
	for _, k := range ks {
		ok, err := f.ds.Has(ctx, dskey(k))
		if err != nil {
			return nil, err
		}
		status := fileproto.AvailabilityStatus_NotExists
		if ok {
			status = fileproto.AvailabilityStatus_Exists
		}
		availability = append(availability, &fileproto.BlockAvailability{
			Cid:    k.Bytes(),
			Status: status,
		})
	}
	return
}
