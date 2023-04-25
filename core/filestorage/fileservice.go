package filestorage

import (
	"context"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonfile/fileblockstore"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/badgerfilestore"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
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
	fileblockstore.BlockStoreLocal
	syncer       *syncer
	syncerCancel context.CancelFunc
	provider     datastore.Datastore
	rpcStore     rpcstore.Service
}

func (f *fileStorage) Init(a *app.App) (err error) {
	f.provider = a.MustComponent(datastore.CName).(datastore.Datastore)
	f.rpcStore = a.MustComponent(rpcstore.CName).(rpcstore.Service)
	return
}

func (f *fileStorage) Name() (name string) {
	return CName
}

func (f *fileStorage) Run(ctx context.Context) (err error) {
	db, err := f.provider.Badger()
	if err != nil {
		return
	}
	bs := badgerfilestore.NewBadgerStorage(db)
	// TODO: rpcHandler
	ps := &proxyStore{
		cache:  bs,
		origin: f.rpcStore.NewStore(),
		index:  badgerfilestore.NewFileBadgerIndex(db),
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
