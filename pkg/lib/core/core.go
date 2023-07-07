package core

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

var log = logging.Logger("anytype-core")

var ErrObjectDoesNotBelongToWorkspace = fmt.Errorf("object does not belong to workspace")

const (
	CName = "anytype"
)

type Service interface {
	Stop() error
	IsStarted() bool

	AccountObjects() threads.DerivedSmartblockIds
	PredefinedObjects(spaceID string) threads.DerivedSmartblockIds

	RegisterPredefinedObjects(spaceID string, ids threads.DerivedSmartblockIds)
	SetAccountSpaceID(spaceID string)
	GetAllWorkspaces() ([]string, error)
	GetWorkspaceIdForObject(spaceID string, objectID string) (string, error)

	ProfileInfo

	app.ComponentRunnable
}

var _ app.Component = (*Anytype)(nil)

var _ Service = (*Anytype)(nil)

type Anytype struct {
	objectStore objectstore.ObjectStore

	accountSpaceID            string
	predefinedObjectsPerSpace map[string]threads.DerivedSmartblockIds

	migrationOnce    sync.Once
	lock             sync.RWMutex
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
		shutdownStartsCh:          make(chan struct{}),
		predefinedObjectsPerSpace: make(map[string]threads.DerivedSmartblockIds),
	}
}

func (a *Anytype) Init(ap *app.App) (err error) {
	a.wallet = ap.MustComponent(wallet.CName).(wallet.Wallet)
	a.config = ap.MustComponent(config.CName).(*config.Config)
	a.objectStore = ap.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	a.commonFiles = ap.MustComponent(fileservice.CName).(fileservice.FileService)
	return
}

func (a *Anytype) Name() string {
	return CName
}

func (a *Anytype) Run(ctx context.Context) (err error) {
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

func (a *Anytype) GetWorkspaceIdForObject(spaceID string, objectID string) (string, error) {
	if strings.HasPrefix(objectID, "_") {
		return addr.AnytypeMarketplaceWorkspace, nil
	}
	a.lock.RLock()
	ids := a.predefinedObjectsPerSpace[spaceID]
	a.lock.RUnlock()

	if ids.IsAccount(objectID) {
		return "", ErrObjectDoesNotBelongToWorkspace
	}
	return ids.Account, nil
}

// PredefinedBlocks returns default blocks like home and archive
// ⚠️ Will return empty struct in case it runs before Anytype.Start()
// TODO Its deprecated
func (a *Anytype) AccountObjects() threads.DerivedSmartblockIds {
	a.lock.RLock()
	defer a.lock.RUnlock()

	return a.predefinedObjectsPerSpace[a.accountSpaceID]
}

func (a *Anytype) SetAccountSpaceID(spaceID string) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.accountSpaceID = spaceID
}

func (a *Anytype) RegisterPredefinedObjects(spaceID string, ids threads.DerivedSmartblockIds) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.predefinedObjectsPerSpace[spaceID] = ids
}

func (a *Anytype) PredefinedObjects(spaceID string) threads.DerivedSmartblockIds {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return a.predefinedObjectsPerSpace[spaceID]
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
