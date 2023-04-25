package filestorage

import (
	"context"
	"fmt"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonfile/fileblockstore"
	"github.com/anytypeio/any-sync/commonfile/fileproto"
	"github.com/anytypeio/any-sync/commonspace/spacestorage"
	"github.com/anytypeio/any-sync/net/rpc/server"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"

	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/storage"
)

const CName = fileblockstore.CName

var log = logger.NewNamed(CName)

func New() FileStorage {
	return &fileStorage{}
}

type FileStorage interface {
	app.ComponentRunnable
	fileblockstore.BlockStoreLocal
}

type fileStorage struct {
	// TODO use cache proxy directly
	fileblockstore.BlockStoreLocal

	syncer       *syncer
	syncerCancel context.CancelFunc
	provider     datastore.Datastore
	rpcStore     rpcstore.Service
	spaceService space.Service
	handler      *rpcHandler
	spaceStorage storage.ClientStorage
}

func (f *fileStorage) Init(a *app.App) (err error) {
	f.provider = a.MustComponent(datastore.CName).(datastore.Datastore)
	f.rpcStore = a.MustComponent(rpcstore.CName).(rpcstore.Service)
	f.spaceStorage = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	f.handler = &rpcHandler{spaceStorage: f.spaceStorage}
	f.spaceService = a.MustComponent(space.CName).(space.Service)
	return fileproto.DRPCRegisterFile(a.MustComponent(server.CName).(server.DRPCServer), f.handler)
}

func (f *fileStorage) Name() (name string) {
	return CName
}

func (f *fileStorage) patchAccountIdCtx(ctx context.Context) context.Context {
	return fileblockstore.CtxWithSpaceId(ctx, f.spaceService.AccountId())
}

func (f *fileStorage) Run(ctx context.Context) (err error) {
	db, err := f.provider.Badger()
	if err != nil {
		return
	}
	bs, err := newFlatStore()
	if err != nil {
		return fmt.Errorf("flatstore: %w", err)
	}
	f.handler.store = bs
	ps := &proxyStore{
		cache:  bs,
		origin: f.rpcStore.NewStore(),
		index:  NewFileBadgerIndex(db),
	}
	f.BlockStoreLocal = ps
	f.syncer = &syncer{ps: ps, done: make(chan struct{})}
	ctx, f.syncerCancel = context.WithCancel(ctx)
	go f.syncer.run(ctx)
	return
}

func (f *fileStorage) Close(ctx context.Context) (err error) {
	if f.syncerCancel != nil {
		f.syncerCancel()
		<-f.syncer.done
	}
	return
}

func (f *fileStorage) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	return f.BlockStoreLocal.Get(f.patchAccountIdCtx(ctx), k)
}

func (f *fileStorage) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	return f.BlockStoreLocal.GetMany(f.patchAccountIdCtx(ctx), ks)
}

func (f *fileStorage) Add(ctx context.Context, bs []blocks.Block) (err error) {
	return f.BlockStoreLocal.Add(f.patchAccountIdCtx(ctx), bs)
}

func (f *fileStorage) Delete(ctx context.Context, k cid.Cid) error {
	return f.BlockStoreLocal.Delete(f.patchAccountIdCtx(ctx), k)
}

func (f *fileStorage) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	return f.BlockStoreLocal.ExistsCids(f.patchAccountIdCtx(ctx), ks)
}

func (f *fileStorage) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	return f.BlockStoreLocal.NotExistsBlocks(f.patchAccountIdCtx(ctx), bs)
}
