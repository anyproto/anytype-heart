package rpcstore

import (
	"context"
	"github.com/anytypeio/any-sync/commonfile/fileblockstore"
	"github.com/anytypeio/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"sync"
)

var closedBlockChan chan blocks.Block

func init() {
	closedBlockChan = make(chan blocks.Block)
	close(closedBlockChan)
}

type RpcStore interface {
	fileblockstore.BlockStore
	AddAsync(ctx context.Context, bs []blocks.Block) (successCh chan cid.Cid)
	DeleteMany(ctx context.Context, cids ...cid.Cid) (err error)
}

type store struct {
	s  *service
	cm *clientManager
	mu sync.RWMutex
}

func (s *store) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	var (
		ready = make(chan result, 1)
		data  []byte
	)
	if err = s.cm.ReadOp(ctx, ready, func(c *client) (e error) {
		data, e = c.get(ctx, k)
		return
	}, k); err != nil {
		return
	}
	select {
	case res := <-ready:
		if res.err != nil {
			return nil, res.err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return blocks.NewBlockWithCid(data, k)
}

func (s *store) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	var (
		ready  = make(chan result, len(ks))
		dataCh = make(chan blocks.Block, len(ks))
	)
	var newGetFunc = func(k cid.Cid) func(c *client) error {
		return func(c *client) error {
			data, err := c.get(ctx, k)
			if err != nil {
				return err
			}
			b, _ := blocks.NewBlockWithCid(data, k)
			dataCh <- b
			return nil
		}
	}
	for _, k := range ks {
		if err := s.cm.ReadOp(ctx, ready, newGetFunc(k), k); err != nil {
			log.Error("getMany: can't add tasks", zap.Error(err))
			return closedBlockChan
		}
	}
	var resultCh = make(chan blocks.Block)
	go func() {
		defer close(resultCh)
		for i := 0; i < len(ks); i++ {
			// wait ready signal
			select {
			case <-ctx.Done():
				return
			case res := <-ready:
				if res.err != nil {
					log.Info("get many got task error", zap.Error(res.err))
					continue
				}
			}
			// wait block
			var b blocks.Block
			select {
			case <-ctx.Done():
				return
			case b = <-dataCh:
			}
			// send block
			select {
			case <-ctx.Done():
				return
			case resultCh <- b:
			}
		}
	}()
	return resultCh
}

func (s *store) Add(ctx context.Context, bs []blocks.Block) error {
	var (
		ready = make(chan result, len(bs))
	)
	var newPutFunc = func(b blocks.Block) func(c *client) error {
		return func(c *client) error {
			return c.put(ctx, b.Cid(), b.RawData())
		}
	}
	for _, b := range bs {
		if err := s.cm.WriteOp(ctx, ready, newPutFunc(b), b.Cid()); err != nil {
			return err
		}
	}
	var errs []error
	for i := 0; i < len(bs); i++ {
		select {
		case res := <-ready:
			if res.err != nil {
				errs = append(errs, res.err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if len(errs) > 0 {
		return multierr.Combine(errs...)
	}
	return nil
}

func (s *store) AddAsync(ctx context.Context, bs []blocks.Block) (successCh chan cid.Cid) {
	successCh = make(chan cid.Cid, len(bs))
	go func() {
		defer close(successCh)

		var (
			cids        = make([]cid.Cid, 0, len(bs))
			ready       = make(chan result, 1)
			checkResult []*fileproto.BlockAvailability
		)

		for _, b := range bs {
			cids = append(cids, b.Cid())
		}
		// check blocks availability
		if err := s.cm.WriteOp(ctx, ready, func(c *client) (err error) {
			checkResult, err = c.checkBlocksAvailability(ctx, cids...)
			return err
		}, cid.Cid{}); err != nil {
			log.Info("addAsync add check op error", zap.Error(err))
			return
		}
		// wait availability result
		select {
		case <-ctx.Done():
			return
		case <-ready:
		}
		// exclude existing ids
		var excludeCids []cid.Cid
		for _, check := range checkResult {
			if check.Status == fileproto.AvailabilityStatus_Exists || check.Status == fileproto.AvailabilityStatus_ExistsInSpace {
				// TODO: make bound for the not in space ids
				if c, e := cid.Cast(check.Cid); e == nil {
					excludeCids = append(excludeCids, c)
					successCh <- c
				}
			}
		}

		if len(excludeCids) > 0 {
			fileteredBs := bs[:0]
			for _, b := range bs {
				if !slices.Contains(excludeCids, b.Cid()) {
					fileteredBs = append(fileteredBs, b)
				}
			}
			bs = fileteredBs
		}

		if len(bs) == 0 {
			return
		}

		// put non-existent blocks
		ready = make(chan result, len(bs))
		var newPutFunc = func(b blocks.Block) func(c *client) error {
			return func(c *client) error {
				return c.put(ctx, b.Cid(), b.RawData())
			}
		}
		for _, b := range bs {
			if err := s.cm.WriteOp(ctx, ready, newPutFunc(b), b.Cid()); err != nil {
				log.Info("addAsync add op error", zap.Error(err))
				return
			}
		}
		for i := 0; i < len(bs); i++ {
			select {
			case res := <-ready:
				if res.err == nil {
					successCh <- res.cid
				} else {
					log.Info("addAsync: task error", zap.Error(res.err))
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return
}

func (s *store) Delete(ctx context.Context, c cid.Cid) error {
	return s.DeleteMany(ctx, c)
}

func (s *store) DeleteMany(ctx context.Context, cids ...cid.Cid) error {
	var ready = make(chan result, 1)
	if err := s.cm.WriteOp(ctx, ready, func(c *client) error {
		return c.delete(ctx, cids...)
	}, cid.Cid{}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ready:
		return res.err
	}
}

func (s *store) Close() (err error) {
	return s.cm.Close()
}
