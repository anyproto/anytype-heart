package space

/*
AI generated

Name: Space Lifecycle Manager
Scope: global

## Responsibility
- Manages lifecycle of all space types (personal, shareable, streamable, one-to-one)
- Orchestrates space creation, loading, joining, and deletion
- Maintains registry of active space controllers
- Handles account initialization (new vs existing accounts, tech space creation)
- Triggers an on-demand head-sync round across all spaces (SyncAllSpaceHeads, used on app foreground)

## Background Tasks
- spaceWatcher: subscribes to space view changes in tech space, triggers controller updates (onSpaceStatusUpdated)
- tryToJoinSpaceStream: retries joining stream space with exponential backoff when autoJoinStreamSpace is configured
- SyncAllSpaceHeads: spawns a bounded-parallelism goroutine batch that runs a head-sync diff round for each actively loaded space

## Documentation
Space startup flow:
1. Init: derive personal/tech space IDs, setup watcher
2. Run: init marketplace space, then either createAccount (new) or initAccount (existing)
3. Tech space must be ready (channel closed) before regular spaces can be loaded
4. SpaceView changes in tech space trigger controller starts/updates via watcher subscription
*/

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/inbox/inboxservice"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclobjectmanager"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spacefactory"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/encode"
	"github.com/anyproto/anytype-heart/util/uri"
)

const CName = "client.space"

var log = logger.NewNamed(CName)

var (
	loadTechSpaceDeadline = 15 * time.Second
	// preloadRemainingSpacesTimeout bounds how long lazy mode defers loading
	// the non-preferred spaces. Var (not const) so tests can override it.
	preloadRemainingSpacesTimeout = 10 * time.Second
)

var (
	ErrIncorrectSpaceID   = errors.New("incorrect space id")
	ErrSpaceNotExists     = errors.New("space not exists")
	ErrSpaceStorageMissig = errors.New("space storage missing")
	ErrSpaceDeleted       = errors.New("space is deleted")
	ErrSpaceIsClosing     = errors.New("space is closing")
	ErrFailedToLoad       = errors.New("failed to load space")
)

func New() Service {
	return &service{}
}

type Service interface {
	Create(ctx context.Context, description *spaceinfo.SpaceDescription) (space clientspace.Space, err error)
	CreateOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription, bobProfile *model.IdentityProfileWithKey) (sp clientspace.Space, err error)
	Join(ctx context.Context, id, aclHeadId string) error
	InviteJoin(ctx context.Context, id, aclHeadId string) error
	CancelLeave(ctx context.Context, id string) (err error)
	Get(ctx context.Context, id string) (space clientspace.Space, err error)
	Wait(ctx context.Context, spaceId string) (sp clientspace.Space, err error)
	AddStreamable(ctx context.Context, id string, guestKey crypto.PrivKey) (err error)
	Delete(ctx context.Context, id string) (err error)
	TechSpaceId() string
	PersonalSpaceId() string
	FirstCreatedSpaceId() string
	TechSpace() *clientspace.TechSpace
	GetPersonalSpace(ctx context.Context) (space clientspace.Space, err error)
	GetTechSpace(ctx context.Context) (space clientspace.Space, err error)
	SpaceViewId(spaceId string) (spaceViewId string, err error)
	AccountMetadataSymKey() crypto.SymKey
	AccountMetadataPayload() []byte
	PreloadRemainingSpaces(ctx context.Context) error
	app.ComponentRunnable
}

type coordinatorStatusUpdater interface {
	app.Component
	UpdateCoordinatorStatus()
}

type NotificationSender interface {
	app.Component
	CreateAndSend(notification *model.Notification) error
}

type AclJoiner interface {
	Join(ctx context.Context, spaceId, networkId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error
}

type service struct {
	techSpace           *clientspace.TechSpace
	techSpaceReady      chan struct{}
	factory             spacefactory.SpaceFactory
	spaceCore           spacecore.SpaceCoreService
	aclJoiner           AclJoiner
	accountService      accountservice.Service
	inboxSender         inboxservice.Sender
	identityService     dependencies.IdentityService
	config              *config.Config
	notificationService NotificationSender
	updater             coordinatorStatusUpdater
	spaceNameGetter     objectstore.SpaceNameGetter

	personalSpaceId     string
	techSpaceId         string
	newAccount          bool
	autoJoinStreamSpace string
	spaceControllers    map[string]spacecontroller.SpaceController
	// regChanged is closed and replaced (under s.mu) whenever the registry
	// changes; waitCtrl blocks on it instead of polling.
	regChanged chan struct{}
	// regErr holds the last registration error per space id; cleared when the
	// next registration attempt starts, so failures are retryable.
	regErr           map[string]error
	preferredSpaceId string // from config; "" => eager (load all at startup)
	// lazyMode is set exactly once in initAccount before watcher.Run() and is
	// never mutated afterwards, so it is safe to read without s.mu (the
	// happens-before from watcher.Run() / goroutine spawn covers all readers).
	lazyMode bool
	// released is true once the lazy backlog has been released (RPC | timer |
	// preferred-space fallback); registrations after that demand immediately.
	// Guarded by s.mu.
	released               bool
	preloadOnce            sync.Once     // single release trigger
	preloadCh              chan struct{} // closed by triggerRelease()
	accountMetadataSymKey  crypto.SymKey
	accountMetadataPayload []byte
	repKey                 uint64
	spaceLoaderListener    aclobjectmanager.SpaceLoaderListener
	watcher                *spaceWatcher

	mu        sync.Mutex
	ctx       context.Context // use ctx for the long operations within the lifecycle of the service, excluding Run
	ctxCancel context.CancelFunc
	isClosing atomic.Bool

	firstCreatedSpaceId string
}

func (s *service) Delete(ctx context.Context, id string) (err error) {
	return s.TechSpace().DoSpaceView(ctx, id, func(spaceView techspace.SpaceView) error {
		info := spaceinfo.NewSpacePersistentInfo(id)
		info.SetAccountStatus(spaceinfo.AccountStatusDeleted)
		return spaceView.SetSpacePersistentInfo(info)
	})
}

func (s *service) TechSpace() *clientspace.TechSpace {
	return s.techSpace
}

func (s *service) Init(a *app.App) (err error) {
	s.factory = app.MustComponent[spacefactory.SpaceFactory](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.config = app.MustComponent[*config.Config](a)
	s.aclJoiner = app.MustComponent[AclJoiner](a)
	s.newAccount = s.config.IsNewAccount()
	s.autoJoinStreamSpace = s.config.AutoJoinStream
	s.spaceControllers = make(map[string]spacecontroller.SpaceController)
	s.updater = app.MustComponent[coordinatorStatusUpdater](a)
	subService := app.MustComponent[subscription.Service](a)
	s.notificationService = app.MustComponent[NotificationSender](a)
	s.spaceNameGetter = app.MustComponent[objectstore.SpaceNameGetter](a)
	s.spaceLoaderListener = app.MustComponent[aclobjectmanager.SpaceLoaderListener](a)
	s.identityService = app.MustComponent[dependencies.IdentityService](a)
	s.inboxSender = app.MustComponent[inboxservice.Sender](a)
	s.regChanged = make(chan struct{})
	s.regErr = make(map[string]error)
	s.preferredSpaceId = s.config.PreferredSpaceId
	s.preloadCh = make(chan struct{})
	s.techSpaceReady = make(chan struct{})
	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeRegular)
	if err != nil {
		return
	}
	s.techSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacedomain.SpaceTypeTech)
	if err != nil {
		return
	}
	accountMetadata, metadataSymKey, err := domain.DeriveAccountMetadata(s.accountService.Account().SignKey)
	if err != nil {
		return
	}
	s.accountMetadataSymKey = metadataSymKey
	s.accountMetadataPayload, err = accountMetadata.Marshal()
	if err != nil {
		return fmt.Errorf("marshal account metadata: %w", err)
	}

	s.repKey, err = getRepKey(s.personalSpaceId)
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.watcher = newSpaceWatcher(s.techSpaceId, subService, s)

	return err
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	defer s.updater.UpdateCoordinatorStatus()
	if s.newAccount {
		return s.createAccount(ctx)
	} else {
		s.tryToJoinSpaceStream()
	}
	return s.initAccount(ctx)
}

func (s *service) createTechSpaceForOldAccounts(ctx context.Context) (err error) {
	// check if we have a personal space
	_, err = s.spaceCore.Get(ctx, s.personalSpaceId)
	if err != nil {
		// then we don't have a personal space, so we have nothing, sorry, there is no point in creating tech space
		return fmt.Errorf("init tech space: %w", err)
	}
	// this is an old account
	err = s.createTechSpace(ctx)
	if err != nil {
		return fmt.Errorf("init tech space: %w", err)
	}
	// skipping check for space view because we don't have it
	ctx = context.WithValue(ctx, spacefactory.SkipCheckSpaceViewKey, true)
	_, err = s.startStatus(ctx, spaceinfo.NewSpacePersistentInfo(s.personalSpaceId))
	if err != nil {
		return fmt.Errorf("start personal space: %w", err)
	}
	return nil
}

func (s *service) initAccount(ctx context.Context) (err error) {
	err = s.initMarketplaceSpace(ctx)
	if err != nil {
		return fmt.Errorf("init marketplace space: %w", err)
	}
	s.spaceLoaderListener.OnSpaceLoad(addr.AnytypeMarketplaceWorkspace)
	timeoutCtx, cancel := context.WithTimeout(ctx, loadTechSpaceDeadline)
	err = s.loadTechSpace(timeoutCtx)
	cancel()
	// this crazy logic is needed if the person is restoring the old account locally with no connection and no tech space
	// nolint:nestif
	if errors.Is(err, context.DeadlineExceeded) {
		var personalExists bool
		// checking if personal space exists locally
		personalExists, err = s.spaceCore.StorageExistsLocally(ctx, s.personalSpaceId)
		if err != nil {
			return fmt.Errorf("check personal space: %w", err)
		}
		// ok no space locally, then we have to get a reply from server to know if we have a tech space
		if !personalExists {
			// trying again to load space with normal ctx
			err = s.loadTechSpace(ctx)
		} else {
			// personal exists, then we have to create tech space
			err = s.createTechSpaceForOldAccounts(ctx)
			if err != nil {
				return fmt.Errorf("create tech space for old accounts: %w", err)
			}
		}
	}
	if err != nil && !errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		return fmt.Errorf("init tech space: %w", err)
	}
	// nolint:nestif
	if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		// no tech space on nodes, this is our only chance
		err = s.createTechSpaceForOldAccounts(ctx)
		if err != nil {
			return fmt.Errorf("create tech space for old accounts: %w", err)
		}
	}
	s.lazyMode = s.computeLazyMode(ctx, s.techSpace)
	err = s.watcher.Run()
	if err != nil {
		return fmt.Errorf("run watcher: %w", err)
	}
	if s.lazyMode {
		go func() {
			select {
			case <-s.preloadCh:
			case <-time.After(preloadRemainingSpacesTimeout):
			case <-s.ctx.Done():
				return
			}
			s.releaseAll()
		}()
	}
	s.techSpace.StartSync()
	// only persist networkId after successful space init
	err = s.config.PersistAccountNetworkId()
	if err != nil {
		log.Error("persist network id to config", zap.Error(err))
	}
	return nil
}

func (s *service) createAccount(ctx context.Context) (err error) {
	err = s.initMarketplaceSpace(ctx)
	if err != nil {
		return fmt.Errorf("init marketplace space: %w", err)
	}
	err = s.createTechSpace(ctx)
	if err != nil {
		return fmt.Errorf("init tech space: %w", err)
	}
	// the watcher must run before the first space is created: controllers are
	// registered exclusively through space view events
	err = s.watcher.Run()
	if err != nil {
		return fmt.Errorf("run watcher: %w", err)
	}
	if s.autoJoinStreamSpace == "" {
		firstSpace, err := s.create(ctx, nil)
		if err != nil {
			if errors.Is(err, spacesyncproto.ErrSpaceMissing) || errors.Is(err, treechangeproto.ErrGetTree) {
				err = ErrSpaceNotExists
			}
			// fix for the users that have wrong network id stored in the folder
			err2 := s.config.ResetStoredNetworkId()
			if err2 != nil {
				log.Error("reset network id", zap.Error(err2))
			}
			return fmt.Errorf("init personal space: %w", err)
		}

		s.firstCreatedSpaceId = firstSpace.Id()
	} else {
		s.tryToJoinSpaceStream()
	}

	s.techSpace.StartSync()
	// only persist networkId after successful space init
	err = s.config.PersistAccountNetworkId()
	if err != nil {
		log.Error("persist network id to config", zap.Error(err))
	}
	return nil
}

func (s *service) Create(ctx context.Context, description *spaceinfo.SpaceDescription) (clientspace.Space, error) {
	if s.isClosing.Load() {
		return nil, ErrSpaceIsClosing
	}

	if description.SpaceType == model.SpaceType_SpaceTypeOneToOne {
		return s.CreateOneToOneSendInbox(ctx, description)
	} else {
		return s.create(ctx, description)
	}

}

// Wait returns the space once it is loaded. Since controller registration is
// event-driven (getCtrl waits on the registry), this is the same as Get.
func (s *service) Wait(ctx context.Context, spaceId string) (sp clientspace.Space, err error) {
	return s.Get(ctx, spaceId)
}

func (s *service) Get(ctx context.Context, spaceId string) (sp clientspace.Space, err error) {
	if spaceId == s.techSpaceId {
		return s.getTechSpace(ctx)
	}
	ctrl, err := s.getCtrl(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, ctrl)
}

func (s *service) UpdateSharedLimits(ctx context.Context, limits int) error {
	return s.techSpace.DoAccountObject(ctx, func(accObj techspace.AccountObject) error {
		return accObj.SetSharedSpacesLimit(limits)
	})
}

func (s *service) GetPersonalSpace(ctx context.Context) (sp clientspace.Space, err error) {
	return s.Get(ctx, s.personalSpaceId)
}

func (s *service) GetTechSpace(ctx context.Context) (sp clientspace.Space, err error) {
	return s.Get(ctx, s.techSpaceId)
}

func (s *service) IsPersonal(id string) bool {
	return s.personalSpaceId == id
}

func (s *service) onSpaceStatusUpdated(spaceStatus spaceViewStatus) {
	if s.isClosing.Load() {
		return
	}
	go func() {
		// we want the updates for each space view to be synchronous
		spaceStatus.mx.Lock()
		defer spaceStatus.mx.Unlock()
		if spaceStatus.remoteStatus == spaceinfo.RemoteStatusDeleted && spaceStatus.accountStatus != spaceinfo.AccountStatusDeleted {
			if spaceStatus.localStatus == spaceinfo.LocalStatusOk {
				s.sendNotification(spaceStatus.spaceId)
			}
			err := s.techSpace.DoSpaceView(context.Background(), spaceStatus.spaceId, func(spaceView techspace.SpaceView) error {
				info := spaceinfo.NewSpacePersistentInfo(spaceStatus.spaceId)
				info.SetAccountStatus(spaceinfo.AccountStatusDeleted)
				return spaceView.SetSpacePersistentInfo(info)
			})
			if err != nil {
				log.Warn("failed to update space view", zap.Error(err))
			}
			return
		}
		s.maybeReleaseOnPreferredBroken(spaceStatus)
		s.applySpaceStatus(spaceStatus)
	}()
}

// maybeReleaseOnPreferredBroken implements the B3-accepted dynamic fallback:
// if the preferred space itself is Missing/Removing/Deleted/RemoteDeleted,
// collapse lazy mode (release the backlog). Idempotent.
func (s *service) maybeReleaseOnPreferredBroken(spaceStatus spaceViewStatus) {
	if !s.lazyMode || spaceStatus.spaceId != s.preferredSpaceId {
		return
	}
	if spaceStatus.localStatus == spaceinfo.LocalStatusMissing ||
		spaceStatus.accountStatus == spaceinfo.AccountStatusRemoving ||
		spaceStatus.accountStatus == spaceinfo.AccountStatusDeleted ||
		spaceStatus.remoteStatus == spaceinfo.RemoteStatusDeleted {
		s.triggerRelease()
	}
}

// computeLazyMode decides, once, whether to defer non-preferred spaces.
// B1 (accepted): if the preferred space's view is not on this device,
// techSpace.SpaceViewExists may do a remote lookup (<=15s) before returning
// not-exists; that degrades to eager and is accepted for V1.
func (s *service) computeLazyMode(ctx context.Context, techSpace *clientspace.TechSpace) bool {
	if s.preferredSpaceId == "" ||
		s.preferredSpaceId == s.techSpaceId ||
		s.preferredSpaceId == addr.AnytypeMarketplaceWorkspace {
		return false
	}
	if techSpace == nil {
		return false
	}
	exists, err := techSpace.SpaceViewExists(ctx, s.preferredSpaceId)
	return err == nil && exists
}

// applySpaceStatus creates (idempotently) the space controller for the given
// status and pushes the persistent info into it. In lazy mode non-preferred
// controllers register dormant: they offload/join according to their status
// but do not load until demanded (Get/Wait/preload release).
func (s *service) applySpaceStatus(spaceStatus spaceViewStatus) {
	if s.isClosing.Load() {
		return
	}
	info := statusToInfo(spaceStatus)
	ctrl, err := s.startStatus(s.ctx, info)
	if err != nil && !errors.Is(err, ErrSpaceDeleted) {
		log.Warn("startStatus error", zap.Error(err))
		return
	}
	// startStatus returns (nil, err) on any factory error, so guard against a
	// nil ctrl when err is ErrSpaceDeleted.
	if err != nil {
		return
	}
	if err = ctrl.Update(); err != nil {
		log.Warn("ctrl.Update error", zap.Error(err))
		return
	}
}

// triggerRelease is the single idempotent "release the lazy backlog" signal,
// shared by the AccountPreloadRemainingSpaces RPC, the safety timer, and the
// preferred-space dynamic fallback.
func (s *service) triggerRelease() {
	s.preloadOnce.Do(func() {
		close(s.preloadCh)
	})
}

// PreloadRemainingSpaces releases spaces deferred by lazy mode. Idempotent and
// safe to call at any time.
func (s *service) PreloadRemainingSpaces(ctx context.Context) error {
	s.triggerRelease()
	return nil
}

// releaseAll demands every registered space. Registrations that race with the
// release are covered by startStatus: it re-checks released after inserting
// the controller, so a controller is either in the snapshot taken here or
// demands itself.
func (s *service) releaseAll() {
	if s.isClosing.Load() {
		return
	}
	s.mu.Lock()
	s.released = true
	ctrls := make([]spacecontroller.SpaceController, 0, len(s.spaceControllers))
	for _, ctrl := range s.spaceControllers {
		ctrls = append(ctrls, ctrl)
	}
	s.mu.Unlock()
	for _, ctrl := range ctrls {
		ctrl.Demand()
	}
}

func (s *service) SpaceViewSetOneToOneIdentity(spaceId string, identity string) {
	go func() {
		if err := s.techSpace.SpaceViewSetOneToOneIdentity(s.ctx, spaceId, identity); err != nil {
			log.Warn("SpaceViewSetOneToOneIdentity error", zap.Error(err))
		}
	}()
}

func (s *service) OnWorkspaceChanged(spaceId string, details *domain.Details) {
	if err := s.techSpace.SpaceViewSetData(s.ctx, spaceId, details); err != nil {
		log.Warn("OnWorkspaceChanged error", zap.Error(err))
	}
}

func (s *service) AccountMetadataSymKey() crypto.SymKey {
	return s.accountMetadataSymKey
}

func (s *service) AccountMetadataPayload() []byte {
	return s.accountMetadataPayload
}

func (s *service) UpdateRemoteStatus(ctx context.Context, status spaceinfo.SpaceRemoteStatusInfo) error {
	return s.techSpace.DoSpaceView(ctx, status.LocalInfo.SpaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.SetSpaceLocalInfo(status.LocalInfo)
	})
}

func (s *service) sendNotification(spaceId string) {
	identity := s.accountService.Account().SignKey.GetPublic().Account()
	notificationId := strings.Join([]string{spaceId, identity}, "_")
	spaceName := s.spaceNameGetter.GetSpaceName(spaceId)
	err := s.notificationService.CreateAndSend(&model.Notification{
		Id: notificationId,
		Payload: &model.NotificationPayloadOfParticipantRemove{
			ParticipantRemove: &model.NotificationParticipantRemove{
				SpaceId:   spaceId,
				SpaceName: spaceName,
			},
		},
		Space: spaceId,
	})
	if err != nil {
		log.Error("failed to send notification", zap.Error(err), zap.String("spaceId", spaceId))
	}
}

func (s *service) SpaceViewId(spaceId string) (spaceViewId string, err error) {
	return s.techSpace.SpaceViewId(spaceId)
}

func (s *service) Close(ctx context.Context) error {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	s.isClosing.Store(true)
	s.mu.Lock()
	ctrls := make([]spacecontroller.SpaceController, 0, len(s.spaceControllers))
	for _, ctrl := range s.spaceControllers {
		ctrls = append(ctrls, ctrl)
	}
	s.mu.Unlock()

	wg := sync.WaitGroup{}
	for _, ctrl := range ctrls {
		wg.Add(1)
		go func(ctrl spacecontroller.SpaceController) {
			defer wg.Done()
			err := ctrl.Close(ctx)
			if err != nil {
				log.Error("close space", zap.String("spaceId", ctrl.SpaceId()), zap.Error(err))
			}
		}(ctrl)
	}
	wg.Wait()
	err := s.techSpace.Close(ctx)
	if err != nil {
		log.Error("close tech space", zap.Error(err))
	}
	return s.watcher.Close()
}

func (s *service) AllSpaceIds() (ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.spaceControllers {
		if id == addr.AnytypeMarketplaceWorkspace {
			continue
		}
		ids = append(ids, id)
	}
	return
}

// AllLoadedSpaceIds returns IDs of spaces that are fully loaded (in ModeLoading state).
// Excludes marketplace space and spaces that are being offloaded, deleted, or joining.
func (s *service) AllLoadedSpaceIds() (ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.spaceControllers {
		if c.Mode() != mode.ModeLoading {
			continue
		}
		if id == addr.AnytypeMarketplaceWorkspace {
			continue
		}
		ids = append(ids, id)
	}
	return
}

// syncAllHeadsParallelism bounds how many spaces are head-synced at once so a
// foreground resume doesn't hit every space simultaneously.
const syncAllHeadsParallelism = 10

// SyncAllSpaceHeads triggers an immediate head-sync (diff) round for every
// actively loaded space. Used on app foreground (GO-7302) to refresh all spaces
// promptly after a wakeup instead of waiting for each space's next periodic
// diffsync tick. Runs in the background and does not block the caller; only
// loaded spaces (AllLoadedSpaceIds) can serve a head-sync round — skipping
// offloaded/deleted/initializing ones avoids a warning per inactive space.
func (s *service) SyncAllSpaceHeads() {
	if s.isClosing.Load() {
		return
	}
	ids := s.AllLoadedSpaceIds()
	go func() {
		sem := make(chan struct{}, syncAllHeadsParallelism)
		var wg sync.WaitGroup
		for _, id := range ids {
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				sp, err := s.Get(s.ctx, id)
				if err != nil {
					if s.ctx.Err() == nil {
						log.Warn("sync all space heads: get space", zap.String("spaceId", id), zap.Error(err))
					}
					return
				}
				if err := sp.CommonSpace().SyncHeads(s.ctx); err != nil && s.ctx.Err() == nil {
					log.Warn("sync all space heads: sync heads", zap.String("spaceId", id), zap.Error(err))
				}
			}()
		}
		wg.Wait()
	}()
}

func (s *service) TechSpaceId() string {
	return s.techSpaceId
}

func (s *service) PersonalSpaceId() string {
	return s.personalSpaceId
}

func (s *service) FirstCreatedSpaceId() string {
	return s.firstCreatedSpaceId
}

func (s *service) getTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	select {
	case <-s.techSpaceReady:
		return s.techSpace, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// tryToJoinSpaceStream tries to join space stream if autoJoinStreamSpace is set
// it runs in a goroutine and retries with increasing delay
// stops when service is closed
func (s *service) tryToJoinSpaceStream() {
	if s.autoJoinStreamSpace == "" {
		return
	}
	// retry with increasing delay
	go func() {
		delay := time.Second
		for {
			err := joinSpaceStream(s.ctx, s, s.aclJoiner, s.autoJoinStreamSpace)
			if err == nil {
				return
			}
			if s.ctx.Err() != nil {
				return
			}
			log.Warn("failed to join stream", zap.Error(err))
			select {
			case <-time.After(delay):
				delay *= 2
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func joinSpaceStream(ctx context.Context, spaceService *service, aclJoiner AclJoiner, inviteUrl string) error {
	if inviteUrl == "" {
		return nil
	}

	if aclJoiner == nil {
		return fmt.Errorf("aclJoiner is nil")
	}

	inviteId, inviteKey, spaceId, networkId, err := uri.ParseInviteUrl(inviteUrl)
	if err != nil {
		return err
	}
	if spaceId == "" {
		return fmt.Errorf("spaceId is empty")
	}
	inviteCid, err := cid.Parse(inviteId)
	if err != nil {
		return err
	}
	inviteSymKey, err := encode.DecodeKeyFromBase58(inviteKey)
	if err != nil {
		return err
	}

	techSpace, err := spaceService.getTechSpace(ctx)
	if err != nil {
		return fmt.Errorf("get tech space: %w", err)
	}

	if exists, err := techSpace.SpaceViewExists(ctx, spaceId); err != nil {
		return err
	} else if exists {
		// do not try to join stream if space already joined or removed
		return nil
	}

	return aclJoiner.Join(ctx, spaceId, networkId, inviteCid, inviteSymKey)
}
