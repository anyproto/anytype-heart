package rpcstore

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
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
	BindCids(ctx context.Context, spaceID string, fileId domain.FileId, cids []cid.Cid) (err error)

	AddToFileMany(ctx context.Context, req *fileproto.BlockPushManyRequest) error
	AddToFile(ctx context.Context, spaceId string, fileId domain.FileId, bs []blocks.Block) (err error)
	DeleteFiles(ctx context.Context, spaceId string, fileIds ...domain.FileId) (err error)
	SpaceInfo(ctx context.Context, spaceId string) (info *fileproto.SpaceInfoResponse, err error)
	FilesInfo(ctx context.Context, spaceId string, fileIds ...domain.FileId) ([]*fileproto.FileInfo, error)
	AccountInfo(ctx context.Context) (info *fileproto.AccountInfoResponse, err error)
	IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error
}

type store struct {
	cm *clientManager

	backgroundCtx     context.Context
	backgroundCancel  context.CancelFunc
	trafficStatistics *trafficStatistics
}

func newStore(cm *clientManager, trafficStatistics *trafficStatistics) *store {
	ctx, cancel := context.WithCancel(context.Background())
	return &store{
		cm:                cm,
		backgroundCtx:     ctx,
		backgroundCancel:  cancel,
		trafficStatistics: trafficStatistics,
	}
}

func (s *store) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	var (
		ready = make(chan result, 1)
		data  []byte
	)
	ctx = context.WithValue(ctx, operationNameKey, "get")
	if err = s.cm.ReadOp(ctx, ready, func(c *client) (e error) {
		data, e = c.get(ctx, fileblockstore.CtxGetSpaceId(ctx), k)

		s.trafficStatistics.inbound.Add(int64(len(data)))

		return
	}); err != nil {
		return
	}
	if err := waitResult(s.backgroundCtx, ctx, ready); err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(data, k)
}

func (s *store) IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error {
	_, err := writeOperation(s.backgroundCtx, ctx, s, "iterateFiles", func(c *client) (struct{}, error) {
		return struct{}{}, c.iterateFiles(ctx, iterFunc)
	})
	return err
}

func (s *store) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	var (
		ready  = make(chan result, len(ks))
		dataCh = make(chan blocks.Block, len(ks))
	)
	var newGetFunc = func(k cid.Cid) func(c *client) error {
		return func(c *client) error {
			data, err := c.get(ctx, fileblockstore.CtxGetSpaceId(ctx), k)

			s.trafficStatistics.inbound.Add(int64(len(data)))
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
		if err := s.cm.ReadOp(ctx, ready, newGetFunc(k)); err != nil {
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

func (s *store) AddToFileMany(ctx context.Context, req *fileproto.BlockPushManyRequest) error {
	if len(req.FileBlocks) == 0 {
		return nil
	}

	var (
		ready = make(chan result, 1)
	)
	op := func(c *client) error {
		return c.putMany(ctx, req)
	}
	ctx = context.WithValue(ctx, operationNameKey, "addToFileMany")
	if err := s.cm.WriteOp(ctx, ready, op); err != nil {
		return err
	}
	select {
	case res := <-ready:
		if res.err != nil {
			return res.err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (s *store) AddToFile(ctx context.Context, spaceID string, fileId domain.FileId, bs []blocks.Block) error {
	if len(bs) == 0 {
		return nil
	}

	var (
		ready = make(chan result, len(bs))
	)
	var newPutFunc = func(b blocks.Block) func(c *client) error {
		return func(c *client) error {
			return c.put(ctx, spaceID, fileId, b.Cid(), b.RawData())
		}
	}
	ctx = context.WithValue(ctx, operationNameKey, "addToFile")
	for _, b := range bs {
		if err := s.cm.WriteOp(ctx, ready, newPutFunc(b)); err != nil {
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
	return writeOperation(s.backgroundCtx, ctx, s, "checkAvailability", func(c *client) ([]*fileproto.BlockAvailability, error) {
		return c.checkBlocksAvailability(ctx, spaceID, cids...)
	})
}

func (s *store) BindCids(ctx context.Context, spaceID string, fileId domain.FileId, cids []cid.Cid) error {
	_, err := writeOperation(s.backgroundCtx, ctx, s, "bindCids", func(c *client) (interface{}, error) {
		return nil, c.bind(ctx, spaceID, fileId, cids...)
	})
	return err
}

func (s *store) Delete(ctx context.Context, c cid.Cid) error {
	return ErrUnsupported
}

func (s *store) DeleteFiles(ctx context.Context, spaceId string, fileIds ...domain.FileId) error {
	_, err := writeOperation(s.backgroundCtx, ctx, s, "deleteFiles", func(c *client) (interface{}, error) {
		return nil, c.delete(ctx, spaceId, fileIds...)
	})
	return err
}

func (s *store) AccountInfo(ctx context.Context) (*fileproto.AccountInfoResponse, error) {
	return writeOperation(s.backgroundCtx, ctx, s, "accountInfo", func(c *client) (*fileproto.AccountInfoResponse, error) {
		return c.accountInfo(ctx)
	})
}

func (s *store) SpaceInfo(ctx context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
	return writeOperation(s.backgroundCtx, ctx, s, "spaceInfo", func(c *client) (*fileproto.SpaceInfoResponse, error) {
		return c.spaceInfo(ctx, spaceId)
	})
}

func (s *store) FilesInfo(ctx context.Context, spaceId string, fileIds ...domain.FileId) ([]*fileproto.FileInfo, error) {
	return writeOperation(s.backgroundCtx, ctx, s, "filesInfo", func(c *client) ([]*fileproto.FileInfo, error) {
		return c.filesInfo(ctx, spaceId, fileIds)
	})
}

func (s *store) Close() (err error) {
	if s.backgroundCancel != nil {
		s.backgroundCancel()
	}
	return s.cm.Close()
}

func writeOperation[T any](backgroundCtx context.Context, ctx context.Context, s *store, operationName string, fn func(c *client) (T, error)) (T, error) {
	ready := make(chan result, 1)
	ctx = context.WithValue(ctx, operationNameKey, operationName)
	var res T
	var defaultRes T
	if err := s.cm.WriteOp(ctx, ready, func(c *client) error {
		var opErr error
		res, opErr = fn(c)
		return opErr
	}); err != nil {
		return defaultRes, err
	}
	if err := waitResult(backgroundCtx, ctx, ready); err != nil {
		return defaultRes, err
	}
	return res, nil
}

func waitResult(backgroundCtx context.Context, ctx context.Context, ready chan result) error {
	select {
	case <-backgroundCtx.Done():
		return backgroundCtx.Err()
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ready:
		if res.err != nil {
			return res.err
		}
	}
	return nil
}
