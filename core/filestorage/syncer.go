package filestorage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anytypeio/any-sync/commonfile/fileblockstore"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"
)

const syncerOpBatch = 10

type syncer struct {
	ps   *proxyStore
	done chan struct{}
}

func (s *syncer) run(ctx context.Context) {
	defer close(s.done)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		case <-s.ps.index.HasWorkCh():
		}
		for s.sync(ctx) > 0 {
		}
	}
}

func (s *syncer) sync(ctx context.Context) (doneCount int32) {
	cids, err := s.ps.index.List(syncerOpBatch)
	if err != nil {
		log.Error("index list error", zap.Error(err))
		return
	}
	defer cids.Release()
	l := cids.Len()
	total, _ := s.ps.index.Len()
	log.Debug("remote file sync, got tasks to sync", zap.Int("count", l), zap.Int("inQueue", total))
	if l == 0 {
		return
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var wg sync.WaitGroup
	var doneAtomic atomic.Int32
	for _, sOps := range cids.SpaceOps {
		if len(sOps.Load) > 0 {
			wg.Add(1)
			go func(opt SpaceCidOps) {
				defer wg.Done()
				doneAtomic.Add(s.load(ctx, opt))
			}(sOps)
		}
		if len(sOps.Delete) > 0 {
			wg.Add(1)
			go func(opt SpaceCidOps) {
				defer wg.Done()
				doneAtomic.Add(s.delete(ctx, opt))
			}(sOps)
		}
		if len(sOps.Add) > 0 {
			wg.Add(1)
			go func(opt SpaceCidOps) {
				defer wg.Done()
				doneAtomic.Add(s.add(ctx, opt))
			}(sOps)
		}
	}
	wg.Wait()
	return doneAtomic.Load()
}

func (s *syncer) load(ctx context.Context, spaceOps SpaceCidOps) (doneCount int32) {
	ctx = fileblockstore.CtxWithSpaceId(ctx, spaceOps.SpaceId)
	res := s.ps.origin.GetMany(ctx, spaceOps.Load)
	doneCids := NewCids()
	defer doneCids.Release()
	for b := range res {
		if err := s.ps.cache.Add(ctx, []blocks.Block{b}); err != nil {
			log.Error("syncer: can't add to local store", zap.Error(err))
			continue
		}
		doneCids.Add(spaceOps.SpaceId, OpLoad, b.Cid())
	}
	if err := s.ps.index.Done(doneCids); err != nil {
		log.Error("syncer: index.Done error", zap.Error(err))
		return
	}
	doneCount = int32(doneCids.Len())
	log.Info("successfully loaded cids", zap.Int32("count", doneCount))
	return
}

func (s *syncer) add(ctx context.Context, spaceOps SpaceCidOps) (doneCount int32) {
	doneCids := NewCids()
	defer doneCids.Release()
	res := s.ps.cache.GetMany(ctx, spaceOps.Add)
	var bs []blocks.Block
	for b := range res {
		bs = append(bs, b)
	}
	ctx = fileblockstore.CtxWithSpaceId(ctx, spaceOps.SpaceId)

	successCidsCh := s.ps.origin.AddAsync(ctx, bs)
	for doneCid := range successCidsCh {
		doneCids.Add(spaceOps.SpaceId, OpAdd, doneCid)
	}

	doneCount = int32(doneCids.Len())
	if doneCount == 0 {
		return
	}

	if err := s.ps.index.Done(doneCids); err != nil {
		log.Error("syncer: index.Done error", zap.Error(err))
		return
	}
	log.Info("successfully added cids", zap.Int32("count", doneCount))
	return
}

func (s *syncer) delete(ctx context.Context, spaceOps SpaceCidOps) (doneCount int32) {
	doneCids := NewCids()
	defer doneCids.Release()
	ctx = fileblockstore.CtxWithSpaceId(ctx, spaceOps.SpaceId)
	cids := make([]cid.Cid, len(spaceOps.Delete))
	for i := range spaceOps.Delete {
		cids[i] = spaceOps.Delete[i]
		doneCids.Add(spaceOps.SpaceId, OpDelete, spaceOps.Delete[i])
	}
	log.Debug(fmt.Sprintf("cids: %v", cids))
	if err := s.ps.origin.DeleteMany(ctx, cids...); err != nil {
		log.Debug("syncer: can't remove from remote", zap.Error(err))
		return 0
	}
	if err := s.ps.index.Done(doneCids); err != nil {
		log.Error("syncer: index.Done error", zap.Error(err))
	}
	doneCount = int32(doneCids.Len())
	log.Info("successfully removed cids", zap.Int32("count", doneCount))
	return
}
