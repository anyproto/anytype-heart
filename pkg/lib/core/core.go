package core

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonfile/fileservice"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"github.com/anytypeio/any-sync/commonspace/object/treemanager"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	files2 "github.com/anytypeio/go-anytype-middleware/core/files"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
)

var log = logging.Logger("anytype-core")

var ErrObjectDoesNotBelongToWorkspace = fmt.Errorf("object does not belong to workspace")

const (
	CName = "anytype"
)

type Service interface {
	Stop() error
	IsStarted() bool

	EnsurePredefinedBlocks(ctx context.Context) error
	PredefinedBlocks() threads.DerivedSmartblockIds

	FileByHash(ctx context.Context, hash string) (File, error)
	FileAdd(ctx context.Context, opts ...files2.AddOption) (File, error)
	FileStoreKeys(fileKeys ...files2.FileKeys) error

	ImageByHash(ctx context.Context, hash string) (Image, error)
	ImageAdd(ctx context.Context, opts ...files2.AddOption) (Image, error)

	GetAllWorkspaces() ([]string, error)
	GetWorkspaceIdForObject(objectId string) (string, error)

	ProfileInfo

	app.ComponentRunnable
}

var _ app.Component = (*Anytype)(nil)

var _ Service = (*Anytype)(nil)

type ObjectsDeriver interface {
	DeriveTreeCreatePayload(ctx context.Context, tp coresb.SmartBlockType) (*treestorage.TreeStorageCreatePayload, error)
	DeriveObject(ctx context.Context, payload *treestorage.TreeStorageCreatePayload, newAccount bool) (err error)
}

type Anytype struct {
	files            *files2.Service
	objectStore      objectstore.ObjectStore
	fileStore        filestore.FileStore
	fileBlockStorage filestorage.FileStorage
	deriver          ObjectsDeriver

	predefinedBlockIds threads.DerivedSmartblockIds

	migrationOnce    sync.Once
	lock             sync.Mutex
	isStarted        bool // use under the lock
	shutdownStartsCh chan struct {
	} // closed when node shutdown starts

	subscribeOnce sync.Once
	config        *config.Config
	wallet        wallet.Wallet

	commonFiles fileservice.FileService
}

func New() *Anytype {
	return &Anytype{
		shutdownStartsCh: make(chan struct{}),
	}
}

func (a *Anytype) Init(ap *app.App) (err error) {
	a.wallet = ap.MustComponent(wallet.CName).(wallet.Wallet)
	a.config = ap.MustComponent(config.CName).(*config.Config)
	a.objectStore = ap.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	a.fileStore = ap.MustComponent(filestore.CName).(filestore.FileStore)
	a.files = ap.MustComponent(files2.CName).(*files2.Service)
	a.commonFiles = ap.MustComponent(fileservice.CName).(fileservice.FileService)
	a.deriver = ap.MustComponent(treemanager.CName).(ObjectsDeriver)
	a.fileBlockStorage = app.MustComponent[filestorage.FileStorage](ap)
	return
}

func (a *Anytype) Name() string {
	return CName
}

func (a *Anytype) Run(ctx context.Context) (err error) {
	if err = a.RunMigrations(); err != nil {
		return
	}

	a.start()
	return nil
}

func (a *Anytype) IsStarted() bool {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.isStarted
}

func (a *Anytype) GetAllWorkspaces() ([]string, error) {
	return nil, nil
}

func (a *Anytype) GetWorkspaceIdForObject(objectId string) (string, error) {
	if strings.HasPrefix(objectId, "_") {
		return addr.AnytypeMarketplaceWorkspace, nil
	}
	if a.predefinedBlockIds.IsAccount(objectId) {
		return "", ErrObjectDoesNotBelongToWorkspace
	}
	return a.predefinedBlockIds.Account, nil
}

// PredefinedBlocks returns default blocks like home and archive
// ⚠️ Will return empty struct in case it runs before Anytype.Start()
func (a *Anytype) PredefinedBlocks() threads.DerivedSmartblockIds {
	return a.predefinedBlockIds
}

func (a *Anytype) HandlePeerFound(p peer.AddrInfo) {
	// TODO: [MR] mdns
}

func (a *Anytype) start() {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.isStarted {
		return
	}

	a.isStarted = true
}

func (a *Anytype) EnsurePredefinedBlocks(ctx context.Context) (err error) {
	sbTypes := []coresb.SmartBlockType{
		coresb.SmartBlockTypeWorkspace,
		coresb.SmartBlockTypeProfilePage,
		coresb.SmartBlockTypeArchive,
		coresb.SmartBlockTypeWidget,
		coresb.SmartBlockTypeHome,
	}
	payloads := make([]*treestorage.TreeStorageCreatePayload, len(sbTypes))
	for i, sbt := range sbTypes {
		payloads[i], err = a.deriver.DeriveTreeCreatePayload(ctx, sbt)
		if err != nil {
			log.With(zap.Error(err)).Debug("derived tree object with error")
			return
		}
		a.predefinedBlockIds.InsertId(sbt, payloads[i].RootRawChange.Id)
	}

	for _, payload := range payloads {
		err = a.deriver.DeriveObject(ctx, payload, a.config.NewAccount)
		if err != nil {
			log.With(zap.Error(err)).Debug("derived object with error")
			return
		}
	}

	return nil
}

func (a *Anytype) Close(ctx context.Context) (err error) {
	metrics.SharedClient.Close()
	return a.Stop()
}

func (a *Anytype) Stop() error {
	fmt.Printf("stopping the library...\n")
	defer fmt.Println("library has been successfully stopped")
	a.lock.Lock()
	defer a.lock.Unlock()
	a.isStarted = false

	if a.shutdownStartsCh != nil {
		close(a.shutdownStartsCh)
	}

	return nil
}
