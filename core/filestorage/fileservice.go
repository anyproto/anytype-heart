package filestorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonfile/fileblockstore"
	"github.com/anytypeio/any-sync/commonfile/fileproto"
	"github.com/anytypeio/any-sync/commonspace/spacestorage"
	"github.com/anytypeio/any-sync/net/rpc/server"
	"github.com/dgraph-io/badger/v3"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

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

	LocalDiskUsage(ctx context.Context) (uint64, error)
}

type fileStorage struct {
	proxy   *proxyStore
	handler *rpcHandler

	cfg        *config.Config
	flatfsPath string

	provider     datastore.Datastore
	rpcStore     rpcstore.Service
	spaceService space.Service
	spaceStorage storage.ClientStorage
}

var _ fileblockstore.BlockStoreLocal = &fileStorage{}

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
	localStore, err := newFlatStore(f.flatfsPath)
	if err != nil {
		return fmt.Errorf("flatstore: %w", err)
	}
	f.handler.store = localStore

	oldStore, storeErr := f.initOldStore()
	if storeErr != nil {
		log.Error("can't open legacy file store", zap.Error(storeErr))
	}
	ps := &proxyStore{
		localStore: localStore,
		origin:     f.rpcStore.NewStore(),
		oldStore:   oldStore,
	}
	f.proxy = ps
	return
}

func (f *fileStorage) initOldStore() (*badger.DB, error) {
	if f.cfg.LegacyFileStorePath == "" {
		return nil, nil
	}
	if _, err := os.Stat(f.cfg.LegacyFileStorePath); os.IsNotExist(err) {
		return nil, nil
	}
	return badger.Open(badger.DefaultOptions(f.cfg.LegacyFileStorePath).WithReadOnly(true).WithBypassLockGuard(true))
}

func (f *fileStorage) LocalDiskUsage(ctx context.Context) (uint64, error) {
	return f.proxy.localStore.ds.DiskUsage(ctx)
}

func (f *fileStorage) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	return f.proxy.Get(f.patchAccountIdCtx(ctx), k)
}

func (f *fileStorage) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	return f.proxy.GetMany(f.patchAccountIdCtx(ctx), ks)
}

func (f *fileStorage) Add(ctx context.Context, bs []blocks.Block) (err error) {
	return f.proxy.Add(f.patchAccountIdCtx(ctx), bs)
}

func (f *fileStorage) Delete(ctx context.Context, k cid.Cid) error {
	return f.proxy.Delete(f.patchAccountIdCtx(ctx), k)
}

func (f *fileStorage) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	return f.proxy.ExistsCids(f.patchAccountIdCtx(ctx), ks)
}

func (f *fileStorage) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	return f.proxy.NotExistsBlocks(f.patchAccountIdCtx(ctx), bs)
}

func (f *fileStorage) Close(ctx context.Context) (err error) {
	return f.proxy.Close()
}
