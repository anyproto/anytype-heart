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
	"github.com/dgraph-io/badger/v3"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"os"
	"path/filepath"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/storage"
)

const CName = fileblockstore.CName
const FlatfsDirName = "flatfs"

var log = logger.NewNamed(CName)

func New() FileStorage {
	return &fileStorage{}
}

type FileStorage interface {
	fileblockstore.BlockStoreLocal
	app.ComponentRunnable
}

type fileStorage struct {
	fileblockstore.BlockStoreLocal

	cfg          *config.Config
	flatfsPath   string
	provider     datastore.Datastore
	rpcStore     rpcstore.Service
	spaceService space.Service
	handler      *rpcHandler
	spaceStorage storage.ClientStorage
}

func (f *fileStorage) Init(a *app.App) (err error) {
	cfg := app.MustComponent[*config.Config](a)
	f.cfg = cfg
	fileCfg, err := cfg.FSConfig()
	if err != nil {
		return fmt.Errorf("fail to get file config: %s", err)
	}

	f.rpcStore = a.MustComponent(rpcstore.CName).(rpcstore.Service)
	f.spaceStorage = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	f.handler = &rpcHandler{spaceStorage: f.spaceStorage}
	f.spaceService = a.MustComponent(space.CName).(space.Service)
	if fileCfg.IPFSStorageAddr == "" {
		f.flatfsPath = filepath.Join(app.MustComponent[wallet.Wallet](a).RepoPath(), FlatfsDirName)
	} else {
		if _, err := os.Stat(fileCfg.IPFSStorageAddr); os.IsNotExist(err) {
			return fmt.Errorf("local storage by address: %s not found", fileCfg.IPFSStorageAddr)
		}
		f.flatfsPath = fileCfg.IPFSStorageAddr
	}

	return fileproto.DRPCRegisterFile(a.MustComponent(server.CName).(server.DRPCServer), f.handler)
}

func (f *fileStorage) Name() (name string) {
	return CName
}

func (f *fileStorage) patchAccountIdCtx(ctx context.Context) context.Context {
	return fileblockstore.CtxWithSpaceId(ctx, f.spaceService.AccountId())
}

func (f *fileStorage) Run(ctx context.Context) (err error) {
	bs, err := newFlatStore(f.flatfsPath)
	if err != nil {
		return fmt.Errorf("flatstore: %w", err)
	}
	f.handler.store = bs

	var oldStore *badger.DB
	if f.cfg.LegacyFileStorePath != "" {
		oldStore, err = badger.Open(badger.DefaultOptions(f.cfg.LegacyFileStorePath))
		if err != nil {
			return fmt.Errorf("open old file store: %w", err)
		}
	}
	ps := &proxyStore{
		cache:    bs,
		origin:   f.rpcStore.NewStore(),
		oldStore: oldStore,
	}
	f.BlockStoreLocal = ps
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

func (f *fileStorage) Close(ctx context.Context) (err error) {
	return
}
