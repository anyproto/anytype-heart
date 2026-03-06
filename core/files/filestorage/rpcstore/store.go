package rpcstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/rpcerr"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

var closedBlockChan chan blocks.Block

var (
	ErrUnsupported                 = errors.New("unsupported operation")
	ErrNoConnectionToAnyFileClient = errors.New("no connection to any file client")
)

func init() {
	closedBlockChan = make(chan blocks.Block)
	close(closedBlockChan)
}

type ctxKey string

const CtxWaitAvailable = ctxKey("waitAvailable")

func ContextWithWaitAvailable(ctx context.Context) context.Context {
	return context.WithValue(ctx, CtxWaitAvailable, true)
}

func IsWaitWhenAvailable(ctx context.Context) bool {
	wait, ok := ctx.Value(CtxWaitAvailable).(bool)
	return ok && wait
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

const (
	getManyWorkers   = 4
	localPeerTimeout = time.Second
	localPeerBanTTL  = 5 * time.Minute
)

type store struct {
	pool      pool.Pool
	peerStore peerstore.PeerStore

	mu           sync.Mutex
	reservedPeer peer.Peer
	reservedConn drpc.Conn

	bannedMu       sync.Mutex
	bannedLocalMap map[string]time.Time
}

func newStore(pool pool.Pool, peerStore peerstore.PeerStore) *store {
	return &store{
		pool:           pool,
		peerStore:      peerStore,
		bannedLocalMap: make(map[string]time.Time),
	}
}

func (s *store) banLocalPeer(peerId string) {
	s.bannedMu.Lock()
	defer s.bannedMu.Unlock()
	s.bannedLocalMap[peerId] = time.Now().Add(localPeerBanTTL)
}

func (s *store) filterBannedPeers(peerIds []string) []string {
	s.bannedMu.Lock()
	defer s.bannedMu.Unlock()
	now := time.Now()
	result := make([]string, 0, len(peerIds))
	for _, id := range peerIds {
		if banUntil, ok := s.bannedLocalMap[id]; ok {
			if now.Before(banUntil) {
				continue
			}
			delete(s.bannedLocalMap, id)
		}
		result = append(result, id)
	}
	return result
}

// getNodePeer returns a peer from ResponsibleFilePeers via pool.GetOneOf.
func (s *store) getNodePeer(ctx context.Context) (peer.Peer, error) {
	peerIds := s.peerStore.ResponsibleFilePeers()
	if len(peerIds) == 0 {
		return nil, ErrNoConnectionToAnyFileClient
	}
	return s.pool.GetOneOf(ctx, peerIds)
}

// doNodeDrpc borrows a sub-connection from a node peer, executes the RPC, and returns it.
// Used for big-payload operations (block data transfers).
func (s *store) doNodeDrpc(ctx context.Context, do func(cl fileproto.DRPCFileClient) error) error {
	p, err := s.getNodePeer(ctx)
	if err != nil {
		return fmt.Errorf("get node peer: %w", err)
	}
	return p.DoDrpc(ctx, func(conn drpc.Conn) error {
		return do(fileproto.NewDRPCFileClient(conn))
	})
}

// doNodeReserved uses a lazily-acquired reserved sub-connection for small-payload operations.
// A mutex serializes access since DRPC connections support only one concurrent stream.
func (s *store) doNodeReserved(ctx context.Context, do func(cl fileproto.DRPCFileClient) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureReservedConn(ctx); err != nil {
		return err
	}
	err := do(fileproto.NewDRPCFileClient(s.reservedConn))
	if err != nil {
		// Connection may be broken; reset for reconnect on next call
		s.resetReservedConnLocked()
		return err
	}
	return nil
}

func (s *store) ensureReservedConn(ctx context.Context) error {
	if s.reservedConn != nil {
		select {
		case <-s.reservedConn.Closed():
			s.reservedConn = nil
			s.reservedPeer = nil
		default:
			return nil
		}
	}
	p, err := s.getNodePeer(ctx)
	if err != nil {
		return fmt.Errorf("get node peer for reserved conn: %w", err)
	}
	conn, err := p.AcquireDrpcConn(ctx)
	if err != nil {
		return fmt.Errorf("acquire reserved conn: %w", err)
	}
	s.reservedPeer = p
	s.reservedConn = conn
	return nil
}

func (s *store) resetReservedConnLocked() {
	s.reservedConn = nil
	s.reservedPeer = nil
}

// Get retrieves a block, trying local peers first then falling back to the node peer.
func (s *store) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	spaceId := fileblockstore.CtxGetSpaceId(ctx)
	data, err := s.getFromLocalPeers(ctx, spaceId, k)
	if err != nil {
		data, err = s.getFromNodePeer(ctx, spaceId, k)
	}
	if err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(data, k)
}

func (s *store) getFromLocalPeers(ctx context.Context, spaceId string, k cid.Cid) ([]byte, error) {
	localPeerIds := s.filterBannedPeers(s.peerStore.AllLocalPeers())
	if len(localPeerIds) == 0 {
		return nil, fmt.Errorf("no local peers available")
	}
	localCtx, cancel := context.WithTimeout(ctx, localPeerTimeout)
	defer cancel()
	p, err := s.pool.GetOneOf(localCtx, localPeerIds)
	if err != nil {
		return nil, fmt.Errorf("get local peer: %w", err)
	}
	data, err := s.getBlock(localCtx, p, spaceId, k, false)
	if err != nil {
		if errors.Is(err, net.ErrUnableToConnect) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			s.banLocalPeer(p.Id())
		}
		return nil, err
	}
	return data, nil
}

func (s *store) getFromNodePeer(ctx context.Context, spaceId string, k cid.Cid) ([]byte, error) {
	p, err := s.getNodePeer(ctx)
	if err != nil {
		return nil, fmt.Errorf("get node peer: %w", err)
	}
	return s.getBlock(ctx, p, spaceId, k, IsWaitWhenAvailable(ctx))
}

func (s *store) getBlock(ctx context.Context, p peer.Peer, spaceId string, k cid.Cid, wait bool) ([]byte, error) {
	var data []byte
	err := p.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err := fileproto.NewDRPCFileClient(conn).BlockGet(ctx, &fileproto.BlockGetRequest{
			SpaceId: spaceId,
			Cid:     k.Bytes(),
			Wait:    wait,
		})
		if err != nil {
			err = rpcerr.Unwrap(err)
			if errors.Is(err, fileprotoerr.ErrCIDNotFound) {
				return format.ErrNotFound{Cid: k}
			}
			return err
		}
		data = resp.Data
		return nil
	})
	return data, err
}

// GetMany retrieves multiple blocks concurrently with a bounded worker pool,
// trying local peers first then falling back to node.
func (s *store) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	resultCh := make(chan blocks.Block, len(ks))
	go func() {
		defer close(resultCh)
		spaceId := fileblockstore.CtxGetSpaceId(ctx)
		var wg sync.WaitGroup
		sem := make(chan struct{}, getManyWorkers)
		for _, k := range ks {
			wg.Add(1)
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Done()
				return
			}
			go func(k cid.Cid) {
				defer func() {
					<-sem
					wg.Done()
				}()
				data, err := s.getFromLocalPeers(ctx, spaceId, k)
				if err != nil {
					data, err = s.getFromNodePeer(ctx, spaceId, k)
				}
				if err != nil {
					log.Info("get many: block error", zap.String("cid", k.String()), zap.Error(err))
					return
				}
				b, err := blocks.NewBlockWithCid(data, k)
				if err != nil {
					log.Info("get many: new block error", zap.String("cid", k.String()), zap.Error(err))
					return
				}
				select {
				case <-ctx.Done():
				case resultCh <- b:
				}
			}(k)
		}
		wg.Wait()
	}()
	return resultCh
}

func (s *store) Add(ctx context.Context, bs []blocks.Block) error {
	return ErrUnsupported
}

func (s *store) Delete(ctx context.Context, c cid.Cid) error {
	return ErrUnsupported
}

// AddToFile pushes blocks to the file node one at a time.
func (s *store) AddToFile(ctx context.Context, spaceId string, fileId domain.FileId, bs []blocks.Block) error {
	if len(bs) == 0 {
		return nil
	}
	for _, b := range bs {
		err := s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
			_, err := cl.BlockPush(ctx, &fileproto.BlockPushRequest{
				SpaceId: spaceId,
				FileId:  fileId.String(),
				Cid:     b.Cid().Bytes(),
				Data:    b.RawData(),
			})
			if err != nil {
				return rpcerr.Unwrap(err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// AddToFileMany pushes multiple file blocks in a single RPC call.
func (s *store) AddToFileMany(ctx context.Context, req *fileproto.BlockPushManyRequest) error {
	if len(req.FileBlocks) == 0 {
		return nil
	}
	return s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
		_, err := cl.BlockPushMany(ctx, req)
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		return nil
	})
}

// CheckAvailability checks which CIDs exist on the file node.
func (s *store) CheckAvailability(ctx context.Context, spaceID string, cids []cid.Cid) ([]*fileproto.BlockAvailability, error) {
	cidsB := make([][]byte, len(cids))
	for i, c := range cids {
		cidsB[i] = c.Bytes()
	}
	var result []*fileproto.BlockAvailability
	err := s.doNodeReserved(ctx, func(cl fileproto.DRPCFileClient) error {
		resp, err := cl.BlocksCheck(ctx, &fileproto.BlocksCheckRequest{
			SpaceId: spaceID,
			Cids:    cidsB,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		result = resp.BlocksAvailability
		return nil
	})
	return result, err
}

// BindCids binds CIDs to a file on the file node.
func (s *store) BindCids(ctx context.Context, spaceID string, fileId domain.FileId, cids []cid.Cid) error {
	cidsB := make([][]byte, len(cids))
	for i, c := range cids {
		cidsB[i] = c.Bytes()
	}
	return s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
		_, err := cl.BlocksBind(ctx, &fileproto.BlocksBindRequest{
			SpaceId: spaceID,
			FileId:  fileId.String(),
			Cids:    cidsB,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		return nil
	})
}

// DeleteFiles deletes files from the file node.
func (s *store) DeleteFiles(ctx context.Context, spaceId string, fileIds ...domain.FileId) error {
	rawFileIds := make([]string, 0, len(fileIds))
	for _, id := range fileIds {
		rawFileIds = append(rawFileIds, id.String())
	}
	return s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
		_, err := cl.FilesDelete(ctx, &fileproto.FilesDeleteRequest{
			SpaceId: spaceId,
			FileIds: rawFileIds,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		return nil
	})
}

// AccountInfo retrieves account-level file storage info.
func (s *store) AccountInfo(ctx context.Context) (*fileproto.AccountInfoResponse, error) {
	var result *fileproto.AccountInfoResponse
	err := s.doNodeReserved(ctx, func(cl fileproto.DRPCFileClient) error {
		resp, err := cl.AccountInfo(ctx, &fileproto.AccountInfoRequest{})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		result = resp
		return nil
	})
	return result, err
}

// SpaceInfo retrieves space-level file storage info.
func (s *store) SpaceInfo(ctx context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
	var result *fileproto.SpaceInfoResponse
	err := s.doNodeReserved(ctx, func(cl fileproto.DRPCFileClient) error {
		resp, err := cl.SpaceInfo(ctx, &fileproto.SpaceInfoRequest{
			SpaceId: spaceId,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		result = resp
		return nil
	})
	return result, err
}

// FilesInfo retrieves info about specific files.
func (s *store) FilesInfo(ctx context.Context, spaceId string, fileIds ...domain.FileId) ([]*fileproto.FileInfo, error) {
	rawFileIds := make([]string, 0, len(fileIds))
	for _, id := range fileIds {
		rawFileIds = append(rawFileIds, id.String())
	}
	var result []*fileproto.FileInfo
	err := s.doNodeReserved(ctx, func(cl fileproto.DRPCFileClient) error {
		resp, err := cl.FilesInfo(ctx, &fileproto.FilesInfoRequest{
			SpaceId: spaceId,
			FileIds: rawFileIds,
		})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		result = resp.FilesInfo
		return nil
	})
	return result, err
}

// IterateFiles iterates over all files across all spaces via streaming RPC.
func (s *store) IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error {
	return s.doNodeDrpc(ctx, func(cl fileproto.DRPCFileClient) error {
		resp, err := cl.AccountInfo(ctx, &fileproto.AccountInfoRequest{})
		if err != nil {
			return rpcerr.Unwrap(err)
		}
		for _, space := range resp.Spaces {
			if err := iterateSpaceFiles(ctx, cl, space.SpaceId, iterFunc); err != nil {
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

// Close is a no-op; the reserved connection is closed with the pool component.
func (s *store) Close() error {
	return nil
}
