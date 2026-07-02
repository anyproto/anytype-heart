package spacev2

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/encode"
	"github.com/anyproto/anytype-heart/util/uri"
)

// The documented error set callers branch on (mirrors v1's, with the
// storage-missing typo fixed).
var (
	ErrSpaceNotExists      = errors.New("space not exists")
	ErrSpaceStorageMissing = errors.New("space storage missing")
	ErrFailedToLoad        = errors.New("failed to load space")
	// ErrSpaceIsClosing is what waiters get once the service shuts down.
	ErrSpaceIsClosing = ErrClosed
)

// convertSpaceError maps any-sync errors to the documented error set.
func convertSpaceError(err error) error {
	switch {
	case errors.Is(err, spacesyncproto.ErrSpaceIsDeleted):
		return ErrSpaceDeleted
	case errors.Is(err, spacecore.ErrSpaceIsDeleted):
		return ErrSpaceDeleted
	case errors.Is(err, spacestorage.ErrSpaceStorageMissing):
		return ErrSpaceStorageMissing
	default:
		return err
	}
}

// Get returns the loaded space. The space must be known (its controller
// exists once the SpaceView was discovered); a paused or deferred space is
// promoted and loaded on demand, so Get blocks until the load completes or
// fails.
func (s *service) Get(ctx context.Context, spaceId string) (clientspace.Space, error) {
	if spaceId == s.techSpaceId {
		return s.getTechSpace(ctx)
	}
	if spaceId == addr.AnytypeMarketplaceWorkspace {
		return s.marketplace.Get()
	}
	ctrl := s.registry.get(spaceId)
	if ctrl == nil {
		return nil, ErrSpaceNotExists
	}
	return s.waitLoaded(ctx, ctrl)
}

// Wait blocks until the space is loaded, tolerating a space whose controller
// does not exist yet: if the SpaceView exists (possibly after a bounded
// remote lookup), the controller is created directly. Use it when the space
// may not be known yet (object open, account bootstrap).
func (s *service) Wait(ctx context.Context, spaceId string) (clientspace.Space, error) {
	if spaceId == addr.AnytypeMarketplaceWorkspace {
		return s.marketplace.Get()
	}
	ts, err := s.getTechSpace(ctx)
	if err != nil {
		return nil, err
	}
	if spaceId == s.techSpaceId {
		return ts, nil
	}
	exists, err := ts.SpaceViewExists(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("check space view exists: %w", err)
	}
	if !exists {
		return nil, ErrSpaceNotExists
	}
	ctrl, err := s.registry.getOrCreate(spaceId)
	if err != nil {
		return nil, convertSpaceError(err)
	}
	return s.waitLoaded(ctx, ctrl)
}

func (s *service) waitLoaded(ctx context.Context, ctrl *controller) (clientspace.Space, error) {
	ctrl.SetWanted(true)
	sp, err := ctrl.WaitLoaded(ctx)
	if err != nil {
		return nil, convertSpaceError(err)
	}
	return sp, nil
}

// Create creates a new space: a network-registered core space with a random
// id, local storage marked created, a SpaceView, and returns it fully loaded.
func (s *service) Create(ctx context.Context, description *spaceinfo.SpaceDescription) (clientspace.Space, error) {
	if s.ctx.Err() != nil {
		return nil, ErrSpaceIsClosing
	}
	if description != nil && description.SpaceType == model.SpaceType_SpaceTypeOneToOne {
		return s.CreateOneToOneSendInbox(ctx, description)
	}
	return s.create(ctx, description)
}

func (s *service) create(ctx context.Context, description *spaceinfo.SpaceDescription) (clientspace.Space, error) {
	coreSpace, err := s.spaceCore.Create(ctx, spacedomain.SpaceTypeRegular, s.repKey, s.accountMetadataPayload)
	if err != nil {
		return nil, fmt.Errorf("create core space: %w", err)
	}
	info := spaceinfo.NewSpacePersistentInfo(coreSpace.Id())
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
	return s.materializeCreatedSpace(ctx, coreSpace, info, description)
}

// materializeCreatedSpace finishes any create flow: mark the storage created
// (triggers mandatory-object bootstrap on first build), create the SpaceView,
// and load through the single reconciler path.
func (s *service) materializeCreatedSpace(ctx context.Context, coreSpace *spacecore.AnySpace, info spaceinfo.SpacePersistentInfo, description *spaceinfo.SpaceDescription) (clientspace.Space, error) {
	if err := coreSpace.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx); err != nil {
		return nil, fmt.Errorf("mark space created: %w", err)
	}
	if err := s.techSpace.SpaceViewCreate(ctx, coreSpace.Id(), true, info, description); err != nil {
		return nil, fmt.Errorf("create space view: %w", err)
	}
	ctrl, err := s.registry.getOrCreate(coreSpace.Id())
	if err != nil {
		return nil, convertSpaceError(err)
	}
	sp, err := s.waitLoaded(ctx, ctrl)
	if err != nil {
		return nil, fmt.Errorf("load created space: %w", err)
	}
	s.delController.UpdateCoordinatorStatus()
	return sp, nil
}

// CreateOneToOneSendInbox creates a DM space with the given identity and
// schedules the inbox invite (initiator flow: avatar / QR code).
func (s *service) CreateOneToOneSendInbox(ctx context.Context, description *spaceinfo.SpaceDescription) (clientspace.Space, error) {
	if description.OneToOneIdentity == "" {
		return nil, fmt.Errorf("create one-to-one: OneToOneIdentity is missing")
	}
	bobProfile, err := s.identityService.WaitProfileWithKey(ctx, description.OneToOneIdentity)
	if err != nil {
		return nil, fmt.Errorf("wait profile of %s: %w", description.OneToOneIdentity, err)
	}
	description.Name = bobProfile.IdentityProfile.Name
	description.IconImage = bobProfile.IdentityProfile.IconCid
	description.OneToOneInboxSentStatus = spaceinfo.OneToOneInboxSentStatusToSend
	sp, err := s.CreateOneToOne(ctx, description, bobProfile)
	if err != nil {
		return nil, fmt.Errorf("create one-to-one: %w", err)
	}
	if err = s.inboxSender.ResendFailedOneToOneInvites(ctx); err != nil {
		log.Error("reschedule one-to-one inbox resend", zap.Error(err))
	}
	return sp, nil
}

// CreateOneToOne creates the deterministic DM space for the identity pair.
func (s *service) CreateOneToOne(ctx context.Context, description *spaceinfo.SpaceDescription, bobProfile *model.IdentityProfileWithKey) (clientspace.Space, error) {
	myIdentity := s.accountService.Account().SignKey.GetPublic().Account()
	if description.OneToOneIdentity == myIdentity {
		return nil, fmt.Errorf("can't create one-to-one chat with self")
	}
	bPk, err := crypto.DecodeAccountAddress(bobProfile.IdentityProfile.Identity)
	if err != nil {
		return nil, fmt.Errorf("decode identity: %w", err)
	}
	coreSpace, err := s.spaceCore.CreateOneToOneSpace(ctx, bPk)
	if err != nil {
		return nil, fmt.Errorf("create one-to-one core space: %w", err)
	}

	info := spaceinfo.NewSpacePersistentInfo(coreSpace.Id())
	info.OneToOneIdentity = bobProfile.IdentityProfile.Identity
	info.Name = description.Name
	info.OneToOneRequestMetadataKey = encodeRequestMetadataKey(bobProfile.RequestMetadata)
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)

	// A one-to-one space may have been created and locally removed before
	// (its id is deterministic in the identity pair): reset the stale view so
	// the space can come back instead of staying stuck in deleted state.
	spaceView, err := s.techSpace.GetSpaceView(ctx, coreSpace.Id())
	if err != nil && !errors.Is(err, techspace.ErrSpaceViewNotExists) {
		return nil, fmt.Errorf("get space view: %w", err)
	}
	if spaceView != nil {
		if err = s.reviveOneToOneView(spaceView, info); err != nil {
			return nil, err
		}
		if err = coreSpace.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx); err != nil {
			return nil, fmt.Errorf("mark space created: %w", err)
		}
		ctrl, ctrlErr := s.registry.getOrCreate(coreSpace.Id())
		if ctrlErr != nil {
			return nil, convertSpaceError(ctrlErr)
		}
		sp, loadErr := s.waitLoaded(ctx, ctrl)
		if loadErr != nil {
			return nil, fmt.Errorf("load one-to-one space: %w", loadErr)
		}
		s.delController.UpdateCoordinatorStatus()
		return sp, nil
	}
	return s.materializeCreatedSpace(ctx, coreSpace, info, description)
}

func (s *service) reviveOneToOneView(spaceView techspace.SpaceView, info spaceinfo.SpacePersistentInfo) error {
	spaceView.Lock()
	defer spaceView.Unlock()
	localInfo := spaceView.GetLocalInfo()
	if localInfo.GetLocalStatus() == spaceinfo.LocalStatusOk {
		return nil
	}
	reset := spaceinfo.NewSpaceLocalInfo(info.SpaceID)
	reset.SetLocalStatus(spaceinfo.LocalStatusUnknown).
		SetRemoteStatus(spaceinfo.RemoteStatusUnknown)
	if err := spaceView.SetSpaceLocalInfo(reset); err != nil {
		return fmt.Errorf("reset one-to-one local info: %w", err)
	}
	if err := spaceView.SetSpacePersistentInfo(info); err != nil {
		return fmt.Errorf("reset one-to-one persistent info: %w", err)
	}
	return nil
}

// Join records the intention to join a space: the SpaceView carries
// AccountStatusJoining + the invite ACL head, and the reconciler runs the
// join waiter. Returns once recorded; acceptance is asynchronous (v1
// semantics). Unlike v1, an existing view is always moved to Joining — the
// v1 fresh-controller path left a stale status in place.
func (s *service) Join(ctx context.Context, id, aclHeadId string) error {
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusJoining)
	return s.setStatusAndDemand(ctx, info)
}

// InviteJoin activates a space this account was directly added to: the view
// gets AccountStatusActive + the ACL head, and the space loads.
func (s *service) InviteJoin(ctx context.Context, id, aclHeadId string) error {
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAclHeadId(aclHeadId).SetAccountStatus(spaceinfo.AccountStatusActive)
	return s.setStatusAndDemand(ctx, info)
}

// setStatusAndDemand writes the persistent info (creating the view when
// missing), then makes sure the space's controller exists, is demanded, and
// re-decides. This is the unidirectional write path shared by Join/InviteJoin.
func (s *service) setStatusAndDemand(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	ts, err := s.getTechSpace(ctx)
	if err != nil {
		return err
	}
	if err = ts.SpaceViewCreate(ctx, info.SpaceID, false, info, nil); err != nil {
		if !errors.Is(err, techspace.ErrSpaceViewExists) {
			return fmt.Errorf("create space view: %w", err)
		}
		if err = ts.SetPersistentInfo(ctx, info); err != nil {
			return fmt.Errorf("set persistent info: %w", err)
		}
	}
	ctrl, err := s.registry.getOrCreate(info.SpaceID)
	if err != nil {
		return convertSpaceError(err)
	}
	ctrl.SetWanted(true)
	ctrl.Poke()
	return nil
}

// CancelLeave restores a space that was marked deleted on this account
// (offloaded or offloading): flipping the status back to Active makes the
// reconciler reload it.
func (s *service) CancelLeave(ctx context.Context, id string) error {
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusActive)
	return s.techSpace.SetPersistentInfo(ctx, info)
}

// Delete marks the space deleted for this account; the reconciler offloads
// local data and the deletion driver performs the network delete for owned
// spaces.
func (s *service) Delete(ctx context.Context, id string) error {
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusDeleted)
	return s.techSpace.SetPersistentInfo(ctx, info)
}

// AddStreamable registers a guest/stream space accessed with a private guest
// key and starts loading it. Idempotent: an existing view is reused.
func (s *service) AddStreamable(ctx context.Context, id string, guestKey crypto.PrivKey) error {
	encodedKey, err := crypto.EncodeKeyToString(guestKey)
	if err != nil {
		return fmt.Errorf("encode guest key: %w", err)
	}
	info := spaceinfo.NewSpacePersistentInfo(id)
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown).
		SetEncodedKey(encodedKey)
	ts, err := s.getTechSpace(ctx)
	if err != nil {
		return err
	}
	if err = ts.SpaceViewCreate(ctx, id, false, info, nil); err != nil && !errors.Is(err, techspace.ErrSpaceViewExists) {
		return fmt.Errorf("create streamable space view: %w", err)
	}
	ctrl, err := s.registry.getOrCreate(id)
	if err != nil {
		return convertSpaceError(err)
	}
	ctrl.SetWanted(true)
	return nil
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

func (s *service) IsPersonal(id string) bool {
	return s.personalSpaceId == id
}

// TechSpace returns the tech space without waiting for bootstrap; nil before
// Run resolved it (v1 contract).
func (s *service) TechSpace() *clientspace.TechSpace {
	return s.techSpace
}

func (s *service) GetPersonalSpace(ctx context.Context) (clientspace.Space, error) {
	return s.Get(ctx, s.personalSpaceId)
}

func (s *service) GetTechSpace(ctx context.Context) (clientspace.Space, error) {
	return s.getTechSpace(ctx)
}

// getTechSpace blocks on the bootstrap barrier so early callers never observe
// a nil tech space.
func (s *service) getTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	select {
	case <-s.techSpaceReady:
		return s.techSpace, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *service) SpaceViewId(spaceId string) (string, error) {
	return s.techSpace.SpaceViewId(spaceId)
}

func (s *service) AccountMetadataSymKey() crypto.SymKey {
	return s.accountMetadataSymKey
}

func (s *service) AccountMetadataPayload() []byte {
	return s.accountMetadataPayload
}

// PreloadRemainingSpaces releases spaces deferred by lazy mode. Idempotent.
func (s *service) PreloadRemainingSpaces(ctx context.Context) error {
	s.triggerPreload()
	return nil
}

// UpdateRemoteStatus publishes the coordinator's view of a space (deletion
// driver seam, deletioncontroller.SpaceManager).
func (s *service) UpdateRemoteStatus(ctx context.Context, status spaceinfo.SpaceRemoteStatusInfo) error {
	return s.techSpace.DoSpaceView(ctx, status.LocalInfo.SpaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.SetSpaceLocalInfo(status.LocalInfo)
	})
}

// UpdateSharedLimits publishes the account's shared-spaces limit
// (deletioncontroller.SpaceManager).
func (s *service) UpdateSharedLimits(ctx context.Context, limits int) error {
	return s.techSpace.DoAccountObject(ctx, func(accountObject techspace.AccountObject) error {
		return accountObject.SetSharedSpacesLimit(limits)
	})
}

// AllSpaceIds lists every tracked real space (deletioncontroller.SpaceManager).
func (s *service) AllSpaceIds() (ids []string) {
	ctrls := s.registry.all()
	ids = make([]string, 0, len(ctrls))
	for _, ctrl := range ctrls {
		ids = append(ids, ctrl.SpaceId())
	}
	return ids
}

// AllLoadedSpaceIds lists the spaces currently resident in memory.
func (s *service) AllLoadedSpaceIds() (ids []string) {
	for _, ctrl := range s.registry.all() {
		if ctrl.State() == StateLoaded {
			ids = append(ids, ctrl.SpaceId())
		}
	}
	return ids
}

// syncAllHeadsParallelism bounds how many spaces head-sync at once on a
// foreground resume.
const syncAllHeadsParallelism = 10

// SyncAllSpaceHeads triggers an immediate diff round for every loaded space
// (app-foreground refresh, GO-7302). Non-blocking.
func (s *service) SyncAllSpaceHeads() {
	if s.ctx.Err() != nil {
		return
	}
	ids := s.AllLoadedSpaceIds()
	go func() {
		sem := make(chan struct{}, syncAllHeadsParallelism)
		var wg sync.WaitGroup
		for _, id := range ids {
			sem <- struct{}{}
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				defer func() { <-sem }()
				ctrl := s.registry.get(id)
				if ctrl == nil {
					return
				}
				sp := ctrl.SpaceIfLoaded()
				if sp == nil {
					return
				}
				if err := sp.CommonSpace().SyncHeads(s.ctx); err != nil && s.ctx.Err() == nil {
					log.Warn("sync space heads", zap.String("spaceId", id), zap.Error(err))
				}
			}(id)
		}
		wg.Wait()
	}()
}

// OnWorkspaceChanged mirrors workspace details into the SpaceView (the
// space-switcher UI reads them cross-space from the tech space).
func (s *service) OnWorkspaceChanged(spaceId string, details *domain.Details) {
	if err := s.techSpace.SpaceViewSetData(s.ctx, spaceId, details); err != nil {
		log.Warn("mirror workspace details to space view", zap.String("spaceId", spaceId), zap.Error(err))
	}
}

func (s *service) SpaceViewSetOneToOneIdentity(spaceId string, identity string) {
	go func() {
		if err := s.techSpace.SpaceViewSetOneToOneIdentity(s.ctx, spaceId, identity); err != nil {
			log.Warn("set one-to-one identity on space view", zap.String("spaceId", spaceId), zap.Error(err))
		}
	}()
}

// sendParticipantRemoveNotification informs the user this account was removed
// from (or lost) a space; the deterministic id coalesces duplicates.
func (s *service) sendParticipantRemoveNotification(spaceId string) {
	identity := s.accountService.Account().SignKey.GetPublic().Account()
	err := s.notificationService.CreateAndSend(&model.Notification{
		Id: strings.Join([]string{spaceId, identity}, "_"),
		Payload: &model.NotificationPayloadOfParticipantRemove{
			ParticipantRemove: &model.NotificationParticipantRemove{
				SpaceId:   spaceId,
				SpaceName: s.spaceNameGetter.GetSpaceName(spaceId),
			},
		},
		Space: spaceId,
	})
	if err != nil {
		log.Error("send participant-remove notification", zap.String("spaceId", spaceId), zap.Error(err))
	}
}

// tryToJoinSpaceStream joins the configured auto-join stream space in the
// background, retrying with exponential backoff until the service closes.
func (s *service) tryToJoinSpaceStream() {
	if s.autoJoinStreamSpace == "" {
		return
	}
	go func() {
		delay := time.Second
		for {
			err := s.joinSpaceStream(s.ctx, s.autoJoinStreamSpace)
			if err == nil || s.ctx.Err() != nil {
				return
			}
			log.Warn("join stream space", zap.Error(err))
			select {
			case <-time.After(delay):
				delay *= 2
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *service) joinSpaceStream(ctx context.Context, inviteUrl string) error {
	inviteId, inviteKey, spaceId, networkId, err := uri.ParseInviteUrl(inviteUrl)
	if err != nil {
		return fmt.Errorf("parse invite url: %w", err)
	}
	if spaceId == "" {
		return fmt.Errorf("invite url has no space id")
	}
	inviteCid, err := cid.Parse(inviteId)
	if err != nil {
		return fmt.Errorf("parse invite cid: %w", err)
	}
	inviteSymKey, err := encode.DecodeKeyFromBase58(inviteKey)
	if err != nil {
		return fmt.Errorf("decode invite key: %w", err)
	}
	ts, err := s.getTechSpace(ctx)
	if err != nil {
		return fmt.Errorf("get tech space: %w", err)
	}
	exists, err := ts.SpaceViewExists(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("check space view exists: %w", err)
	}
	if exists {
		// already joined (or removed) — nothing to do
		return nil
	}
	return s.aclJoiner.Join(ctx, spaceId, networkId, inviteCid, inviteSymKey)
}

func encodeRequestMetadataKey(requestMetadata []byte) string {
	return base64.StdEncoding.EncodeToString(requestMetadata)
}

func getRepKey(spaceId string) (uint64, error) {
	sepIdx := strings.Index(spaceId, ".")
	if sepIdx == -1 {
		return 0, fmt.Errorf("space id has no replication key part")
	}
	return strconv.ParseUint(spaceId[sepIdx+1:], 36, 64)
}
