package spacev2

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/inbox/inboxservice"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/deletioncontroller"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/virtualspaceservice"
)

// preloadRemainingSpacesTimeout is the lazy-mode safety net: deferred spaces
// are promoted after this delay even if no explicit preload arrives.
const preloadRemainingSpacesTimeout = 10 * time.Second

// NotificationSender sends client notifications (participant-remove on
// remote deletion); implemented by the notifications service.
type NotificationSender interface {
	app.Component
	CreateAndSend(notification *model.Notification) error
}

// AclJoiner performs the network ACL join used by the stream auto-join flow;
// implemented by core/acl.
type AclJoiner interface {
	Join(ctx context.Context, spaceId, networkId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error
}

// service is the v2 space orchestrator. It owns the controller registry, the
// SpaceView watcher (the reactive spine) and account bootstrap. The outward
// API surface (Get/Wait/Create/Join/Delete/...) builds on ensureController +
// SetWanted + WaitLoaded.
type service struct {
	spaceCore           spacecore.SpaceCoreService
	accountService      accountservice.Service
	config              *config.Config
	subService          subscription.Service
	objectFactory       objectcache.ObjectFactory
	storageService      storage.ClientStorage
	indexer             dependencies.SpaceIndexer
	installer           dependencies.BundledObjectsInstaller
	migrationService    dependencies.MigrationService
	fileOffloader       dependencies.FileOffloader
	delController       deletioncontroller.DeletionController
	virtualSpaceService virtualspaceservice.VirtualSpaceService
	notificationService NotificationSender
	spaceNameGetter     objectstore.SpaceNameGetter
	identityService     dependencies.IdentityService
	inboxSender         inboxservice.Sender
	aclJoiner           AclJoiner

	// parentApp is the chain per-space pipeline apps hang off: the main app
	// plus the tech space registered after resolution.
	parentApp *app.App

	personalSpaceId        string
	techSpaceId            string
	newAccount             bool
	preferredSpaceId       string
	lazyMode               bool
	autoJoinStreamSpace    string
	repKey                 uint64
	accountMetadataSymKey  crypto.SymKey
	accountMetadataPayload []byte
	firstCreatedSpaceId    string

	techSpace *clientspace.TechSpace
	// techSpaceReady is the happens-before barrier for s.techSpace: closed
	// exactly once, only after assignment; left open on failed bootstrap so
	// tech-space getters block instead of reading nil.
	techSpaceReady chan struct{}

	backendDeps *BackendDeps
	registry    *registry
	watcher     *spaceWatcher
	marketplace *marketplaceSpace

	preloadOnce sync.Once
	preloadCh   chan struct{} // closed by triggerPreload (lazy drain)
	// lazyReleased flips once the deferred backlog is drained: from then on
	// lazy mode is over and every discovery — including SpaceViews that first
	// appear later in the session (shared/created on another device) — is
	// demanded eagerly, matching v1's collapse-to-eager `releasing` flag.
	lazyReleased atomic.Bool
	ctx          context.Context
	ctxCancel    context.CancelFunc
}

// The deletion controller drives remote-status updates through this service
// (poll results, shared limits); keep the seam satisfied.
var _ deletioncontroller.SpaceManager = (*service)(nil)

func New() *service {
	return &service{}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.config = app.MustComponent[*config.Config](a)
	s.subService = app.MustComponent[subscription.Service](a)
	s.objectFactory = app.MustComponent[objectcache.ObjectFactory](a)
	s.storageService = app.MustComponent[storage.ClientStorage](a)
	s.indexer = app.MustComponent[dependencies.SpaceIndexer](a)
	s.installer = app.MustComponent[dependencies.BundledObjectsInstaller](a)
	s.migrationService = app.MustComponent[dependencies.MigrationService](a)
	s.fileOffloader = app.MustComponent[dependencies.FileOffloader](a)
	s.delController = app.MustComponent[deletioncontroller.DeletionController](a)
	s.virtualSpaceService = app.MustComponent[virtualspaceservice.VirtualSpaceService](a)
	s.notificationService = app.MustComponent[NotificationSender](a)
	s.spaceNameGetter = app.MustComponent[objectstore.SpaceNameGetter](a)
	s.identityService = app.MustComponent[dependencies.IdentityService](a)
	s.inboxSender = app.MustComponent[inboxservice.Sender](a)
	s.aclJoiner = app.MustComponent[AclJoiner](a)
	s.parentApp = a.ChildApp()

	s.newAccount = s.config.IsNewAccount()
	s.preferredSpaceId = s.config.PreferredSpaceId
	s.autoJoinStreamSpace = s.config.AutoJoinStream
	s.techSpaceReady = make(chan struct{})
	s.preloadCh = make(chan struct{})
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())

	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeRegular)
	if err != nil {
		return fmt.Errorf("derive personal space id: %w", err)
	}
	s.techSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeTech)
	if err != nil {
		return fmt.Errorf("derive tech space id: %w", err)
	}
	s.repKey, err = getRepKey(s.personalSpaceId)
	if err != nil {
		return fmt.Errorf("derive replication key: %w", err)
	}

	accountMetadata, metadataSymKey, err := domain.DeriveAccountMetadata(s.accountService.Account().SignKey)
	if err != nil {
		return fmt.Errorf("derive account metadata: %w", err)
	}
	s.accountMetadataSymKey = metadataSymKey
	s.accountMetadataPayload, err = accountMetadata.Marshal()
	if err != nil {
		return fmt.Errorf("marshal account metadata: %w", err)
	}

	s.registry = newRegistry(s.newController)
	s.watcher = newSpaceWatcher(s.subService, s.techSpaceId, s.onSpaceViewEvent)
	return nil
}

func (s *service) Run(ctx context.Context) error {
	defer s.delController.UpdateCoordinatorStatus()
	if err := s.initMarketplace(); err != nil {
		return fmt.Errorf("init marketplace space: %w", err)
	}

	ts, oldAccount, err := resolveTechSpace(ctx, techSpaceResolution{
		newAccount: s.newAccount,
		create:     s.createTechSpace,
		load:       s.loadTechSpace,
		personalCoreReachable: func(ctx context.Context) error {
			if _, err := s.spaceCore.Get(ctx, s.personalSpaceId); err != nil {
				return fmt.Errorf("get personal core space: %w", err)
			}
			return nil
		},
		personalStorageExists: func(ctx context.Context) (bool, error) {
			return s.spaceCore.StorageExistsLocally(ctx, s.personalSpaceId)
		},
	})
	if err != nil {
		if s.newAccount {
			// a wrong stored network id makes every network op fail; reset so
			// the next attempt renegotiates (v1 createAccount behavior)
			if resetErr := s.config.ResetStoredNetworkId(); resetErr != nil {
				log.Error("reset stored network id", zap.Error(resetErr))
			}
		}
		return fmt.Errorf("resolve tech space: %w", err)
	}
	s.techSpace = ts
	s.parentApp.Register(ts)
	s.backendDeps = &BackendDeps{
		App:                    s.parentApp,
		TechSpace:              ts,
		SpaceCore:              s.spaceCore,
		AccountService:         s.accountService,
		ObjectFactory:          s.objectFactory,
		Storage:                s.storageService,
		Indexer:                s.indexer,
		Installer:              s.installer,
		MigrationService:       s.migrationService,
		FileOffloader:          s.fileOffloader,
		DeletionController:     s.delController,
		PersonalSpaceId:        s.personalSpaceId,
		AccountMetadataPayload: s.accountMetadataPayload,
	}
	close(s.techSpaceReady)

	if oldAccount {
		// the freshly created tech space has no view for the (pre-existing)
		// personal space yet; create it so the watcher loads the space
		if err = s.ensurePersonalSpaceView(ctx); err != nil {
			return fmt.Errorf("ensure personal space view: %w", err)
		}
	}
	s.lazyMode = s.computeLazyMode(ctx)

	if err = s.watcher.Run(); err != nil {
		return fmt.Errorf("run space watcher: %w", err)
	}

	if s.newAccount && s.autoJoinStreamSpace == "" {
		// a new account starts with one created space (not the derived
		// personal space); preserve v1's synchronous bootstrap contract —
		// Run returns with that space fully loaded
		firstSpace, createErr := s.create(ctx, nil)
		if createErr != nil {
			if errors.Is(createErr, spacesyncproto.ErrSpaceMissing) || errors.Is(createErr, treechangeproto.ErrGetTree) {
				createErr = ErrSpaceNotExists
			}
			// fix for users that have a wrong network id stored in the folder
			if resetErr := s.config.ResetStoredNetworkId(); resetErr != nil {
				log.Error("reset stored network id", zap.Error(resetErr))
			}
			return fmt.Errorf("create first space: %w", createErr)
		}
		s.firstCreatedSpaceId = firstSpace.Id()
	} else if s.autoJoinStreamSpace != "" {
		s.tryToJoinSpaceStream()
	}

	if s.lazyMode {
		go s.drainDeferredLater()
	}

	// deliberately after the watcher is running, so no status change is missed
	s.techSpace.StartSync()

	// persist the network id only after a fully successful space init — the
	// id is the network-mismatch guard and must not be written on failure
	if err = s.config.PersistAccountNetworkId(); err != nil {
		log.Error("persist account network id", zap.Error(err))
	}
	return nil
}

// computeLazyMode decides once whether non-preferred spaces are deferred at
// startup. A preferred space whose view is absent on this device degrades to
// eager (accepted: the existence check may do a bounded remote lookup).
func (s *service) computeLazyMode(ctx context.Context) bool {
	if s.preferredSpaceId == "" ||
		s.preferredSpaceId == s.techSpaceId ||
		s.preferredSpaceId == addr.AnytypeMarketplaceWorkspace {
		return false
	}
	exists, err := s.techSpace.SpaceViewExists(ctx, s.preferredSpaceId)
	return err == nil && exists
}

func (s *service) Close(ctx context.Context) error {
	s.ctxCancel()
	var errs []error
	if s.watcher != nil {
		s.watcher.Close()
	}
	if s.registry != nil {
		if err := s.registry.close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("close controllers: %w", err))
		}
	}
	if s.techSpace != nil {
		if err := s.techSpace.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("close tech space: %w", err))
		}
	}
	return errors.Join(errs...)
}

// newController is the registry factory: cheap and non-blocking.
func (s *service) newController(spaceId string) (*controller, error) {
	if s.backendDeps == nil {
		return nil, fmt.Errorf("tech space not resolved yet")
	}
	return newController(spaceId, newSpaceBackend(spaceId, s.backendDeps), controllerOptions{}), nil
}

// onSpaceViewEvent is the reactive spine: any SpaceView change ensures the
// space's controller exists, applies the discovery demand policy, and wakes
// the reconciler. Must stay non-blocking — it runs on the subscription
// goroutine.
func (s *service) onSpaceViewEvent(ev spaceViewEvent) {
	if ev.spaceId == "" || ev.spaceId == s.techSpaceId || ev.spaceId == addr.AnytypeMarketplaceWorkspace {
		return
	}
	if ev.remoteStatus == spaceinfo.RemoteStatusDeleted && ev.accountStatus != spaceinfo.AccountStatusDeleted {
		// the coordinator deleted the space but the account status has not
		// caught up: flip it (and notify if the space was in use); the write
		// re-enters this watcher and drives the offload
		go s.reactRemoteDeleted(ev)
		return
	}
	if s.lazyMode && ev.spaceId == s.preferredSpaceId && preferredBroken(ev) {
		// the preferred space cannot serve as the only loaded space: release
		// the deferred backlog
		s.triggerPreload()
	}
	ctrl, err := s.registry.getOrCreate(ev.spaceId)
	if err != nil {
		if !errors.Is(err, ErrClosed) {
			log.Warn("ensure controller on space view event", zap.String("spaceId", ev.spaceId), zap.Error(err))
		}
		return
	}
	if s.wantedOnDiscovery(ev.spaceId) {
		ctrl.SetWanted(true)
	}
	ctrl.Poke()
}

func preferredBroken(ev spaceViewEvent) bool {
	return ev.localStatus == spaceinfo.LocalStatusMissing ||
		ev.accountStatus == spaceinfo.AccountStatusRemoving ||
		ev.accountStatus == spaceinfo.AccountStatusDeleted ||
		ev.remoteStatus == spaceinfo.RemoteStatusDeleted
}

// reactRemoteDeleted publishes AccountStatusDeleted for a space the
// coordinator reports as deleted, sending the participant-remove notification
// when the space was actively used on this device.
func (s *service) reactRemoteDeleted(ev spaceViewEvent) {
	if ev.localStatus == spaceinfo.LocalStatusOk {
		s.sendParticipantRemoveNotification(ev.spaceId)
	}
	info := spaceinfo.NewSpacePersistentInfo(ev.spaceId)
	info.SetAccountStatus(spaceinfo.AccountStatusDeleted)
	if err := s.techSpace.SetPersistentInfo(s.ctx, info); err != nil {
		log.Warn("publish remote deletion", zap.String("spaceId", ev.spaceId), zap.Error(err))
	}
}

func (s *service) triggerPreload() {
	s.preloadOnce.Do(func() {
		close(s.preloadCh)
	})
}

// wantedOnDiscovery is the demand policy for spaces discovered via the
// watcher: eager mode loads everything; lazy mode (a preferred space is
// configured) loads only the preferred space until the deferred backlog is
// drained (preload RPC / safety timer), after which discovery is eager again.
// Get/Wait promote on demand.
func (s *service) wantedOnDiscovery(spaceId string) bool {
	if !s.lazyMode || s.lazyReleased.Load() {
		return true
	}
	return spaceId == s.preferredSpaceId
}

// drainDeferredLater promotes all deferred spaces once the preload trigger
// fires (RPC or safety timer). In v2 "deferred" is just wanted=false, so the
// drain is a SetWanted sweep — no cached statuses, no backlog bookkeeping.
func (s *service) drainDeferredLater() {
	select {
	case <-s.preloadCh:
	case <-time.After(preloadRemainingSpacesTimeout):
	case <-s.ctx.Done():
		return
	}
	// End lazy mode BEFORE sweeping: an event racing the sweep either sees
	// lazyReleased (demanded via wantedOnDiscovery) or its controller is
	// already registered and caught by the sweep — no discovery can slip
	// through undemanded. Without this flip, a space first discovered after
	// the drain would never background-load or sync (v1 collapsed to eager
	// after the first release via its `releasing` flag).
	s.lazyReleased.Store(true)
	for _, ctrl := range s.registry.all() {
		ctrl.SetWanted(true)
	}
}
