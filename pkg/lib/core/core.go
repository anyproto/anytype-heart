package core

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/app"
	"github.com/anytypeio/go-anytype-infrastructure-experiments/common/commonspace/object/treegetter"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/configfetcher"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pin"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"go.uber.org/zap"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var log = logging.Logger("anytype-core")

var ErrObjectDoesNotBelongToWorkspace = fmt.Errorf("object does not belong to workspace")

const (
	CName  = "anytype"
	tmpDir = "tmp"
)

type Service interface {
	Account() string // deprecated, use wallet component
	Device() string  // deprecated, use wallet component
	Start() error
	Stop() error
	IsStarted() bool
	SpaceService() space.Service

	EnsurePredefinedBlocks(ctx context.Context, mustSyncFromRemote bool) error
	PredefinedBlocks() threads.DerivedSmartblockIds

	// FileOffload removes file blocks recursively, but leave details
	FileOffload(id string) (bytesRemoved uint64, err error)

	FileByHash(ctx context.Context, hash string) (File, error)
	FileAdd(ctx context.Context, opts ...files.AddOption) (File, error)
	FileAddWithBytes(ctx context.Context, content []byte, filename string) (File, error)         // deprecated
	FileAddWithReader(ctx context.Context, content io.ReadSeeker, filename string) (File, error) // deprecated
	FileGetKeys(hash string) (*files.FileKeys, error)
	FileStoreKeys(fileKeys ...files.FileKeys) error

	ImageByHash(ctx context.Context, hash string) (Image, error)
	ImageAdd(ctx context.Context, opts ...files.AddOption) (Image, error)
	ImageAddWithBytes(ctx context.Context, content []byte, filename string) (Image, error)         // deprecated
	ImageAddWithReader(ctx context.Context, content io.ReadSeeker, filename string) (Image, error) // deprecated

	GetAllWorkspaces() ([]string, error)
	GetWorkspaceIdForObject(objectId string) (string, error)

	ObjectStore() objectstore.ObjectStore // deprecated
	FileStore() filestore.FileStore       // deprecated
	ThreadsIds() ([]string, error)        // deprecated

	ObjectInfoWithLinks(id string) (*model.ObjectInfoWithLinks, error)
	ObjectList() ([]*model.ObjectInfo, error)

	ProfileInfo

	app.ComponentRunnable
	TempDir() string
}

var _ app.Component = (*Anytype)(nil)

var _ Service = (*Anytype)(nil)

type initFunc = func(id string) *smartblock.InitContext
type ObjectsDeriver interface {
	DeriveTreeObject(ctx context.Context, tp coresb.SmartBlockType, f initFunc) (sb smartblock.SmartBlock, release func(), err error)
}

type Anytype struct {
	files        *files.Service
	cafe         cafe.Client
	mdns         mdns.Service
	objectStore  objectstore.ObjectStore
	fileStore    filestore.FileStore
	fetcher      configfetcher.ConfigFetcher
	sendEvent    func(event *pb.Event)
	deriver      ObjectsDeriver
	spaceService space.Service

	ds datastore.Datastore

	predefinedBlockIds threads.DerivedSmartblockIds
	pinService         pin.FilePinService
	ipfs               ipfs.Node
	logLevels          map[string]string

	opts ServiceOptions

	replicationWG    sync.WaitGroup
	migrationOnce    sync.Once
	lock             sync.Mutex
	isStarted        bool // use under the lock
	shutdownStartsCh chan struct {
	} // closed when node shutdown starts

	recordsbatch        batchAdder
	subscribeOnce       sync.Once
	config              *config.Config
	wallet              wallet.Wallet
	tmpFolderAutocreate sync.Once
	tempDir             string
}

func (a *Anytype) ThreadsIds() ([]string, error) {
	return nil, nil
}

type batchAdder interface {
	Add(msgs ...interface{}) error
	Close(ctx context.Context) (err error)
}

func New() *Anytype {
	return &Anytype{
		shutdownStartsCh: make(chan struct{}),
	}
}

func (a *Anytype) Init(ap *app.App) (err error) {
	a.wallet = ap.MustComponent(wallet.CName).(wallet.Wallet)
	a.config = ap.MustComponent(config.CName).(*config.Config)
	a.recordsbatch = ap.MustComponent("recordsbatcher").(batchAdder)
	a.objectStore = ap.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	a.fileStore = ap.MustComponent(filestore.CName).(filestore.FileStore)
	a.ds = ap.MustComponent(datastore.CName).(datastore.Datastore)
	a.cafe = ap.MustComponent(cafe.CName).(cafe.Client)
	a.files = ap.MustComponent(files.CName).(*files.Service)
	a.pinService = ap.MustComponent(pin.CName).(pin.FilePinService)
	a.ipfs = ap.MustComponent(ipfs.CName).(ipfs.Node)
	a.sendEvent = ap.MustComponent(event.CName).(event.Sender).Send
	a.fetcher = ap.MustComponent(configfetcher.CName).(configfetcher.ConfigFetcher)
	a.deriver = ap.MustComponent(treegetter.CName).(ObjectsDeriver)
	a.spaceService = ap.MustComponent(space.CName).(space.Service)
	return
}

func (a *Anytype) Name() string {
	return CName
}

func (a *Anytype) SpaceService() space.Service {
	return a.spaceService
}

// Deprecated, use wallet component directly
func (a *Anytype) Account() string {
	pk, _ := a.wallet.GetAccountPrivkey()
	if pk == nil {
		return ""
	}
	return pk.Address()
}

// Deprecated, use wallet component directly
func (a *Anytype) Device() string {
	pk, _ := a.wallet.GetDevicePrivkey()
	if pk == nil {
		return ""
	}
	return pk.Address()
}

func (a *Anytype) Run(ctx context.Context) (err error) {
	if err = a.Start(); err != nil {
		return
	}

	return a.EnsurePredefinedBlocks(ctx, a.config.NewAccount)
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

func (a *Anytype) Start() error {
	err := a.RunMigrations()
	if err != nil {
		return err
	}

	return a.start()
}

func (a *Anytype) start() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.isStarted {
		return nil
	}

	a.isStarted = true
	return nil
}

func (a *Anytype) EnsurePredefinedBlocks(ctx context.Context, newAccount bool) (err error) {
	sbTypes := []coresb.SmartBlockType{
		coresb.SmartBlockTypeArchive,
		coresb.SmartblockTypeMarketplaceType,
		coresb.SmartblockTypeMarketplaceRelation,
		coresb.SmartblockTypeMarketplaceTemplate,
		coresb.SmartBlockTypeWidget,
		coresb.SmartBlockTypeProfilePage,
		coresb.SmartBlockTypeWorkspace,
		coresb.SmartBlockTypeHome,
	}
	for _, sbt := range sbTypes {
		obj, release, err := a.deriver.DeriveTreeObject(ctx, sbt, func(id string) *smartblock.InitContext {
			return &smartblock.InitContext{Ctx: ctx, State: state.NewDoc(id, nil).(*state.State)}
		})
		if err != nil {
			log.With(zap.Error(err)).Debug("derived object with error")
			return
		}
		a.predefinedBlockIds.InsertId(sbt, obj.Id())
		release()
	}

	// TODO: [MR] derive trees in the new infra
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

	// fixme useless!
	a.replicationWG.Wait()

	return nil
}

func (a *Anytype) TempDir() string {
	// it shouldn't be a case when it is called before wallet init, but just in case lets add the check here
	if a.wallet == nil || a.wallet.RootPath() == "" {
		return os.TempDir()
	}

	var err error
	// simultaneous calls to TempDir will wait for the once func to finish, so it will be fine
	a.tmpFolderAutocreate.Do(func() {
		path := filepath.Join(a.wallet.RootPath(), tmpDir)
		err = os.MkdirAll(path, 0700)
		if err != nil {
			log.Errorf("failed to make temp dir, use the default system one: %s", err.Error())
			a.tempDir = os.TempDir()
		} else {
			a.tempDir = path
		}
	})

	return a.tempDir
}
