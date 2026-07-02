package spacev2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclwaiter"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/deletioncontroller"
	"github.com/anyproto/anytype-heart/space/internal/components/aclnotifications"
	"github.com/anyproto/anytype-heart/space/internal/components/aclobjectmanager"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/migration"
	"github.com/anyproto/anytype-heart/space/internal/components/participantwatcher"
	"github.com/anyproto/anytype-heart/space/internal/components/personalmigration"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// deleteStorageTimeout bounds the storage-delete lock acquisition during
// offload (matches v1's deleteStorageLockTimeout).
const deleteStorageTimeout = 10 * time.Second

// BackendDeps carries the shared dependencies every space backend calls into.
// Assembled once by the service; the same instance is shared by all backends.
type BackendDeps struct {
	// App is the parent for per-space pipeline child apps. Its chain must
	// resolve the main-app components the reused pipeline components need
	// (accountservice, objectstore, identity service, notifications, indexer/
	// SpaceLoaderListener, acl joining client, nodeconf, ...) plus TechSpace.
	App                *app.App
	TechSpace          techspace.TechSpace
	SpaceCore          spacecore.SpaceCoreService
	AccountService     accountservice.Service
	ObjectFactory      objectcache.ObjectFactory
	Storage            storage.ClientStorage
	Indexer            dependencies.SpaceIndexer
	Installer          dependencies.BundledObjectsInstaller
	MigrationService   dependencies.MigrationService
	FileOffloader      dependencies.FileOffloader
	DeletionController deletioncontroller.DeletionController
	PersonalSpaceId    string
	// AccountMetadataPayload is this account's marshalled ACL metadata
	// (domain.DeriveAccountMetadata). Passed as owner metadata to the ACL
	// object manager for the personal and guest (streamable) spaces, exactly
	// as v1 does.
	AccountMetadataPayload []byte
}

// pipelineCloser is what a started post-load pipeline exposes back to the
// backend; *app.App satisfies it.
type pipelineCloser interface {
	Close(ctx context.Context) error
}

// spaceBackend implements Backend for one real space against the reused
// layers. All methods run on the controller's reconcile goroutine, so the
// per-load residency fields need no locking.
type spaceBackend struct {
	spaceId string
	deps    *BackendDeps

	view techspace.SpaceView // lazily fetched, cached for the account session

	// residency state between Load and Unload
	pipeline   pipelineCloser
	loadCancel context.CancelFunc

	// seams: default to the real implementations, overridden in tests
	build         func(ctx, loadCtx context.Context, guestKey crypto.PrivKey) (clientspace.Space, error)
	startPipeline func(ctx context.Context, sp clientspace.Space, guestKey crypto.PrivKey) (pipelineCloser, error)
	runJoinWaiter func(ctx context.Context, aclHeadId string) error
}

func newSpaceBackend(spaceId string, deps *BackendDeps) *spaceBackend {
	b := &spaceBackend{
		spaceId: spaceId,
		deps:    deps,
	}
	b.build = b.buildClientSpace
	b.startPipeline = b.startComponentPipeline
	b.runJoinWaiter = b.runAclJoinWaiter
	return b
}

func (b *spaceBackend) spaceView(ctx context.Context) (techspace.SpaceView, error) {
	if b.view != nil {
		return b.view, nil
	}
	view, err := b.deps.TechSpace.GetSpaceView(ctx, b.spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space view of %s: %w", b.spaceId, err)
	}
	b.view = view
	return view, nil
}

func (b *spaceBackend) AccountStatus(ctx context.Context) (spaceinfo.AccountStatus, error) {
	view, err := b.spaceView(ctx)
	if err != nil {
		return spaceinfo.AccountStatusUnknown, err
	}
	view.Lock()
	defer view.Unlock()
	info := view.GetPersistentInfo()
	return info.GetAccountStatus(), nil
}

func (b *spaceBackend) persistentInfo(ctx context.Context) (spaceinfo.SpacePersistentInfo, error) {
	view, err := b.spaceView(ctx)
	if err != nil {
		return spaceinfo.SpacePersistentInfo{}, err
	}
	view.Lock()
	defer view.Unlock()
	return view.GetPersistentInfo(), nil
}

func (b *spaceBackend) localStatus(ctx context.Context) (spaceinfo.LocalStatus, error) {
	view, err := b.spaceView(ctx)
	if err != nil {
		return spaceinfo.LocalStatusUnknown, err
	}
	view.Lock()
	defer view.Unlock()
	info := view.GetLocalInfo()
	return info.GetLocalStatus(), nil
}

func (b *spaceBackend) setLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	view, err := b.spaceView(ctx)
	if err != nil {
		return err
	}
	view.Lock()
	defer view.Unlock()
	if err = view.SetSpaceLocalInfo(info); err != nil {
		return fmt.Errorf("set local info of %s: %w", b.spaceId, err)
	}
	return nil
}

func (b *spaceBackend) setPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	view, err := b.spaceView(ctx)
	if err != nil {
		return err
	}
	view.Lock()
	defer view.Unlock()
	if err = view.SetSpacePersistentInfo(info); err != nil {
		return fmt.Errorf("set persistent info of %s: %w", b.spaceId, err)
	}
	return nil
}

// Load builds the resident space: publish LocalStatus, build via the reused
// clientspace/spacecore layers, gate on mandatory objects and the recorded
// ACL head, then start the reused post-load component pipeline (acl object
// manager, participant watcher, notifications, migrations). OnSpaceLoad fires
// from inside the pipeline (aclobjectmanager), as in v1.
func (b *spaceBackend) Load(ctx context.Context) (clientspace.Space, error) {
	info, err := b.persistentInfo(ctx)
	if err != nil {
		return nil, err
	}
	var guestKey crypto.PrivKey
	if info.EncodedKey != "" {
		guestKey, err = crypto.DecodeKeyFromString(info.EncodedKey, crypto.UnmarshalEd25519PrivateKey, nil)
		if err != nil {
			return nil, Fatal(fmt.Errorf("decode guest key of %s: %w", b.spaceId, err))
		}
	}

	// Optimistic-Ok fast path: a space whose store is on disk and was Ok last
	// session keeps reporting Ok during the rebuild, so clients don't hide it
	// on cold start behind a transient Loading.
	local, err := b.localStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !(local == spaceinfo.LocalStatusOk && b.deps.Storage.SpaceExists(b.spaceId)) {
		loading := spaceinfo.NewSpaceLocalInfo(b.spaceId)
		loading.SetLocalStatus(spaceinfo.LocalStatusLoading)
		if err = b.setLocalInfo(ctx, loading); err != nil {
			return nil, err
		}
	}

	loadCtx, loadCancel := context.WithCancel(context.Background())
	sp, err := b.build(ctx, loadCtx, guestKey)
	if err != nil {
		loadCancel()
		return nil, b.mapLoadError(ctx, err)
	}
	cleanup := func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), deleteStorageTimeout)
		defer cancel()
		if closeErr := sp.Close(closeCtx); closeErr != nil {
			log.Warn("close space after failed load", zap.String("spaceId", b.spaceId), zap.Error(closeErr))
		}
		loadCancel()
	}

	accessType := spaceinfo.AccessTypePrivate
	if b.spaceId == b.deps.PersonalSpaceId {
		accessType = spaceinfo.AccessTypePersonal
	}
	if err = b.setAccessType(ctx, accessType); err != nil {
		cleanup()
		return nil, err
	}

	if err = sp.WaitMandatoryObjects(ctx); err != nil {
		cleanup()
		return nil, b.mapLoadError(ctx, err)
	}
	if head := info.GetAclHeadId(); head != "" {
		if err = aclHasHead(sp, head); err != nil {
			cleanup()
			// retryable: the ACL simply hasn't synced up to the join head yet
			return nil, err
		}
	}

	pipeline, err := b.startPipeline(ctx, sp, guestKey)
	if err != nil {
		cleanup()
		return nil, err
	}
	b.pipeline = pipeline
	b.loadCancel = loadCancel

	ok := spaceinfo.NewSpaceLocalInfo(b.spaceId)
	ok.SetLocalStatus(spaceinfo.LocalStatusOk)
	if err = b.setLocalInfo(ctx, ok); err != nil {
		log.Warn("publish LocalStatusOk", zap.String("spaceId", b.spaceId), zap.Error(err))
	}
	return sp, nil
}

func (b *spaceBackend) setAccessType(ctx context.Context, accessType spaceinfo.AccessType) error {
	view, err := b.spaceView(ctx)
	if err != nil {
		return err
	}
	view.Lock()
	defer view.Unlock()
	if err = view.SetAccessType(accessType); err != nil {
		return fmt.Errorf("set access type of %s: %w", b.spaceId, err)
	}
	return nil
}

// aclHasHead reports whether the local ACL already contains the recorded
// head; used to gate load completion after a join was accepted.
func aclHasHead(sp clientspace.Space, headId string) error {
	acl := sp.CommonSpace().Acl()
	acl.RLock()
	defer acl.RUnlock()
	if _, err := acl.Get(headId); err != nil {
		return fmt.Errorf("acl head %s not synced yet: %w", headId, err)
	}
	return nil
}

// mapLoadError translates build failures into LocalStatus publications and
// the controller's retry semantics (terminal outcomes are Fatal; transient
// ones keep LocalStatusLoading and are retried with backoff; a canceled ctx
// means shutdown and must leave the persisted status untouched).
func (b *spaceBackend) mapLoadError(ctx context.Context, err error) error {
	statusCtx, cancel := context.WithTimeout(context.Background(), deleteStorageTimeout)
	defer cancel()
	switch {
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, spacecore.ErrSpaceDeletionPending):
		info := spaceinfo.NewSpaceLocalInfo(b.spaceId)
		info.SetLocalStatus(spaceinfo.LocalStatusMissing).
			SetRemoteStatus(spaceinfo.RemoteStatusWaitingDeletion)
		b.publishFailedStatus(statusCtx, info)
		return Fatal(err)
	case errors.Is(err, spacecore.ErrSpaceIsDeleted):
		info := spaceinfo.NewSpaceLocalInfo(b.spaceId)
		info.SetLocalStatus(spaceinfo.LocalStatusMissing).
			SetRemoteStatus(spaceinfo.RemoteStatusDeleted)
		b.publishFailedStatus(statusCtx, info)
		return Fatal(err)
	case errors.Is(err, objecttree.ErrHasInvalidChanges) || errors.Is(err, spacedomain.ErrUnexpectedSpaceType):
		info := spaceinfo.NewSpaceLocalInfo(b.spaceId)
		info.SetLocalStatus(spaceinfo.LocalStatusMissing)
		b.publishFailedStatus(statusCtx, info)
		return Fatal(err)
	default:
		return err
	}
}

func (b *spaceBackend) publishFailedStatus(ctx context.Context, info spaceinfo.SpaceLocalInfo) {
	if err := b.setLocalInfo(ctx, info); err != nil {
		log.Warn("publish failed-load status", zap.String("spaceId", b.spaceId), zap.Error(err))
	}
}

// Unload releases the resident space keeping on-disk data: close the
// component pipeline (which fires OnSpaceUnload), then the space itself
// (which also evicts the spacecore ocache entry, stopping sync).
func (b *spaceBackend) Unload(ctx context.Context, sp clientspace.Space) error {
	var errs []error
	if b.pipeline != nil {
		if err := b.pipeline.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("close pipeline: %w", err))
		}
		b.pipeline = nil
	}
	if b.loadCancel != nil {
		b.loadCancel()
		b.loadCancel = nil
	}
	if sp != nil {
		if err := sp.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("close space: %w", err))
		}
	}
	return errors.Join(errs...)
}

// Offload deletes the local data of a deleted space. Idempotent: an already
// offloaded space (LocalStatusMissing) only re-registers with the deletion
// controller, and missing storage is not an error.
func (b *spaceBackend) Offload(ctx context.Context) error {
	local, err := b.localStatus(ctx)
	if err != nil {
		return err
	}
	b.deps.DeletionController.AddSpaceToDelete(b.spaceId)
	if local == spaceinfo.LocalStatusMissing {
		return nil
	}
	delCtx, cancel := context.WithTimeout(ctx, deleteStorageTimeout)
	err = b.deps.Storage.DeleteSpaceStorage(delCtx, b.spaceId)
	cancel()
	if err != nil && !errors.Is(err, spacestorage.ErrSpaceStorageMissing) {
		return fmt.Errorf("delete space storage: %w", err)
	}
	if _, _, err = b.deps.FileOffloader.FileSpaceOffload(ctx, b.spaceId, true); err != nil {
		return fmt.Errorf("offload files: %w", err)
	}
	if err = b.deps.Indexer.RemoveIndexes(b.spaceId); err != nil {
		return fmt.Errorf("remove indexes: %w", err)
	}
	missing := spaceinfo.NewSpaceLocalInfo(b.spaceId)
	missing.SetLocalStatus(spaceinfo.LocalStatusMissing)
	return b.setLocalInfo(ctx, missing)
}

// Join publishes the joining participant status and runs the ACL waiter until
// the join resolves; the waiter's callbacks write the resulting AccountStatus
// (Active or Deleted) to the SpaceView.
func (b *spaceBackend) Join(ctx context.Context) error {
	view, err := b.spaceView(ctx)
	if err != nil {
		return err
	}
	view.Lock()
	persistent := view.GetPersistentInfo()
	aclHeadId := persistent.GetAclHeadId()
	if err = view.SetMyParticipantStatus(model.ParticipantStatus_Joining); err != nil {
		view.Unlock()
		return fmt.Errorf("set my participant status of %s: %w", b.spaceId, err)
	}
	view.Unlock()
	unknown := spaceinfo.NewSpaceLocalInfo(b.spaceId)
	unknown.SetLocalStatus(spaceinfo.LocalStatusUnknown)
	if err = b.setLocalInfo(ctx, unknown); err != nil {
		return err
	}
	return b.runJoinWaiter(ctx, aclHeadId)
}

// buildClientSpace is the real build seam: spacecore.Get + clientspace.BuildSpace.
func (b *spaceBackend) buildClientSpace(ctx, loadCtx context.Context, guestKey crypto.PrivKey) (clientspace.Space, error) {
	if guestKey != nil {
		ctx = context.WithValue(ctx, spacecore.OptsKey, spacecore.Opts{SignKey: guestKey})
	}
	coreSpace, err := b.deps.SpaceCore.Get(ctx, b.spaceId)
	if err != nil {
		return nil, fmt.Errorf("get core space: %w", err)
	}
	sp, err := clientspace.BuildSpace(ctx, clientspace.SpaceDeps{
		Indexer:          b.deps.Indexer,
		Installer:        b.deps.Installer,
		CommonSpace:      coreSpace,
		ObjectFactory:    b.deps.ObjectFactory,
		AccountService:   b.deps.AccountService,
		PersonalSpaceId:  b.deps.PersonalSpaceId,
		StorageService:   b.deps.Storage,
		SpaceCore:        b.deps.SpaceCore,
		LoadCtx:          loadCtx,
		KeyValueObserver: coreSpace.KeyValueObserver(),
		MigrationService: b.deps.MigrationService,
	})
	if err != nil {
		return nil, fmt.Errorf("build client space: %w", err)
	}
	return sp, nil
}

// startComponentPipeline hosts the reused post-load domain components in a
// child app. The presetLoader shim satisfies their spaceloader.SpaceLoader
// wait barrier with the already-built space; spacestatus is the real v1
// component (resolved both by type and by CName).
func (b *spaceBackend) startComponentPipeline(ctx context.Context, sp clientspace.Space, guestKey crypto.PrivKey) (pipelineCloser, error) {
	var ownerMetadata []byte
	if b.spaceId == b.deps.PersonalSpaceId || guestKey != nil {
		ownerMetadata = b.deps.AccountMetadataPayload
	}
	child := b.deps.App.ChildApp()
	child.Register(spacestatus.New(b.spaceId)).
		Register(newPresetLoader(sp)).
		Register(aclnotifications.NewAclNotificationSender()).
		Register(aclobjectmanager.New(ownerMetadata, guestKey)).
		Register(participantwatcher.New()).
		Register(migration.New())
	if b.spaceId == b.deps.PersonalSpaceId {
		child.Register(personalmigration.New())
	}
	if err := child.Start(ctx); err != nil {
		return nil, fmt.Errorf("start space pipeline: %w", err)
	}
	return child, nil
}

// runAclJoinWaiter hosts the any-sync aclwaiter until the join is accepted
// (write AccountStatusActive + head) or rejected (write AccountStatusDeleted
// + head, emit the decline notification), mirroring v1's joiner.
func (b *spaceBackend) runAclJoinWaiter(ctx context.Context, aclHeadId string) error {
	done := make(chan error, 1)
	finish := func(res error) {
		select {
		case done <- res:
		default:
		}
	}
	notifSender := aclnotifications.NewAclNotificationSender()
	child := b.deps.App.ChildApp()
	child.Register(notifSender).
		Register(aclwaiter.New(b.spaceId, aclHeadId,
			func(acl list.AclList) error {
				info := spaceinfo.NewSpacePersistentInfo(b.spaceId)
				info.SetAccountStatus(spaceinfo.AccountStatusActive).
					SetAclHeadId(acl.Head().Id)
				if err := b.setPersistentInfo(ctx, info); err != nil {
					return err
				}
				finish(nil)
				return nil
			},
			func(acl list.AclList) error {
				info := spaceinfo.NewSpacePersistentInfo(b.spaceId)
				info.SetAccountStatus(spaceinfo.AccountStatusDeleted).
					SetAclHeadId(acl.Head().Id)
				if err := b.setPersistentInfo(ctx, info); err != nil {
					return err
				}
				notifSender.AddSingleRecord(acl.Id(), acl.Head(), 0, b.spaceId, spaceinfo.AccountStatusDeleted)
				finish(nil)
				return nil
			}))
	if err := child.Start(ctx); err != nil {
		return fmt.Errorf("start join waiter: %w", err)
	}
	var res error
	select {
	case <-ctx.Done():
		res = ctx.Err()
	case res = <-done:
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), deleteStorageTimeout)
	defer cancel()
	if err := child.Close(closeCtx); err != nil {
		log.Warn("close join waiter", zap.String("spaceId", b.spaceId), zap.Error(err))
	}
	return res
}
