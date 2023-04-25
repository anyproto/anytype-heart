package filestorage

import (
	"context"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonfile/fileblockstore"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/badgerfilestore"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"io"
)

const CName = fileblockstore.CName

var log = logger.NewNamed(CName)

func New() FileStorage {
	return &fileStorage{}
}

type FileStorage interface {
	app.ComponentRunnable
	fileblockstore.BlockStore
}

type fileStorage struct {
	fileblockstore.BlockStore
	syncer       *syncer
	syncerCancel context.CancelFunc
}

func (f *fileStorage) Init(a *app.App) (err error) {
	provider := a.MustComponent(datastore.CName).(datastore.Datastore)
	db, err := provider.Badger()
	if err != nil {
		return
	}
	bs := badgerfilestore.NewBadgerStorage(db)
	ps := &proxyStore{
		cache:  bs,
		origin: a.MustComponent(rpcstore.CName).(rpcstore.Service).NewStore(),
		index:  badgerfilestore.NewFileBadgerIndex(db),
	}
	f.BlockStore = ps
	f.syncer = &syncer{ps: ps, done: make(chan struct{})}
	return
}

func (f *fileStorage) Name() (name string) {
	return CName
}

func (f *fileStorage) Run(ctx context.Context) (err error) {
	ctx, f.syncerCancel = context.WithCancel(ctx)
	go f.syncer.run(ctx)
	return
}

func (f *fileStorage) Close(ctx context.Context) (err error) {
	if f.syncerCancel != nil {
		f.syncerCancel()
		<-f.syncer.done
	}
	return f.BlockStore.(io.Closer).Close()
}
