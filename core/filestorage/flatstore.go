package filestorage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	flatfs "github.com/ipfs/go-ds-flatfs"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/pb"
)

type flatStore struct {
	ds                         *flatfs.Datastore
	localBytesUsageEventSender *localBytesUsageEventSender
}

func newFlatStore(path string, sendEvent func(event *pb.Event), sendEventBatchTimeout time.Duration) (*flatStore, error) {
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
		localBytesUsageEventSender: newLocalBytesUsageEventSender(sendEvent, sendEventBatchTimeout, bytesUsage),
	}, nil
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
				log.Error("localStore.GetMany", zap.Error(err))
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
	f.sendLocalBytesUsageEvent(ctx)
	return nil
}

func (f *flatStore) Delete(ctx context.Context, c cid.Cid) error {
	err := f.ds.Delete(ctx, dskey(c))
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
		ok, err := f.ds.Has(ctx, dskey(k))
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

func (f *flatStore) Close() error {
	return f.ds.Close()
}

type localBytesUsageEventSender struct {
	sendEvent   func(event *pb.Event)
	batchPeriod time.Duration

	sync.Mutex
	timer           *time.Timer
	localBytesUsage uint64
}

func newLocalBytesUsageEventSender(sendEvent func(event *pb.Event), batchPeriod time.Duration, initialLocalBytesUsage uint64) *localBytesUsageEventSender {
	d := &localBytesUsageEventSender{
		sendEvent: sendEvent,

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
	d.sendEvent(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfFileLocalUsage{
					FileLocalUsage: &pb.EventFileLocalUsage{
						LocalBytesUsage: usage,
					},
				},
			},
		},
	})
}
