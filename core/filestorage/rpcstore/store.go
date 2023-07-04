package rpcstore

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

var closedBlockChan chan blocks.Block

var ErrUnsupported = errors.New("unsupported operation")

func init() {
	closedBlockChan = make(chan blocks.Block)
	close(closedBlockChan)
}

type RpcStore interface {
	fileblockstore.BlockStore

	CheckAvailability(ctx context.Context, spaceID string, cids []cid.Cid) (checkResult []*fileproto.BlockAvailability, err error)
	BindCids(ctx context.Context, spaceID string, fileID string, cids []cid.Cid) (err error)

	AddToFile(ctx context.Context, spaceId string, fileId string, bs []blocks.Block) (err error)
	DeleteFiles(ctx context.Context, spaceId string, fileIds ...string) (err error)
	SpaceInfo(ctx context.Context, spaceId string) (info *fileproto.SpaceInfoResponse, err error)
	FilesInfo(ctx context.Context, spaceId string, fileIds ...string) ([]*fileproto.FileInfo, error)
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
	ctx = context.WithValue(ctx, operationNameKey, "get")
	if err = s.cm.ReadOp(ctx, ready, func(c *client) (e error) {
		data, e = c.get(ctx, fileblockstore.CtxGetSpaceId(ctx), k)
		return
	}, k); err != nil {
		return
	}
	if err := waitResult(ctx, ready); err != nil {
		return nil, err
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
			data, err := c.get(ctx, fileblockstore.CtxGetSpaceId(ctx), k)
			if err != nil {
				return err
			}
			b, _ := blocks.NewBlockWithCid(data, k)
			dataCh <- b
			return nil
		}
	}
	ctx = context.WithValue(ctx, operationNameKey, "getMany")
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
	return ErrUnsupported
}

func (s *store) AddToFile(ctx context.Context, spaceID string, fileID string, bs []blocks.Block) error {
	if len(bs) == 0 {
		return nil
	}

	var (
		ready = make(chan result, len(bs))
	)
	var newPutFunc = func(b blocks.Block) func(c *client) error {
		return func(c *client) error {
			return c.put(ctx, spaceID, fileID, b.Cid(), b.RawData())
		}
	}
	ctx = context.WithValue(ctx, operationNameKey, "addToFile")
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

func (s *store) CheckAvailability(ctx context.Context, spaceID string, cids []cid.Cid) ([]*fileproto.BlockAvailability, error) {
	return writeOperation(ctx, s, "checkAvailability", func(c *client) ([]*fileproto.BlockAvailability, error) {
		return c.checkBlocksAvailability(ctx, spaceID, cids...)
	})
}

func (s *store) BindCids(ctx context.Context, spaceID string, fileID string, cids []cid.Cid) error {
	_, err := writeOperation(ctx, s, "bindCids", func(c *client) (interface{}, error) {
		return nil, c.bind(ctx, spaceID, fileID, cids...)
	})
	return err
}

func (s *store) Delete(ctx context.Context, c cid.Cid) error {
	return ErrUnsupported
}

func (s *store) DeleteFiles(ctx context.Context, spaceId string, fileIds ...string) error {
	_, err := writeOperation(ctx, s, "deleteFiles", func(c *client) (interface{}, error) {
		return nil, c.delete(ctx, spaceId, fileIds...)
	})
	return err
}

func (s *store) SpaceInfo(ctx context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
	return writeOperation(ctx, s, "spaceInfo", func(c *client) (*fileproto.SpaceInfoResponse, error) {
		return c.spaceInfo(ctx, spaceId)
	})
}

func (s *store) FilesInfo(ctx context.Context, spaceId string, fileIds ...string) ([]*fileproto.FileInfo, error) {
	return writeOperation(ctx, s, "filesInfo", func(c *client) ([]*fileproto.FileInfo, error) {
		return c.filesInfo(ctx, spaceId, fileIds)
	})
}

func (s *store) Close() (err error) {
	return s.cm.Close()
}

func writeOperation[T any](ctx context.Context, s *store, operationName string, fn func(c *client) (T, error)) (T, error) {
	ready := make(chan result, 1)
	ctx = context.WithValue(ctx, operationNameKey, operationName)
	var res T
	if err := s.cm.WriteOp(ctx, ready, func(c *client) error {
		var opErr error
		res, opErr = fn(c)
		return opErr
	}, cid.Cid{}); err != nil {
		return res, err
	}
	if err := waitResult(ctx, ready); err != nil {
		return res, err
	}
	return res, nil
}

func waitResult(ctx context.Context, ready chan result) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ready:
		if res.err != nil {
			return res.err
		}
	}
	return nil
}
