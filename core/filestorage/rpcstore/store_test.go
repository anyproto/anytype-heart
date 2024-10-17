package rpcstore

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/net/rpc/rpctest"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

var ctx = context.Background()

func TestStore_Put(t *testing.T) {
	fx := newFixture(t)
	defer fx.Finish(t)

	bs := []blocks.Block{
		blocks.NewBlock([]byte{'1'}),
		blocks.NewBlock([]byte{'2'}),
		blocks.NewBlock([]byte{'3'}),
	}
	err := fx.AddToFile(ctx, "", "", bs)
	assert.NoError(t, err)
	for _, b := range bs {
		assert.NotNil(t, fx.serv.data[string(b.Cid().Bytes())])
	}
}

func TestStore_DeleteFiles(t *testing.T) {
	fx := newFixture(t)
	defer fx.Finish(t)
	bs := []blocks.Block{
		blocks.NewBlock([]byte{'1'}),
	}
	require.NoError(t, fx.AddToFile(ctx, "spaceId", "fileId", bs))
	assert.Len(t, fx.serv.data, 1)
	assert.Len(t, fx.serv.files, 1)
	require.NoError(t, fx.DeleteFiles(ctx, "spaceId", "fileId"))
	assert.Len(t, fx.serv.files, 0)
}

func TestStore_Get(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.Finish(t)
		bs := []blocks.Block{
			blocks.NewBlock([]byte{'1'}),
		}
		err := fx.AddToFile(ctx, "", "", bs)
		require.NoError(t, err)
		b, err := fx.Get(ctx, bs[0].Cid())
		require.NoError(t, err)
		assert.Equal(t, []byte{'1'}, b.RawData())
	})
	t.Run("not found", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.Finish(t)
		bs := []blocks.Block{
			blocks.NewBlock([]byte{'1'}),
		}
		b, err := fx.Get(ctx, bs[0].Cid())
		assert.Nil(t, b)
		assert.ErrorIs(t, err, format.ErrNotFound{})
	})
}

func TestStore_GetMany(t *testing.T) {
	fx := newFixture(t)
	defer fx.Finish(t)

	bs := []blocks.Block{
		blocks.NewBlock([]byte{'1'}),
		blocks.NewBlock([]byte{'2'}),
		blocks.NewBlock([]byte{'3'}),
	}
	err := fx.AddToFile(ctx, "", "", bs)
	assert.NoError(t, err)

	res := fx.GetMany(ctx, []cid.Cid{
		bs[0].Cid(),
		bs[1].Cid(),
		bs[2].Cid(),
	})
	var resBlocks []blocks.Block
	for b := range res {
		resBlocks = append(resBlocks, b)
	}
	require.Len(t, resBlocks, 3)
	sort.Slice(resBlocks, func(i, j int) bool {
		return string(resBlocks[i].RawData()) < string(resBlocks[j].RawData())
	})
	assert.Equal(t, bs, resBlocks)
}

func TestStore_AddAsync(t *testing.T) {
	fx := newFixture(t)
	defer fx.Finish(t)

	bs := []blocks.Block{
		blocks.NewBlock([]byte{'1'}),
		blocks.NewBlock([]byte{'2'}),
		blocks.NewBlock([]byte{'3'}),
	}
	err := fx.AddToFile(ctx, "", "", bs[:1])
	assert.NoError(t, err)

	require.NoError(t, fx.AddToFile(ctx, "", "", bs))
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		a: new(app.App),
		s: New().(*service),
		serv: &testServer{
			data:  make(map[string][]byte),
			files: make(map[string][][]byte),
		},
		ctrl:     ctrl,
		nodeConf: mock_nodeconf.NewMockService(ctrl),
	}

	var filePeers []string
	for i := 0; i < 11; i++ {
		filePeers = append(filePeers, fmt.Sprint(i))
	}
	rserv := rpctest.NewTestServer()
	require.NoError(t, fileproto.DRPCRegisterFile(rserv.Mux, fx.serv))

	fx.nodeConf.EXPECT().Name().Return(nodeconf.CName).AnyTimes()
	fx.nodeConf.EXPECT().Init(fx.a).AnyTimes()
	fx.nodeConf.EXPECT().Run(ctx).AnyTimes()
	fx.nodeConf.EXPECT().Close(ctx).AnyTimes()
	fx.nodeConf.EXPECT().FilePeers().Return(filePeers).AnyTimes()

	fx.a.Register(fx.s).
		Register(mock_accountservice.NewAccountServiceWithAccount(fx.ctrl, &accountdata.AccountKeys{})).
		Register(rpctest.NewTestPool().WithServer(rserv)).
		Register(fx.nodeConf).
		Register(peerstore.New())
	require.NoError(t, fx.a.Start(ctx))
	fx.store = fx.s.NewStore().(*store)
	return fx
}

type fixture struct {
	*store
	s        *service
	a        *app.App
	serv     *testServer
	ctrl     *gomock.Controller
	nodeConf *mock_nodeconf.MockService
}

func (fx *fixture) Finish(t *testing.T) {
	assert.NoError(t, fx.store.Close())
	assert.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}

type testServer struct {
	mu    sync.Mutex
	data  map[string][]byte
	files map[string][][]byte
}

func (t *testServer) FilesGet(request *fileproto.FilesGetRequest, stream fileproto.DRPCFile_FilesGetStream) error {
	return fileprotoerr.ErrForbidden
}

func (t *testServer) AccountLimitSet(ctx context.Context, request *fileproto.AccountLimitSetRequest) (*fileproto.Ok, error) {
	return nil, fileprotoerr.ErrForbidden
}

func (t *testServer) SpaceLimitSet(ctx context.Context, request *fileproto.SpaceLimitSetRequest) (*fileproto.Ok, error) {
	return nil, fileprotoerr.ErrForbidden
}

func (t *testServer) BlockGet(ctx context.Context, req *fileproto.BlockGetRequest) (resp *fileproto.BlockGetResponse, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if data, ok := t.data[string(req.Cid)]; ok {
		return &fileproto.BlockGetResponse{
			Cid:  req.Cid,
			Data: data,
		}, nil
	} else {
		return nil, fileprotoerr.ErrCIDNotFound
	}
}

func (t *testServer) BlockPush(ctx context.Context, req *fileproto.BlockPushRequest) (*fileproto.Ok, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data[string(req.Cid)] = req.Data
	fcids := t.files[req.FileId]
	t.files[req.FileId] = append(fcids, req.Cid)
	return &fileproto.Ok{}, nil
}

func (t *testServer) BlocksCheck(ctx context.Context, req *fileproto.BlocksCheckRequest) (resp *fileproto.BlocksCheckResponse, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	resp = &fileproto.BlocksCheckResponse{}
	for _, c := range req.Cids {
		status := fileproto.AvailabilityStatus_NotExists
		if _, ok := t.data[string(c)]; ok {
			status = fileproto.AvailabilityStatus_Exists
		}
		resp.BlocksAvailability = append(resp.BlocksAvailability, &fileproto.BlockAvailability{
			Cid:    c,
			Status: status,
		})
	}
	return
}

func (t *testServer) BlocksBind(ctx context.Context, req *fileproto.BlocksBindRequest) (*fileproto.Ok, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.files[req.FileId] = append(t.files[req.FileId], req.Cids...)
	return &fileproto.Ok{}, nil
}

func (t *testServer) FilesDelete(ctx context.Context, req *fileproto.FilesDeleteRequest) (*fileproto.FilesDeleteResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, fileId := range req.FileIds {
		delete(t.files, fileId)
	}
	return &fileproto.FilesDeleteResponse{}, nil
}

func (t *testServer) FilesInfo(ctx context.Context, req *fileproto.FilesInfoRequest) (*fileproto.FilesInfoResponse, error) {
	resp := &fileproto.FilesInfoResponse{}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, fileId := range req.FileIds {
		resp.FilesInfo = append(resp.FilesInfo, &fileproto.FileInfo{
			FileId:     fileId,
			UsageBytes: uint64(len(t.files[fileId])),
			CidsCount:  uint32(len(t.files[fileId])),
		})
	}
	return resp, nil
}

func (t *testServer) SpaceInfo(ctx context.Context, req *fileproto.SpaceInfoRequest) (*fileproto.SpaceInfoResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	resp := &fileproto.SpaceInfoResponse{
		LimitBytes: 99999999,
	}
	for _, b := range t.data {
		resp.TotalUsageBytes += uint64(len(b))
		resp.CidsCount++
	}
	resp.FilesCount = uint64(len(t.files))
	return resp, nil
}

func (t *testServer) AccountInfo(ctx context.Context, req *fileproto.AccountInfoRequest) (*fileproto.AccountInfoResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	resp := &fileproto.AccountInfoResponse{
		LimitBytes: 99999999,
	}
	for _, b := range t.data {
		resp.TotalUsageBytes += uint64(len(b))
		resp.TotalCidsCount++
	}
	return resp, nil
}

func (t *testServer) Check(ctx context.Context, req *fileproto.CheckRequest) (*fileproto.CheckResponse, error) {
	return &fileproto.CheckResponse{AllowWrite: true}, nil
}
