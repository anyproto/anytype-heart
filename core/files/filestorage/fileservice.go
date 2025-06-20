package filestorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/net/rpc/server"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
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

	NewLocalStoreGarbageCollector() LocalStoreGarbageCollector
	LocalDiskUsage(ctx context.Context) (uint64, error)
	IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error
}

type fileStorage struct {
	proxy   *proxyStore
	handler *rpcHandler

	cfg        *config.Config
	flatfsPath string

	rpcStore     rpcstore.Service
	spaceStorage storage.ClientStorage
	eventSender  event.Sender
}

var _ fileblockstore.BlockStoreLocal = &fileStorage{}

func (f *fileStorage) Init(a *app.App) (err error) {
	cfg := app.MustComponent[*config.Config](a)
	f.cfg = cfg
	fileCfg, err := cfg.FSConfig()
	if err != nil {
		return fmt.Errorf("fail to get file config: %w", err)
	}

	f.rpcStore = a.MustComponent(rpcstore.CName).(rpcstore.Service)
	f.spaceStorage = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	f.handler = &rpcHandler{spaceStorage: f.spaceStorage}
	f.eventSender = app.MustComponent[event.Sender](a)
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

func (f *fileStorage) Run(ctx context.Context) (err error) {
	localStore, err := newFlatStore(f.flatfsPath, f.eventSender, 1*time.Second)
	if err != nil {
		return fmt.Errorf("flatstore: %w", err)
	}
	f.handler.store = localStore

	ps := newProxyStore(localStore, f.rpcStore.NewStore())
	f.proxy = ps
	return
}

func (f *fileStorage) IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error {
	return f.proxy.origin.IterateFiles(ctx, iterFunc)
}

func (f *fileStorage) LocalDiskUsage(ctx context.Context) (uint64, error) {
	return f.proxy.localStore.ds.DiskUsage(ctx)
}

func (f *fileStorage) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	return f.proxy.Get(ctx, k)
}

func (f *fileStorage) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	return f.proxy.GetMany(ctx, ks)
}

func (f *fileStorage) Add(ctx context.Context, bs []blocks.Block) (err error) {
	return f.proxy.Add(ctx, bs)
}

func (f *fileStorage) Delete(ctx context.Context, k cid.Cid) error {
	return f.proxy.Delete(ctx, k)
}

func (f *fileStorage) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	return f.proxy.ExistsCids(ctx, ks)
}

func (f *fileStorage) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	return f.proxy.NotExistsBlocks(ctx, bs)
}

func (f *fileStorage) NewLocalStoreGarbageCollector() LocalStoreGarbageCollector {
	return newFlatStoreGarbageCollector(f.proxy.localStore)
}

func (f *fileStorage) Close(ctx context.Context) (err error) {
	if f.proxy != nil {
		return f.proxy.Close()
	}
	return nil
}
