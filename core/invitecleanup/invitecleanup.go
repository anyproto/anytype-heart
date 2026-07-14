package invitecleanup

/*
Name: Revoked Invite Cleanup
Scope: global

## Responsibility
- Removes the invite files of revoked invites from the file node, so that an old invite link no
  longer resolves to anything.

## Documentation
CleanupSpace is the background pass, driven per space by space/internal/components/invitecleaner
20-80s after the space loads. InviteRevoked is called by core/acl once a revocation has gone through,
and removes that invite's file straight away rather than leaving it for the next pass.

A space is certified clean by writing the id of the last invite revocation the cleanup covered to
inviteCleanupDone on its spaceview. The certificate syncs across the account's devices, so only one
of them does the work — and because revoking an invite appends a record to the acl and so moves the
id, a revocation voids the certificate by itself, everywhere, with nothing to withdraw and nothing to
race. Generating an invite does not move it: a new invite leaves nothing behind to delete.

The acl cannot enumerate revoked invites: AclState().Invites() only reports live ones, and an
invite's acl id is a record id with no relation to its file cid. Candidates therefore come from the
change history of the workspace object, which recorded every invite the space ever had.

Deleting an invite file cannot be undone, and an invite that is still in use must keep its file.
decide() establishes that an invite was revoked before its file is removed.
*/

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/invitestore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/encode"
)

const CName = "core.invitecleanup"

var log = logger.NewNamed(CName)

// retryDelays is the backoff between attempts of a step that failed with a transient error.
var retryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}

// eagerDeleteTimeout bounds the deletion that follows a revocation. It runs detached from the RPC
// that triggered it, and whatever it fails to finish is left to the background pass.
var eagerDeleteTimeout = time.Minute

// concurrentPasses caps how many spaces are scanned at once. An account can have hundreds, they all
// load at once, and a scan is not cheap: a coordinator round trip, a replay of the workspace object's
// whole history, and a fetch per invite file. The jitter in the cleaner spreads their start; this
// bounds what happens when they nevertheless overlap. Anything held back simply waits — no space is
// skipped.
const concurrentPasses = 4

type Service interface {
	app.ComponentRunnable
	// CleanupSpace deletes the invite files of every revoked invite of sp, unless the space has
	// already been certified clean for every revocation its acl carries.
	CleanupSpace(ctx context.Context, sp clientspace.Space) error
	// InviteRevoked reports an invite the acl has actually revoked, and deletes its file straight
	// away, in the background, rather than leaving it for the next CleanupSpace.
	InviteRevoked(ctx context.Context, spaceId string, info domain.InviteInfo) error
}

// objectDeleter deletes a file object left behind by an old invite. Declared here rather than
// imported from core/block, which would be an import cycle.
type objectDeleter interface {
	DeleteObjectByFullID(id domain.FullID) error
}

func New() Service {
	return &service{}
}

type service struct {
	inviteStore       invitestore.Service
	coordinatorClient coordinatorclient.CoordinatorClient
	rpcStore          rpcstore.RpcStore
	fileObjectService fileobject.Service
	objectDeleter     objectDeleter
	spaceService      space.Service

	// ctx bounds the detached deletions, so that Close does not have to wait out their backoff
	ctx    context.Context
	cancel context.CancelFunc

	// eagerMu guards closed and the WaitGroup counter together: a revocation racing Close would
	// otherwise Add to a WaitGroup that Wait is already blocked on, which panics.
	eagerMu sync.Mutex
	closed  bool
	eager   sync.WaitGroup

	// passes bounds how many spaces are scanned at once
	passes chan struct{}
}

func (s *service) Init(a *app.App) error {
	s.inviteStore = app.MustComponent[invitestore.Service](a)
	s.coordinatorClient = app.MustComponent[coordinatorclient.CoordinatorClient](a)
	s.rpcStore = app.MustComponent[rpcstore.Service](a).NewStore()
	s.fileObjectService = app.MustComponent[fileobject.Service](a)
	s.objectDeleter = app.MustComponent[objectDeleter](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.passes = make(chan struct{}, concurrentPasses)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) Run(_ context.Context) error {
	return nil
}

func (s *service) Close(_ context.Context) error {
	s.eagerMu.Lock()
	s.closed = true
	s.eagerMu.Unlock()

	s.cancel()
	s.eager.Wait()
	return nil
}

// goEager starts a detached deletion, unless the service is already closing.
func (s *service) goEager(f func(ctx context.Context)) {
	s.eagerMu.Lock()
	defer s.eagerMu.Unlock()
	if s.closed {
		return
	}
	s.eager.Add(1)
	go func() {
		defer s.eager.Done()
		ctx, cancel := context.WithTimeout(s.ctx, eagerDeleteTimeout)
		defer cancel()
		f(ctx)
	}()
}

// CleanupSpace deletes the invite files of every invite of sp that the acl has revoked.
func (s *service) CleanupSpace(ctx context.Context, sp clientspace.Space) error {
	if s.ctx.Err() != nil {
		// the app is going down. The caller's own context is cancelled when the space unloads, which is
		// what actually bounds this, but there is no reason to start work we are about to abandon.
		return nil
	}

	acl, err := s.readAcl(sp)
	if err != nil {
		return fmt.Errorf("read acl: %w", err)
	}
	if acl.lastRevocation == noRevocations {
		// Nothing has ever been revoked in this space, so it holds no invite file that may be removed:
		// not a space that was never shared, not one whose only invite is still in use, not a guest
		// invite, which is never revoked. decide() could not clear a single candidate here, so there is
		// nothing to scan for and nothing to certify.
		//
		// This is the cheap exit nearly every space takes: an in-memory walk of the acl records, no
		// coordinator round trip, no history replay, and not even a spaceview write.
		return nil
	}
	if !acl.isOwner {
		// ownership can still be transferred to us, so the space stays uncertified and is re-checked
		// on the next load. The acl read above is all that costs.
		return nil
	}
	certified, err := s.certificate(ctx, sp.Id())
	if err != nil {
		return fmt.Errorf("read clean certificate: %w", err)
	}
	if certified == acl.lastRevocation {
		// every revocation the acl carries has already been cleaned up, here or on another device
		return nil
	}

	// Everything below reaches the network and replays the workspace object's history. Hold the line
	// here rather than let every space in the account do it at once.
	select {
	case s.passes <- struct{}{}:
		defer func() { <-s.passes }()
	case <-ctx.Done():
		return nil
	case <-s.ctx.Done():
		return nil
	}

	workspace, err := s.readWorkspace(sp)
	if err != nil {
		return fmt.Errorf("read workspace: %w", err)
	}

	// From here on the acl decides which files go and whether the space gets a certificate that
	// silences this pass on every device. Neither is worth guessing at from a local copy that may be
	// behind.
	acl, err = s.refreshAcl(ctx, sp)
	if err != nil {
		// leave the space uncertified rather than act on an acl we could not confirm
		log.Warn("refresh acl", zap.String("spaceId", sp.Id()), zap.Error(err))
		return nil
	}

	candidates, err := s.collectCandidates(ctx, sp)
	if err != nil {
		return fmt.Errorf("collect invite files from workspace history: %w", err)
	}

	complete := true
	for _, candidate := range candidates {
		disposition, reason := s.cleanupInvite(ctx, sp, acl, workspace, candidate)
		switch disposition {
		case dispositionResolved:
		case dispositionSkip:
			log.Warn("give up on invite file",
				zap.String("spaceId", sp.Id()), zap.String("cid", candidate.cid), zap.Error(reason))
		default:
			// deferred, and anything a later change adds: an unrecognised disposition costs a pass, it
			// does not earn a certificate
			log.Warn("invite file cleanup deferred",
				zap.String("spaceId", sp.Id()), zap.String("cid", candidate.cid), zap.Error(reason))
			complete = false
		}
	}
	if !complete {
		return nil
	}
	// certify against the acl the scan actually ran on. Should a revocation have landed since, the
	// certificate no longer matches the acl and the next pass runs again — which is exactly right,
	// because that revocation's file is not in the candidate list this pass worked from.
	return s.certify(ctx, sp.Id(), acl.lastRevocation)
}

func (s *service) InviteRevoked(ctx context.Context, spaceId string, info domain.InviteInfo) error {
	if info.InviteFileCid == "" || info.InviteFileKey == "" {
		return nil
	}

	// Detached and bounded: the caller is an RPC handler and must not wait out a file node that is
	// not answering. Nothing depends on this succeeding.
	s.goEager(func(ctx context.Context) {
		sp, err := s.spaceService.Get(ctx, spaceId)
		if err != nil {
			log.Warn("eager invite cleanup: get space", zap.String("spaceId", spaceId), zap.Error(err))
			return
		}
		acl, err := s.refreshAcl(ctx, sp)
		if err != nil {
			log.Warn("eager invite cleanup: refresh acl", zap.String("spaceId", spaceId), zap.Error(err))
			return
		}
		workspace, err := s.readWorkspace(sp)
		if err != nil {
			log.Warn("eager invite cleanup: read workspace", zap.String("spaceId", spaceId), zap.Error(err))
			return
		}
		file := inviteFile{cid: info.InviteFileCid, key: info.InviteFileKey}
		if disposition, reason := s.cleanupInvite(ctx, sp, acl, workspace, file); disposition != dispositionResolved {
			log.Warn("eager invite cleanup did not finish",
				zap.String("spaceId", spaceId), zap.String("cid", info.InviteFileCid), zap.Error(reason))
		}
	})
	return nil
}

// noRevocations is the fingerprint of an acl that has never revoked an invite. It has to be a value
// of its own: an empty fingerprint is what an absent certificate reads as, and a space that has
// nothing to clean up must still be certifiable.
const noRevocations = "none"

// aclView is everything the acl has to say about a space's invites, read once under its lock.
type aclView struct {
	isOwner bool
	// lastRevocation is the id of the last record revoking an invite, or noRevocations. It is what a
	// clean certificate is issued against: revoking an invite appends a record and moves it, which
	// voids the certificate on every device at once, with nothing to withdraw and nothing to race.
	// Generating an invite does not move it — a new invite leaves nothing behind to delete.
	lastRevocation string
	// live are the invite keys the acl still honours
	live []crypto.PubKey
	// known are the invite keys of every invite record the acl carries, revoked or not. An invite key
	// that is not in here is one this device has never seen a record for, which is not the same thing
	// as a revoked one.
	known []crypto.PubKey
}

func (a aclView) isLive(key crypto.PubKey) bool {
	return containsKey(a.live, key)
}

func (a aclView) isKnown(key crypto.PubKey) bool {
	return containsKey(a.known, key)
}

func containsKey(keys []crypto.PubKey, key crypto.PubKey) bool {
	for _, k := range keys {
		if key.Equals(k) {
			return true
		}
	}
	return false
}

// refreshAcl brings the space's acl up to the coordinator's before anything is decided on it, and
// returns the refreshed view.
//
// It costs one round trip: AclGetRecords returns the records *after* the head it is given, so an acl
// that is already current answers with an empty list. Nothing is downloaded in the common case, and
// nothing is asked at all for a space that never had an invite — CleanupSpace short-circuits before
// this on the local acl.
//
// Being behind is not exotic here. The acl and the workspace object are separately synced logs, and
// an invite record that the node accepted but did not manage to hand back leaves the invite live on
// the coordinator and invisible locally, which is precisely the state in which absence must not be
// read as revocation.
func (s *service) refreshAcl(ctx context.Context, sp clientspace.Space) (aclView, error) {
	acl := sp.CommonSpace().Acl()
	acl.RLock()
	head := acl.Head().Id
	acl.RUnlock()

	// one attempt, no backoff: an unreachable coordinator means this pass does not run, and the next
	// space load runs it again. Retrying here would only park a goroutine per space for the whole
	// backoff while the device is offline.
	records, err := s.coordinatorClient.AclGetRecords(ctx, sp.Id(), head)
	if err != nil {
		return aclView{}, fmt.Errorf("get acl records: %w", err)
	}
	if len(records) > 0 {
		acl.Lock()
		err = acl.AddRawRecords(records)
		acl.Unlock()
		// the sync protocol may have delivered the same records while we were asking for them
		if err != nil && !errors.Is(err, list.ErrRecordAlreadyExists) {
			return aclView{}, fmt.Errorf("add acl records: %w", err)
		}
	}
	return s.readAcl(sp)
}

// readAcl collects what the acl knows about the space's invites. AclState().Invites() only lists the
// live ones, so the raw records are the only place a revoked invite leaves a trace.
func (s *service) readAcl(sp clientspace.Space) (aclView, error) {
	acl := sp.CommonSpace().Acl()
	acl.RLock()
	defer acl.RUnlock()

	state := acl.AclState()
	if state == nil {
		return aclView{}, errors.New("no acl state")
	}
	view := readInviteRecords(acl.Records())
	view.isOwner = state.Permissions(state.AccountKey().GetPublic()).IsOwner()
	for _, invite := range state.Invites() {
		view.live = append(view.live, invite.Key)
	}
	return view, nil
}

// readInviteRecords walks the acl's raw records for everything it says about invites: which ones it
// has ever carried, and which revocation came last.
func readInviteRecords(records []*list.AclRecord) aclView {
	view := aclView{lastRevocation: noRevocations}
	for _, record := range records {
		data, ok := record.Model.(*aclrecordproto.AclData)
		if !ok {
			// the root record has a different model and never carries an invite
			continue
		}
		for _, content := range data.GetAclContent() {
			if content.GetInviteRevoke() != nil {
				// records are append-only and in order, so the last one to set this wins. A batch that
				// revokes several invites at once is still one record, and one fingerprint.
				view.lastRevocation = record.Id
			}
			invite := content.GetInvite()
			if invite == nil {
				continue
			}
			key, err := crypto.UnmarshalEd25519PublicKeyProto(invite.InviteKey)
			if err != nil {
				log.Warn("acl invite record with an unreadable key", zap.String("recordId", record.Id))
				continue
			}
			view.known = append(view.known, key)
		}
	}
	return view
}

// workspaceView is what the workspace object currently says about the space's invites.
type workspaceView struct {
	// liveCids are the invite files the workspace currently points at
	liveCids map[string]struct{}
}

func (w workspaceView) saysLive(cid string) bool {
	_, ok := w.liveCids[cid]
	return ok
}

func (s *service) readWorkspace(sp clientspace.Space) (workspaceView, error) {
	view := workspaceView{liveCids: map[string]struct{}{}}
	err := sp.Do(sp.DerivedIDs().Workspace, func(sb smartblock.SmartBlock) error {
		inviteObject, ok := sb.(domain.InviteObject)
		if !ok {
			return errors.New("workspace is not an invite object")
		}
		if info := inviteObject.GetExistingInviteInfo(); info.InviteFileCid != "" {
			view.liveCids[info.InviteFileCid] = struct{}{}
		}
		if guestCid, _ := inviteObject.GetExistingGuestInviteInfo(); guestCid != "" {
			view.liveCids[guestCid] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return workspaceView{}, err
	}
	return view, nil
}

// collectCandidates walks the whole change history of the workspace object and returns every invite
// file it ever pointed at. The invite that is current stays in: whether it is live is for the acl to
// say, not for a detail that may be behind.
func (s *service) collectCandidates(ctx context.Context, sp clientspace.Space) ([]inviteFile, error) {
	// empty Heads means BuildFull: the whole tree from the root, all branches. This is read-only and
	// does not open the workspace as a smartblock.
	tree, err := sp.TreeBuilder().BuildHistoryTree(ctx, sp.DerivedIDs().Workspace, objecttreebuilder.HistoryTreeOpts{})
	if err != nil {
		return nil, fmt.Errorf("build workspace history tree: %w", err)
	}

	collector := newInviteFileCollector()
	err = tree.IterateRoot(sourceimpl.UnmarshalChange, func(change *objecttree.Change) bool {
		if model, ok := change.Model.(*pb.Change); ok {
			collector.addChange(model)
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("iterate workspace history: %w", err)
	}
	return collector.result(), nil
}

// cleanupInvite deletes one invite file, if it is allowed to.
func (s *service) cleanupInvite(
	ctx context.Context,
	sp clientspace.Space,
	acl aclView,
	workspace workspaceView,
	file inviteFile,
) (disposition, error) {
	inviteCid, err := cid.Parse(file.cid)
	if err != nil {
		return dispositionSkip, fmt.Errorf("parse invite cid: %w", err)
	}
	fileKey, err := encode.DecodeKeyFromBase58(file.key)
	if err != nil {
		return dispositionSkip, fmt.Errorf("decode invite file key: %w", err)
	}

	exists, err := s.inviteFileExists(ctx, sp.Id(), inviteCid)
	if err != nil {
		return dispositionDefer, fmt.Errorf("check invite file: %w", err)
	}
	if !exists {
		// nothing left on the file node, so there is nothing to establish. The local traces still go.
		return s.deleteLocalTraces(ctx, sp, inviteCid)
	}

	payload, err := s.invitedPayload(ctx, inviteCid, fileKey)
	if err != nil {
		if isPermanent(err) {
			return dispositionSkip, err
		}
		return dispositionDefer, err
	}
	if disposition, reason := decide(payload, sp.Id(), acl, workspace.saysLive(file.cid)); disposition != dispositionDelete {
		return disposition, reason
	}

	if disposition, err := s.deleteLocalTraces(ctx, sp, inviteCid); disposition != dispositionResolved {
		return disposition, err
	}
	return s.deleteFromNode(ctx, sp.Id(), inviteCid)
}

func (s *service) invitedPayload(ctx context.Context, inviteCid cid.Cid, fileKey crypto.SymKey) (*model.InvitePayload, error) {
	var invite *model.Invite
	err := withRetry(ctx, func() (err error) {
		invite, err = s.inviteStore.GetInvite(ctx, inviteCid, fileKey)
		if errors.Is(err, invitestore.ErrInviteUnreadable) {
			// the key does not decrypt this block, or what came out is not an invite. Fetching it
			// again will not change that, and deferring on it would rescan the space on every launch
			// for the rest of time.
			return permanent(err)
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get invite file: %w", err)
	}
	var payload model.InvitePayload
	if err = payload.Unmarshal(invite.Payload); err != nil {
		return nil, permanent(fmt.Errorf("unmarshal invite payload: %w", err))
	}
	return &payload, nil
}

// inviteFileExists reports whether the file node still holds the invite block. Asking first means an
// invite that was already deleted — by an earlier run that could not record the result, or by another
// device — resolves at once instead of hanging on a block fetch that can never succeed.
func (s *service) inviteFileExists(ctx context.Context, spaceId string, inviteCid cid.Cid) (bool, error) {
	var availability []*fileproto.BlockAvailability
	err := withRetry(ctx, func() (err error) {
		availability, err = s.rpcStore.CheckAvailability(ctx, spaceId, []cid.Cid{inviteCid})
		return err
	})
	if err != nil {
		return false, err
	}
	for _, block := range availability {
		if block.Status != fileproto.AvailabilityStatus_NotExists {
			return true, nil
		}
	}
	return false, nil
}

// deleteLocalTraces removes the file object an old invite may have left behind and drops the space's
// binding of the invite block on the file node.
//
// The unbind is what lets the coordinator delete the block at all: it refuses one that is still bound
// to a space. Invite files used to be bound — to the space itself before GO-2832, to the tech space
// after it, and to nothing at all since GO-6067 — which is why both spaces are named. Almost every
// invite is already unbound, so a failure here is expected and only reappears as a failure of the
// deletion itself.
func (s *service) deleteLocalTraces(ctx context.Context, sp clientspace.Space, inviteCid cid.Cid) (disposition, error) {
	fileId := domain.FileId(inviteCid.String())
	for _, spaceId := range []string{sp.Id(), s.spaceService.TechSpaceId()} {
		fullId := domain.FullFileId{SpaceId: spaceId, FileId: fileId}
		objectId, _, err := s.fileObjectService.GetObjectDetailsByFileId(fullId)
		switch {
		case errors.Is(err, filemodels.ErrObjectNotFound):
		case err != nil:
			return dispositionDefer, fmt.Errorf("find file object: %w", err)
		default:
			if err = s.objectDeleter.DeleteObjectByFullID(domain.FullID{SpaceID: spaceId, ObjectID: objectId}); err != nil {
				return dispositionDefer, fmt.Errorf("delete file object: %w", err)
			}
		}
		if err = s.rpcStore.DeleteFiles(ctx, spaceId, fileId); err != nil {
			log.Debug("unbind invite file",
				zap.String("spaceId", spaceId), zap.String("cid", inviteCid.String()), zap.Error(err))
		}
	}
	return dispositionResolved, nil
}

// deleteFromNode asks the coordinator to delete the invite's file. The coordinator resolves the cid
// to its invite through the binding AclUploadInvite made.
func (s *service) deleteFromNode(ctx context.Context, spaceId string, inviteCid cid.Cid) (disposition, error) {
	err := withRetry(ctx, func() error {
		err := s.coordinatorClient.AclDeleteInvite(ctx, spaceId, inviteCid)
		switch {
		case err == nil, errors.Is(err, fileprotoerr.ErrCIDNotFound):
			return nil
		case errors.Is(err, coordinatorproto.ErrInviteStillActive):
			// unreachable: decide() proved the invite revoked. If it ever fires, our view of the acl
			// disagreed with the coordinator's.
			log.Error("coordinator rejected invite deletion as still active",
				zap.String("spaceId", spaceId), zap.String("cid", inviteCid.String()))
			return permanent(fmt.Errorf("%w: %w", errInviteLive, err))
		case errors.Is(err, coordinatorproto.ErrForbidden):
			return permanent(err)
		default:
			return fmt.Errorf("coordinator delete invite: %w", err)
		}
	})
	switch {
	case err == nil:
		return dispositionResolved, nil
	case errors.Is(err, errInviteLive):
		// come back once the acl agrees with the coordinator
		return dispositionDefer, err
	case isPermanent(err):
		return dispositionSkip, err
	default:
		return dispositionDefer, err
	}
}
