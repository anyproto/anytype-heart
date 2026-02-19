package rpcstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/rpcerr"
	"github.com/cheggaaa/mb/v3"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/domain"
)

type ctxKey string

const CtxWaitAvailable = ctxKey("waitAvailable")

func ContextWithWaitAvailable(ctx context.Context) context.Context {
	return context.WithValue(ctx, CtxWaitAvailable, true)
}

func IsWaitWhenAvailable(ctx context.Context) bool {
	wait, ok := ctx.Value(CtxWaitAvailable).(bool)
	return ok && wait
}

func newClient(ctx context.Context, pool pool.Pool, peerId string, tq *mb.MB[*task]) (*client, error) {
	c := &client{
		peerId:     peerId,
		taskQueue:  tq,
		opLoopDone: make(chan struct{}),
		stat:       newStat(),
		pool:       pool,
	}
	if err := c.checkConnectivity(ctx); err != nil {
		return nil, err
	}
	log.Debug("starting client for peer", zap.String("peer", peerId), zap.Strings("spaces", c.spaceIds))
	var runCtx context.Context
	runCtx, c.opLoopCtxCancel = context.WithCancel(context.Background())
	go c.opLoop(runCtx)
	return c, nil
}

// client gets and executes tasks from taskQueue
// it has an internal queue for a waiting CIDs
type client struct {
	peerId          string
	spaceIds        []string
	allowWrite      bool
	taskQueue       *mb.MB[*task]
	opLoopDone      chan struct{}
	opLoopCtxCancel context.CancelFunc
	stat            *stat
	pool            pool.Pool
	mu              sync.Mutex
}

func (c *client) checkSpaceFilter(t *task) bool {
	// Only if we have a filter for spaceId
	if len(c.spaceIds) > 0 && !slices.Contains(c.spaceIds, t.spaceId) {
		return false
	}
	return true
}

// opLoop gets tasks from taskQueue
func (c *client) opLoop(ctx context.Context) {
	defer close(c.opLoopDone)
	c.mu.Lock()
	allowWrite := c.allowWrite
	c.mu.Unlock()

	writeCond := c.taskQueue.NewCond().WithFilter(func(t *task) bool {
		if !t.write {
			return false
		}
		if !allowWrite {
			return false
		}
		if slices.Index(t.denyPeerIds, c.peerId) != -1 {
			return false
		}
		return c.checkSpaceFilter(t)
	})

	readCond := c.taskQueue.NewCond().WithFilter(func(t *task) bool {
		if t.write {
			return false
		}
		if slices.Index(t.denyPeerIds, c.peerId) != -1 {
			return false
		}
		return c.checkSpaceFilter(t)
	})

	go func() {
		c.runWorkers(ctx, maxSubConnections, readCond)
	}()
	c.runWorkers(ctx, maxSubConnections, writeCond)
}

func (c *client) runWorkers(ctx context.Context, count int, waitCond mb.WaitCond[*task]) {
	connections := make(chan struct{}, count)
	for {
		t, err := waitCond.WithPriority(c.stat.Score()).WaitOne(ctx)
		if err != nil {
			return
		}

		if count == 1 {
			t.execWithClient(c)
		} else {
			connections <- struct{}{}
			go func() {
				t.execWithClient(c)
				<-connections
			}()
		}
	}
}

func (c *client) delete(ctx context.Context, spaceID string, fileIds ...domain.FileId) (err error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		rawFileIds := make([]string, 0, len(fileIds))
		for _, id := range fileIds {
			rawFileIds = append(rawFileIds, id.String())
		}
		if _, err = fileproto.NewDRPCFileClient(conn).FilesDelete(ctx, &fileproto.FilesDeleteRequest{
			SpaceId: spaceID,
			FileIds: rawFileIds,
		}); err != nil {
			return rpcerr.Unwrap(err)
		}
		c.stat.UpdateLastUsage()
		return nil
	})
}

func (c *client) iterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return err
	}
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		cl := fileproto.NewDRPCFileClient(conn)

		resp, err := cl.AccountInfo(ctx, &fileproto.AccountInfoRequest{})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		for _, space := range resp.Spaces {
			err := iterateSpaceFiles(ctx, cl, space.SpaceId, iterFunc)
			if err != nil {
				return fmt.Errorf("iterate space files: %w", err)
			}
		}
		return nil
	})
}

func iterateSpaceFiles(ctx context.Context, client fileproto.DRPCFileClient, spaceId string, iterFunc func(fileId domain.FullFileId)) error {
	filesStream, err := client.FilesGet(ctx, &fileproto.FilesGetRequest{SpaceId: spaceId})
	if err != nil {
		return rpcerr.Unwrap(err)
	}
	defer filesStream.Close()
	for {
		resp, err := filesStream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		iterFunc(domain.FullFileId{
			SpaceId: spaceId,
			FileId:  domain.FileId(resp.FileId),
		})
	}
}

func (c *client) put(ctx context.Context, spaceID string, fileId domain.FileId, cd cid.Cid, data []byte) (err error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	st := time.Now()
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		if _, err = fileproto.NewDRPCFileClient(conn).BlockPush(ctx, &fileproto.BlockPushRequest{
			SpaceId: spaceID,
			FileId:  fileId.String(),
			Cid:     cd.Bytes(),
			Data:    data,
		}); err != nil {
			return rpcerr.Unwrap(err)
		}

		log.Debug("put cid", zap.String("cid", cd.String()))
		c.stat.Add(st, len(data))
		return nil
	})
}

func (c *client) putMany(ctx context.Context, req *fileproto.BlockPushManyRequest) (err error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	st := time.Now()
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		if _, err = fileproto.NewDRPCFileClient(conn).BlockPushMany(ctx, req); err != nil {
			return rpcerr.Unwrap(err)
		}

		var totalDataSize int
		for _, fb := range req.FileBlocks {
			for _, b := range fb.Blocks {
				totalDataSize += len(b.Data)
			}
		}

		c.stat.Add(st, totalDataSize)
		return nil
	})
}

// get sends the get request to the stream and adds task to waiting list
func (c *client) get(ctx context.Context, spaceID string, cd cid.Cid) (data []byte, err error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	st := time.Now()
	var resp *fileproto.BlockGetResponse
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err = fileproto.NewDRPCFileClient(conn).BlockGet(ctx, &fileproto.BlockGetRequest{
			SpaceId: spaceID,
			Cid:     cd.Bytes(),
			Wait:    IsWaitWhenAvailable(ctx),
		})
		if err != nil {
			err = rpcerr.Unwrap(err)
			if errors.Is(err, fileprotoerr.ErrCIDNotFound) {
				return format.ErrNotFound{Cid: cd}
			}
			return err
		}
		log.Debug("get cid", zap.String("cid", cd.String()))
		c.stat.Add(st, len(resp.Data))
		return nil
	})
	if err != nil {
		return
	}
	return resp.Data, nil
}

func (c *client) checkBlocksAvailability(ctx context.Context, spaceID string, cids ...cid.Cid) ([]*fileproto.BlockAvailability, error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return nil, err
	}
	var cidsB = make([][]byte, len(cids))
	for i, c := range cids {
		cidsB[i] = c.Bytes()
	}
	var resp *fileproto.BlocksCheckResponse
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err = fileproto.NewDRPCFileClient(conn).BlocksCheck(ctx, &fileproto.BlocksCheckRequest{
			SpaceId: spaceID,
			Cids:    cidsB,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return resp.BlocksAvailability, nil
}

func (c *client) bind(ctx context.Context, spaceID string, fileId domain.FileId, cids ...cid.Cid) error {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return err
	}
	var cidsB = make([][]byte, len(cids))
	for i, c := range cids {
		cidsB[i] = c.Bytes()
	}
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		_, err = fileproto.NewDRPCFileClient(conn).BlocksBind(ctx, &fileproto.BlocksBindRequest{
			SpaceId: spaceID,
			FileId:  fileId.String(),
			Cids:    cidsB,
		})
		return err
	})
}

func (c *client) accountInfo(ctx context.Context) (info *fileproto.AccountInfoResponse, err error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		info, err = fileproto.NewDRPCFileClient(conn).AccountInfo(ctx, &fileproto.AccountInfoRequest{})
		return err
	})
	return
}

func (c *client) spaceInfo(ctx context.Context, spaceId string) (info *fileproto.SpaceInfoResponse, err error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		info, err = fileproto.NewDRPCFileClient(conn).SpaceInfo(ctx, &fileproto.SpaceInfoRequest{
			SpaceId: spaceId,
		})
		return rpcerr.Unwrap(err)
	})
	return
}

func (c *client) filesInfo(ctx context.Context, spaceId string, fileIds []domain.FileId) (info []*fileproto.FileInfo, err error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	var resp *fileproto.FilesInfoResponse
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		rawFileIds := make([]string, 0, len(fileIds))
		for _, id := range fileIds {
			rawFileIds = append(rawFileIds, id.String())
		}
		resp, err = fileproto.NewDRPCFileClient(conn).FilesInfo(ctx, &fileproto.FilesInfoRequest{
			SpaceId: spaceId,
			FileIds: rawFileIds,
		})
		return err
	})
	if err != nil {
		return
	}
	return resp.FilesInfo, nil
}

func (c *client) checkConnectivity(ctx context.Context) (err error) {
	p, err := c.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	var resp *fileproto.CheckResponse
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err = fileproto.NewDRPCFileClient(conn).Check(ctx, &fileproto.CheckRequest{})
		return err
	})
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spaceIds = resp.SpaceIds
	c.allowWrite = resp.AllowWrite
	return
}

func (c *client) TryClose(objectTTL time.Duration) (bool, error) {
	c.mu.Lock()
	lifeTime := time.Now().Sub(c.stat.lastUsage)
	c.mu.Unlock()

	if lifeTime < objectTTL {
		return false, nil
	}
	return true, c.Close()
}

func (c *client) Close() error {
	c.opLoopCtxCancel()
	<-c.opLoopDone
	return nil
}
