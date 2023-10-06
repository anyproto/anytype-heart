package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

var log = logging.Logger("anytype-core")

const (
	CName = "anytype"
)

type Service interface {
	Stop() error
	IsStarted() bool
	AccountObjects() threads.DerivedSmartblockIds
	PredefinedObjects(spaceID string) threads.DerivedSmartblockIds
	GetSystemTypeID(spaceID string, typeKey domain.TypeKey) string
	GetSystemRelationID(spaceID string, relationKey domain.RelationKey) string

	ProfileInfo

	app.ComponentRunnable
}

var _ app.Component = (*Anytype)(nil)

var _ Service = (*Anytype)(nil)

type personalSpaceIDGetter interface {
	PersonalSpaceID() string
}

type derivedIDsGetter interface {
	DerivedIDs(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error)
}

type Anytype struct {
	derivedIDs     derivedIDsGetter
	personalGetter personalSpaceIDGetter
	objectStore    objectstore.ObjectStore

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
		shutdownStartsCh: make(chan struct{}),
	}
}

func (a *Anytype) Init(ap *app.App) (err error) {
	a.wallet = ap.MustComponent(wallet.CName).(wallet.Wallet)
	a.config = ap.MustComponent(config.CName).(*config.Config)
	a.objectStore = ap.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	a.commonFiles = ap.MustComponent(fileservice.CName).(fileservice.FileService)
	a.derivedIDs = app.MustComponent[derivedIDsGetter](ap)
	a.personalGetter = app.MustComponent[personalSpaceIDGetter](ap)
	return
}

func (a *Anytype) Name() string {
	return CName
}

func (a *Anytype) Run(ctx context.Context) (err error) {
	a.start()
	return nil
}

// TODO: refactor to call tech space
func (a *Anytype) IsStarted() bool {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.isStarted
}

// PredefinedBlocks returns default blocks like home and archive
// ⚠️ Will return empty struct in case it runs before Anytype.Start()
// TODO Its deprecated
func (a *Anytype) AccountObjects() threads.DerivedSmartblockIds {
	return a.PredefinedObjects(a.personalGetter.PersonalSpaceID())
}

func (a *Anytype) PredefinedObjects(spaceID string) threads.DerivedSmartblockIds {
	if spaceID == addr.AnytypeMarketplaceWorkspace {
		return threads.DerivedSmartblockIds{}
	}
	ids, err := a.derivedIDs.DerivedIDs(context.Background(), spaceID)
	if err != nil {
		log.Error("failed to get account objects", zap.Error(err))
		return threads.DerivedSmartblockIds{}
	}
	return ids
}

func (a *Anytype) GetSystemTypeID(spaceID string, typeKey domain.TypeKey) string {
	return a.PredefinedObjects(spaceID).SystemTypes[typeKey]
}

func (a *Anytype) GetSystemRelationID(spaceID string, relationKey domain.RelationKey) string {
	return a.PredefinedObjects(spaceID).SystemRelations[relationKey]
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
