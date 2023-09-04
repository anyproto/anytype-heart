package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/object/treemanager"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space"
	"github.com/libp2p/go-libp2p/core/peer"
)

var log = logging.Logger("anytype-core")

var ErrObjectDoesNotBelongToWorkspace = fmt.Errorf("object does not belong to workspace")

const (
	CName = "anytype"
)

type Service interface {
	Stop() error
	IsStarted() bool

	DerivePredefinedObjects(ctx context.Context, spaceID string, createTrees bool) (predefinedObjectIDs threads.DerivedSmartblockIds, err error)

	DeriveObjectId(ctx context.Context, spaceID string, key domain.UniqueKey) (string, error)

	EnsurePredefinedBlocks(ctx context.Context, spaceID string) (predefinedObjectIDs threads.DerivedSmartblockIds, err error)
	AccountObjects() threads.DerivedSmartblockIds
	PredefinedObjects(spaceID string) threads.DerivedSmartblockIds
	GetSystemTypeID(spaceID string, typeKey bundle.TypeKey) string
	GetSystemRelationID(spaceID string, relationKey bundle.RelationKey) string

	ProfileInfo

	app.ComponentRunnable
}

var _ app.Component = (*Anytype)(nil)

var _ Service = (*Anytype)(nil)

type ObjectsDeriver interface {
	DeriveTreeCreatePayload(ctx context.Context, spaceID string, key domain.UniqueKey) (treestorage.TreeStorageCreatePayload, error)
	DeriveObject(ctx context.Context, spaceID string, payload treestorage.TreeStorageCreatePayload, newAccount bool) (err error)
}

type Anytype struct {
	space       space.Service
	objectStore objectstore.ObjectStore
	deriver     ObjectsDeriver

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
	a.deriver = ap.MustComponent(treemanager.CName).(ObjectsDeriver)
	a.space = app.MustComponent[space.Service](ap)
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

// PredefinedBlocks returns default blocks like home and archive
// ⚠️ Will return empty struct in case it runs before Anytype.Start()
// TODO Its deprecated
func (a *Anytype) AccountObjects() threads.DerivedSmartblockIds {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return a.predefinedObjectsPerSpace[a.space.AccountId()]
}

func (a *Anytype) PredefinedObjects(spaceID string) threads.DerivedSmartblockIds {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return a.predefinedObjectsPerSpace[spaceID]
}

func (a *Anytype) GetSystemTypeID(spaceID string, typeKey bundle.TypeKey) string {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return a.predefinedObjectsPerSpace[spaceID].SystemTypes[typeKey]
}

func (a *Anytype) GetSystemRelationID(spaceID string, relationKey bundle.RelationKey) string {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return a.predefinedObjectsPerSpace[spaceID].SystemRelations[relationKey]
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

func (a *Anytype) DeriveObjectId(ctx context.Context, spaceID string, key domain.UniqueKey) (string, error) {
	// todo: cache it or use the objectstore
	payload, err := a.deriver.DeriveTreeCreatePayload(ctx, spaceID, key)
	if err != nil {
		return "", fmt.Errorf("failed to derive tree create payload for space %s and key %s: %w", spaceID, key, err)
	}
	return payload.RootRawChange.Id, nil
}

func (a *Anytype) DerivePredefinedObjects(ctx context.Context, spaceID string, createTrees bool) (predefinedObjectIDs threads.DerivedSmartblockIds, err error) {
	a.lock.RLock()
	// TODO Weak condition
	ids, ok := a.predefinedObjectsPerSpace[spaceID]
	a.lock.RUnlock()
	if ok && ids.IsFilled() {
		return ids, nil
	}
	ids, err = a.derivePredefinedObjects(ctx, spaceID, createTrees)
	if err != nil {
		return threads.DerivedSmartblockIds{}, err
	}
	return ids, nil
}

func (a *Anytype) derivePredefinedObjects(ctx context.Context, spaceID string, createTrees bool) (predefinedObjectIDs threads.DerivedSmartblockIds, err error) {
	sbTypes := []coresb.SmartBlockType{
		coresb.SmartBlockTypeWorkspace,
		coresb.SmartBlockTypeProfilePage,
		coresb.SmartBlockTypeArchive,
		coresb.SmartBlockTypeWidget,
		coresb.SmartBlockTypeHome,
	}
	payloads := make([]treestorage.TreeStorageCreatePayload, len(sbTypes))
	predefinedObjectIDs.SystemRelations = make(map[bundle.RelationKey]string)
	predefinedObjectIDs.SystemTypes = make(map[bundle.TypeKey]string)

	for i, sbt := range sbTypes {
		a.lock.RLock()
		exists := a.predefinedObjectsPerSpace[spaceID].HasID(sbt)
		a.lock.RUnlock()

		if exists {
			continue
		}
		// we have only 1 object per sbtype so key is empty (also for the backward compatibility, because before we didn't have a key)
		uk, err := domain.NewUniqueKey(sbt, "")
		if err != nil {
			return predefinedObjectIDs, err
		}
		payloads[i], err = a.deriver.DeriveTreeCreatePayload(ctx, spaceID, uk)
		if err != nil {
			log.With("uniqueKey", uk).Errorf("create payload for derived object: %s", err)
			return predefinedObjectIDs, fmt.Errorf("derive tree create payload: %w", err)
		}
		predefinedObjectIDs.InsertId(sbt, payloads[i].RootRawChange.Id)
	}

	for _, ot := range bundle.SystemTypes {
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, ot.String())
		if err != nil {
			return predefinedObjectIDs, err
		}
		id, err := a.DeriveObjectId(ctx, spaceID, uk)
		if err != nil {
			return predefinedObjectIDs, err
		}
		predefinedObjectIDs.SystemTypes[ot] = id
	}

	for _, rk := range bundle.SystemRelations {
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, rk.String())
		if err != nil {
			return predefinedObjectIDs, err
		}
		id, err := a.DeriveObjectId(ctx, spaceID, uk)
		if err != nil {
			return predefinedObjectIDs, err
		}
		predefinedObjectIDs.SystemRelations[rk] = id
	}

	a.lock.Lock()
	a.predefinedObjectsPerSpace[spaceID] = predefinedObjectIDs
	a.lock.Unlock()

	for _, payload := range payloads {
		// todo: move types/relations derivation here
		err = a.deriver.DeriveObject(ctx, spaceID, payload, createTrees)
		if err != nil {
			log.With("id", payload.RootRawChange).Errorf("derive object: %s", err)
			return predefinedObjectIDs, fmt.Errorf("derive object: %w", err)
		}
	}

	return
}

func (a *Anytype) EnsurePredefinedBlocks(ctx context.Context, spaceID string) (threads.DerivedSmartblockIds, error) {
	return a.DerivePredefinedObjects(ctx, spaceID, a.config.NewAccount)
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
