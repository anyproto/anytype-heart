package rpcstore

import (
	"context"
	"storj.io/drpc"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/net/rpc/rpcerr"
	"github.com/cheggaaa/mb/v3"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

func newClient(ctx context.Context, s *service, peerId string, tq *mb.MB[*task]) (*client, error) {
	c := &client{
		peerId:     peerId,
		taskQueue:  tq,
		opLoopDone: make(chan struct{}),
		stat:       newStat(),
		s:          s,
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
	s               *service
	mu              sync.Mutex
}

// opLoop gets tasks from taskQueue
func (c *client) opLoop(ctx context.Context) {
	defer close(c.opLoopDone)
	c.mu.Lock()
	spaceIds := c.spaceIds
	allowWrite := c.allowWrite
	c.mu.Unlock()
	cond := c.taskQueue.NewCond().WithFilter(func(t *task) bool {
		if t.write && !allowWrite {
			return false
		}
		if slices.Index(t.denyPeerIds, c.peerId) != -1 {
			return false
		}
		if len(spaceIds) > 0 && slices.Index(spaceIds, t.spaceId) == -1 {
			return false
		}
		return true
	})
	for {
		t, err := cond.WithPriority(c.stat.Score()).WaitOne(ctx)
		if err != nil {
			return
		}
		t.execWithClient(c)
	}
}

func (c *client) delete(ctx context.Context, spaceID string, fileIds ...string) (err error) {
	p, err := c.s.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		if _, err = fileproto.NewDRPCFileClient(conn).FilesDelete(ctx, &fileproto.FilesDeleteRequest{
			SpaceId: spaceID,
			FileIds: fileIds,
		}); err != nil {
			return rpcerr.Unwrap(err)
		}
		c.stat.UpdateLastUsage()
		return nil
	})
}

func (c *client) put(ctx context.Context, spaceID string, fileID string, cd cid.Cid, data []byte) (err error) {
	p, err := c.s.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	st := time.Now()
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		if _, err = fileproto.NewDRPCFileClient(conn).BlockPush(ctx, &fileproto.BlockPushRequest{
			SpaceId: spaceID,
			FileId:  fileID,
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

// get sends the get request to the stream and adds task to waiting list
func (c *client) get(ctx context.Context, spaceID string, cd cid.Cid) (data []byte, err error) {
	p, err := c.s.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	st := time.Now()
	var resp *fileproto.BlockGetResponse
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err = fileproto.NewDRPCFileClient(conn).BlockGet(ctx, &fileproto.BlockGetRequest{
			SpaceId: spaceID,
			Cid:     cd.Bytes(),
		})
		if err != nil {
			return rpcerr.Unwrap(err)
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
	p, err := c.s.pool.Get(ctx, c.peerId)
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

func (c *client) bind(ctx context.Context, spaceID string, fileID string, cids ...cid.Cid) error {
	p, err := c.s.pool.Get(ctx, c.peerId)
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
			FileId:  fileID,
			Cids:    cidsB,
		})
		return err
	})
}

func (c *client) spaceInfo(ctx context.Context, spaceId string) (info *fileproto.SpaceInfoResponse, err error) {
	p, err := c.s.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		info, err = fileproto.NewDRPCFileClient(conn).SpaceInfo(ctx, &fileproto.SpaceInfoRequest{
			SpaceId: spaceId,
		})
		return err
	})
	return
}

func (c *client) filesInfo(ctx context.Context, spaceId string, fileIds []string) (info []*fileproto.FileInfo, err error) {
	p, err := c.s.pool.Get(ctx, c.peerId)
	if err != nil {
		return
	}
	var resp *fileproto.FilesInfoResponse
	err = p.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err = fileproto.NewDRPCFileClient(conn).FilesInfo(ctx, &fileproto.FilesInfoRequest{
			SpaceId: spaceId,
			FileIds: fileIds,
		})
		return err
	})
	if err != nil {
		return
	}
	return resp.FilesInfo, nil
}

func (c *client) checkConnectivity(ctx context.Context) (err error) {
	p, err := c.s.pool.Get(ctx, c.peerId)
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
	if time.Now().Sub(c.stat.lastUsage) < objectTTL {
		return false, nil
	}
	return true, c.Close()
}

func (c *client) Close() error {
	c.opLoopCtxCancel()
	<-c.opLoopDone
	return nil
}
