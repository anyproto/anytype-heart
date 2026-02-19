package filestorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	flatfs "github.com/ipfs/go-ds-flatfs"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

type flatStore struct {
	ds                         *flatfs.Datastore
	localBytesUsageEventSender *localBytesUsageEventSender
}

func newFlatStore(path string, eventSender event.Sender, sendEventBatchTimeout time.Duration) (*flatStore, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf("mkdir: %w", err)
		}
	}
	ds, err := flatfs.CreateOrOpen(path, flatfs.IPFS_DEF_SHARD, false)
	if err != nil {
		return nil, err
	}

	bytesUsage, err := ds.DiskUsage(context.Background())
	if err != nil {
		log.Error("can't get initial disk usage", zap.Error(err))
	}
	return &flatStore{
		ds:                         ds,
		localBytesUsageEventSender: newLocalBytesUsageEventSender(eventSender, sendEventBatchTimeout, bytesUsage),
	}, nil
}

func (f *flatStore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	raw, err := f.ds.Get(ctx, flatStoreKey(k))
	if errors.Is(err, datastore.ErrNotFound) {
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
				log.Error("localStore.GetMany", zap.Error(err))
				continue
			}
			select {
			case <-ctx.Done():
				return
			case ch <- b:
			}
		}
	}()
	return ch
}

func flatStoreKey(c cid.Cid) datastore.Key {
	return datastore.NewKey(strings.ToUpper(c.String()))
}

func (f *flatStore) Add(ctx context.Context, bs []blocks.Block) error {
	for _, b := range bs {
		if err := f.ds.Put(ctx, flatStoreKey(b.Cid()), b.RawData()); err != nil {
			return fmt.Errorf("put %s: %w", flatStoreKey(b.Cid()), err)
		}
	}
	f.sendLocalBytesUsageEvent(ctx)
	return nil
}

func (f *flatStore) Delete(ctx context.Context, c cid.Cid) error {
	err := f.ds.Delete(ctx, flatStoreKey(c))
	if err != nil {
		return err
	}
	f.sendLocalBytesUsageEvent(ctx)
	return nil
}

func (f *flatStore) sendLocalBytesUsageEvent(ctx context.Context) {
	du, err := f.ds.DiskUsage(ctx)
	if err == nil {
		f.localBytesUsageEventSender.sendLocalBytesUsageEvent(du)
	}
}

func (f *flatStore) PartitionByExistence(ctx context.Context, ks []cid.Cid) (exist []cid.Cid, notExist []cid.Cid, err error) {
	for _, k := range ks {
		ok, err := f.ds.Has(ctx, flatStoreKey(k))
		if err != nil {
			return nil, nil, err
		}
		if ok {
			exist = append(exist, k)
		} else {
			notExist = append(notExist, k)
		}
	}
	return
}

func (f *flatStore) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	for _, k := range ks {
		ok, err := f.ds.Has(ctx, flatStoreKey(k))
		if err != nil {
			return nil, err
		}
		if ok {
			exists = append(exists, k)
		}
	}
	return exists, nil
}

func (f *flatStore) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExist []blocks.Block, err error) {
	for _, b := range bs {
		ok, err := f.ds.Has(ctx, flatStoreKey(b.Cid()))
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
		ok, err := f.ds.Has(ctx, flatStoreKey(k))
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

func (f *flatStore) Close() error {
	return f.ds.Close()
}

func (f *flatStore) Batch(ctx context.Context) (BlockStoreBatch, error) {
	dsBatch, err := f.ds.Batch(ctx)
	if err != nil {
		return nil, err
	}
	// Cast to BatchReader which is implemented by the anyproto fork
	batchReader, ok := dsBatch.(flatfs.BatchReader)
	if !ok {
		return nil, fmt.Errorf("batch does not implement BatchReader interface")
	}
	return &flatStoreBatch{
		store:   f,
		dsBatch: batchReader,
	}, nil
}

type flatStoreBatch struct {
	store   *flatStore
	dsBatch flatfs.BatchReader
}

// Get reads from batch (checks temp dir first, then falls back to main dir via BatchReader)
func (b *flatStoreBatch) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	// The BatchReader interface from anyproto fork supports Get which checks temp then main
	raw, err := b.dsBatch.Get(ctx, flatStoreKey(k))
	if errors.Is(err, datastore.ErrNotFound) {
		return nil, &format.ErrNotFound{Cid: k}
	}
	if err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(raw, k)
}

// GetMany reads multiple blocks from batch
func (b *flatStoreBatch) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	ch := make(chan blocks.Block)
	go func() {
		defer close(ch)
		for _, k := range ks {
			blk, err := b.Get(ctx, k)
			if err == nil {
				ch <- blk
			}
		}
	}()
	return ch
}

// Add adds blocks to the batch (writes to temp directory)
func (b *flatStoreBatch) Add(ctx context.Context, bs []blocks.Block) error {
	for _, block := range bs {
		if err := b.dsBatch.Put(ctx, flatStoreKey(block.Cid()), block.RawData()); err != nil {
			return fmt.Errorf("batch put %s: %w", flatStoreKey(block.Cid()), err)
		}
	}
	return nil
}

// Delete deletes from the batch
func (b *flatStoreBatch) Delete(ctx context.Context, c cid.Cid) error {
	return b.dsBatch.Delete(context.Background(), flatStoreKey(c))
}

// ExistsCids checks if cids exist (in temp or main storage)
func (b *flatStoreBatch) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	for _, k := range ks {
		// BatchReader.Has checks both temp and main directories
		ok, err := b.dsBatch.Has(ctx, flatStoreKey(k))
		if err != nil {
			return nil, err
		}
		if ok {
			exists = append(exists, k)
		}
	}
	return exists, nil
}

// NotExistsBlocks returns blocks that don't exist (checks both temp and main)
func (b *flatStoreBatch) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	for _, block := range bs {
		ok, err := b.dsBatch.Has(ctx, flatStoreKey(block.Cid()))
		if err != nil {
			return nil, err
		}
		if !ok {
			notExists = append(notExists, block)
		}
	}
	return notExists, nil
}

// PartitionByExistence partitions cids by existence (checks both temp and main)
func (b *flatStoreBatch) PartitionByExistence(ctx context.Context, ks []cid.Cid) (exist []cid.Cid, notExist []cid.Cid, err error) {
	for _, k := range ks {
		ok, err := b.dsBatch.Has(ctx, flatStoreKey(k))
		if err != nil {
			return nil, nil, err
		}
		if ok {
			exist = append(exist, k)
		} else {
			notExist = append(notExist, k)
		}
	}
	return
}

// Close does nothing for batch (batch has its own lifecycle)
func (b *flatStoreBatch) Close() error {
	return nil
}

func (b *flatStoreBatch) Commit() error {
	err := b.dsBatch.Commit(context.Background())
	if err == nil {
		b.store.sendLocalBytesUsageEvent(context.Background())
	}
	return err
}

func (b *flatStoreBatch) Discard() error {
	// Cast to DiscardableBatch from anyproto fork of flatfs
	if discarder, ok := b.dsBatch.(flatfs.DiscardableBatch); ok {
		return discarder.Discard(context.Background())
	}
	// Fallback: batch is discarded by not committing (garbage collection will clean up)
	return nil
}

type localBytesUsageEventSender struct {
	eventSender event.Sender
	batchPeriod time.Duration

	sync.Mutex
	timer           *time.Timer
	localBytesUsage uint64
}

func newLocalBytesUsageEventSender(eventSender event.Sender, batchPeriod time.Duration, initialLocalBytesUsage uint64) *localBytesUsageEventSender {
	d := &localBytesUsageEventSender{
		eventSender: eventSender,

		batchPeriod:     batchPeriod,
		localBytesUsage: initialLocalBytesUsage,
	}
	return d
}

func (d *localBytesUsageEventSender) sendLocalBytesUsageEvent(localBytesUsage uint64) {
	d.Lock()
	defer d.Unlock()
	d.localBytesUsage = localBytesUsage

	if d.timer == nil {
		d.timer = time.AfterFunc(d.batchPeriod, func() {
			d.Lock()
			defer d.Unlock()
			d.send(d.localBytesUsage)
			d.timer = nil
		})
	}
}

func (d *localBytesUsageEventSender) send(usage uint64) {
	d.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfFileLocalUsage{
		FileLocalUsage: &pb.EventFileLocalUsage{
			LocalBytesUsage: usage,
		},
	}))
}

type LocalStoreGarbageCollector interface {
	MarkAsUsing(cids []cid.Cid)
	CollectGarbage(ctx context.Context) error
}

type flatStoreGarbageCollector struct {
	flatStore *flatStore

	using map[string]struct{}
}

func (c *flatStoreGarbageCollector) MarkAsUsing(cids []cid.Cid) {
	for _, cid := range cids {
		key := flatStoreKey(cid)
		c.using[key.String()] = struct{}{}
	}
}

func (c *flatStoreGarbageCollector) CollectGarbage(ctx context.Context) error {
	results, err := c.flatStore.ds.Query(ctx, query.Query{
		KeysOnly: true,
	})
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	for res := range results.Next() {
		if _, ok := c.using[res.Key]; !ok {
			err = c.flatStore.ds.Delete(ctx, datastore.NewKey(res.Key))
			if err != nil {
				return fmt.Errorf("delete: %w", err)
			}
		}
	}

	c.flatStore.sendLocalBytesUsageEvent(ctx)
	results.Close()
	return nil
}

func newFlatStoreGarbageCollector(flatStore *flatStore) LocalStoreGarbageCollector {
	return &flatStoreGarbageCollector{
		flatStore: flatStore,
		using:     map[string]struct{}{},
	}
}
