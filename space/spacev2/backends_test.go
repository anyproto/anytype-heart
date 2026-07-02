package spacev2

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/deletioncontroller"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// fakeView records SpaceView reads/writes; unimplemented methods panic via
// the nil embed, which is fine — the backend must not call them.
type fakeView struct {
	techspace.SpaceView
	mu sync.Mutex

	persistent spaceinfo.SpacePersistentInfo
	local      spaceinfo.SpaceLocalInfo

	localWrites       []spaceinfo.SpaceLocalInfo
	persistentWrites  []spaceinfo.SpacePersistentInfo
	accessTypes       []spaceinfo.AccessType
	participantWrites []model.ParticipantStatus
	setLocalInfoErr   error
}

func (v *fakeView) Lock()   { v.mu.Lock() }
func (v *fakeView) Unlock() { v.mu.Unlock() }

func (v *fakeView) GetPersistentInfo() spaceinfo.SpacePersistentInfo { return v.persistent }
func (v *fakeView) GetLocalInfo() spaceinfo.SpaceLocalInfo          { return v.local }

func (v *fakeView) SetSpaceLocalInfo(info spaceinfo.SpaceLocalInfo) error {
	if v.setLocalInfoErr != nil {
		return v.setLocalInfoErr
	}
	v.localWrites = append(v.localWrites, info)
	return nil
}

func (v *fakeView) SetSpacePersistentInfo(info spaceinfo.SpacePersistentInfo) error {
	v.persistentWrites = append(v.persistentWrites, info)
	return nil
}

func (v *fakeView) SetAccessType(acc spaceinfo.AccessType) error {
	v.accessTypes = append(v.accessTypes, acc)
	return nil
}

func (v *fakeView) SetMyParticipantStatus(status model.ParticipantStatus) error {
	v.participantWrites = append(v.participantWrites, status)
	return nil
}

func (v *fakeView) localStatuses() []spaceinfo.LocalStatus {
	v.mu.Lock()
	defer v.mu.Unlock()
	statuses := make([]spaceinfo.LocalStatus, 0, len(v.localWrites))
	for i := range v.localWrites {
		statuses = append(statuses, v.localWrites[i].GetLocalStatus())
	}
	return statuses
}

type fakeTechSpace struct {
	techspace.TechSpace
	view *fakeView
}

func (t *fakeTechSpace) GetSpaceView(ctx context.Context, spaceId string) (techspace.SpaceView, error) {
	return t.view, nil
}

type fakeStorage struct {
	storage.ClientStorage
	exists    bool
	deleted   []string
	deleteErr error
}

func (s *fakeStorage) SpaceExists(id string) bool { return s.exists }

func (s *fakeStorage) DeleteSpaceStorage(ctx context.Context, spaceId string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, spaceId)
	return nil
}

type fakeFileOffloader struct {
	offloaded []string
	err       error
}

func (f *fakeFileOffloader) FileSpaceOffload(ctx context.Context, spaceId string, includeNotPinned bool) (int, uint64, error) {
	if f.err != nil {
		return 0, 0, f.err
	}
	f.offloaded = append(f.offloaded, spaceId)
	return 0, 0, nil
}

type fakeIndexer struct {
	dependencies.SpaceIndexer
	removed []string
}

func (f *fakeIndexer) ReindexMarketplaceSpace(space clientspace.Space) error { return nil }
func (f *fakeIndexer) ReindexSpace(space clientspace.Space) error            { return nil }
func (f *fakeIndexer) RemoveIndexes(spaceID string) error {
	f.removed = append(f.removed, spaceID)
	return nil
}

type fakeDelController struct {
	deletioncontroller.DeletionController
	added []string
}

func (f *fakeDelController) AddSpaceToDelete(spaceId string) { f.added = append(f.added, spaceId) }
func (f *fakeDelController) UpdateCoordinatorStatus()        {}

// loadableSpace is a fake clientspace.Space good enough for backend tests.
type loadableSpace struct {
	clientspace.Space
	mandatoryErr error
	closed       int
}

func (s *loadableSpace) WaitMandatoryObjects(ctx context.Context) error { return s.mandatoryErr }
func (s *loadableSpace) Close(ctx context.Context) error {
	s.closed++
	return nil
}

type fakePipeline struct {
	closed int
}

func (p *fakePipeline) Close(ctx context.Context) error {
	p.closed++
	return nil
}

type backendFixture struct {
	*spaceBackend
	view          *fakeView
	storage       *fakeStorage
	files         *fakeFileOffloader
	indexer       *fakeIndexer
	delController *fakeDelController

	sp       *loadableSpace
	pipeline *fakePipeline
	buildErr error

	buildCalls    int
	pipelineCalls int
	joinCalls     []string
	joinErr       error
}

func newBackendFixture(t *testing.T) *backendFixture {
	fx := &backendFixture{
		view:          &fakeView{persistent: spaceinfo.NewSpacePersistentInfo(testSpaceId)},
		storage:       &fakeStorage{},
		files:         &fakeFileOffloader{},
		indexer:       &fakeIndexer{},
		delController: &fakeDelController{},
		sp:            &loadableSpace{},
		pipeline:      &fakePipeline{},
	}
	deps := &BackendDeps{
		TechSpace:          &fakeTechSpace{view: fx.view},
		Storage:            fx.storage,
		Indexer:            fx.indexer,
		FileOffloader:      fx.files,
		DeletionController: fx.delController,
		PersonalSpaceId:    "personalSpaceId",
	}
	fx.spaceBackend = newSpaceBackend(testSpaceId, deps)
	fx.build = func(ctx, loadCtx context.Context, guestKey crypto.PrivKey) (clientspace.Space, error) {
		fx.buildCalls++
		if fx.buildErr != nil {
			return nil, fx.buildErr
		}
		return fx.sp, nil
	}
	fx.startPipeline = func(ctx context.Context, sp clientspace.Space, guestKey crypto.PrivKey) (pipelineCloser, error) {
		fx.pipelineCalls++
		return fx.pipeline, nil
	}
	fx.runJoinWaiter = func(ctx context.Context, aclHeadId string) error {
		fx.joinCalls = append(fx.joinCalls, aclHeadId)
		return fx.joinErr
	}
	return fx
}

func TestBackendAccountStatus(t *testing.T) {
	// given
	fx := newBackendFixture(t)
	fx.view.persistent.SetAccountStatus(spaceinfo.AccountStatusJoining)

	// when
	status, err := fx.AccountStatus(context.Background())

	// then
	require.NoError(t, err)
	assert.Equal(t, spaceinfo.AccountStatusJoining, status)
}

func TestBackendLoadHappyPath(t *testing.T) {
	// given
	fx := newBackendFixture(t)

	// when
	sp, err := fx.Load(context.Background())

	// then
	require.NoError(t, err)
	assert.Same(t, fx.sp, sp)
	assert.Equal(t, 1, fx.buildCalls)
	assert.Equal(t, 1, fx.pipelineCalls)
	// Loading published before the build, Ok after
	assert.Equal(t, []spaceinfo.LocalStatus{spaceinfo.LocalStatusLoading, spaceinfo.LocalStatusOk}, fx.view.localStatuses())
	assert.Equal(t, []spaceinfo.AccessType{spaceinfo.AccessTypePrivate}, fx.view.accessTypes)
}

func TestBackendLoadPersonalAccessType(t *testing.T) {
	// given
	fx := newBackendFixture(t)
	fx.deps.PersonalSpaceId = testSpaceId

	// when
	_, err := fx.Load(context.Background())

	// then
	require.NoError(t, err)
	assert.Equal(t, []spaceinfo.AccessType{spaceinfo.AccessTypePersonal}, fx.view.accessTypes)
}

func TestBackendLoadOptimisticOkSkipsLoading(t *testing.T) {
	// given: store on disk and Ok in the previous session
	fx := newBackendFixture(t)
	fx.storage.exists = true
	fx.view.local.SetLocalStatus(spaceinfo.LocalStatusOk)

	// when
	_, err := fx.Load(context.Background())

	// then: no transient Loading published, only the final Ok
	require.NoError(t, err)
	assert.Equal(t, []spaceinfo.LocalStatus{spaceinfo.LocalStatusOk}, fx.view.localStatuses())
}

func TestBackendLoadDeletionPendingIsFatal(t *testing.T) {
	// given
	fx := newBackendFixture(t)
	fx.buildErr = spacecore.ErrSpaceDeletionPending

	// when
	_, err := fx.Load(context.Background())

	// then
	require.ErrorIs(t, err, spacecore.ErrSpaceDeletionPending)
	assert.True(t, isFatal(err))
	require.Len(t, fx.view.localWrites, 2)
	last := fx.view.localWrites[1]
	assert.Equal(t, spaceinfo.LocalStatusMissing, last.GetLocalStatus())
	assert.Equal(t, spaceinfo.RemoteStatusWaitingDeletion, last.GetRemoteStatus())
}

func TestBackendLoadSpaceDeletedIsFatal(t *testing.T) {
	// given
	fx := newBackendFixture(t)
	fx.buildErr = spacecore.ErrSpaceIsDeleted

	// when
	_, err := fx.Load(context.Background())

	// then
	require.ErrorIs(t, err, spacecore.ErrSpaceIsDeleted)
	assert.True(t, isFatal(err))
	last := fx.view.localWrites[len(fx.view.localWrites)-1]
	assert.Equal(t, spaceinfo.LocalStatusMissing, last.GetLocalStatus())
	assert.Equal(t, spaceinfo.RemoteStatusDeleted, last.GetRemoteStatus())
}

func TestBackendLoadTransientErrorKeepsLoading(t *testing.T) {
	// given
	fx := newBackendFixture(t)
	fx.buildErr = errors.New("network flake")

	// when
	_, err := fx.Load(context.Background())

	// then: not fatal, and the local status stays at Loading (no Missing)
	require.Error(t, err)
	assert.False(t, isFatal(err))
	assert.Equal(t, []spaceinfo.LocalStatus{spaceinfo.LocalStatusLoading}, fx.view.localStatuses())
}

func TestBackendLoadCanceledLeavesStatusUntouched(t *testing.T) {
	// given: shutdown interrupts the build
	fx := newBackendFixture(t)
	fx.storage.exists = true
	fx.view.local.SetLocalStatus(spaceinfo.LocalStatusOk)
	fx.buildErr = context.Canceled

	// when
	_, err := fx.Load(context.Background())

	// then: no status write at all — a healthy space must keep its Ok
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, isFatal(err))
	assert.Empty(t, fx.view.localWrites)
}

func TestBackendLoadMandatoryObjectsFailureClosesSpace(t *testing.T) {
	// given
	fx := newBackendFixture(t)
	fx.sp.mandatoryErr = errors.New("mandatory objects failed")

	// when
	_, err := fx.Load(context.Background())

	// then: the half-built space is not leaked
	require.Error(t, err)
	assert.False(t, isFatal(err))
	assert.Equal(t, 1, fx.sp.closed)
	assert.Zero(t, fx.pipelineCalls)
}

func TestBackendLoadBadGuestKeyIsFatal(t *testing.T) {
	// given
	fx := newBackendFixture(t)
	fx.view.persistent.SetEncodedKey("not-a-key")

	// when
	_, err := fx.Load(context.Background())

	// then
	require.Error(t, err)
	assert.True(t, isFatal(err))
	assert.Zero(t, fx.buildCalls)
}

func TestBackendUnload(t *testing.T) {
	// given: a loaded space
	fx := newBackendFixture(t)
	sp, err := fx.Load(context.Background())
	require.NoError(t, err)

	// when
	require.NoError(t, fx.Unload(context.Background(), sp))

	// then: pipeline closed before the space, residency dropped
	assert.Equal(t, 1, fx.pipeline.closed)
	assert.Equal(t, 1, fx.sp.closed)
	assert.Nil(t, fx.spaceBackend.pipeline)

	// and: a second unload (nothing resident) is harmless
	require.NoError(t, fx.Unload(context.Background(), nil))
	assert.Equal(t, 1, fx.pipeline.closed)
}

func TestBackendOffload(t *testing.T) {
	// given
	fx := newBackendFixture(t)

	// when
	require.NoError(t, fx.Offload(context.Background()))

	// then: registered for network deletion + local data removed + Missing
	assert.Equal(t, []string{testSpaceId}, fx.delController.added)
	assert.Equal(t, []string{testSpaceId}, fx.storage.deleted)
	assert.Equal(t, []string{testSpaceId}, fx.files.offloaded)
	assert.Equal(t, []string{testSpaceId}, fx.indexer.removed)
	assert.Equal(t, []spaceinfo.LocalStatus{spaceinfo.LocalStatusMissing}, fx.view.localStatuses())
}

func TestBackendOffloadAlreadyMissingSkipsWork(t *testing.T) {
	// given: an already offloaded space
	fx := newBackendFixture(t)
	fx.view.local.SetLocalStatus(spaceinfo.LocalStatusMissing)

	// when
	require.NoError(t, fx.Offload(context.Background()))

	// then: still re-registers with the deletion controller, no local work
	assert.Equal(t, []string{testSpaceId}, fx.delController.added)
	assert.Empty(t, fx.storage.deleted)
	assert.Empty(t, fx.files.offloaded)
	assert.Empty(t, fx.indexer.removed)
	assert.Empty(t, fx.view.localWrites)
}

func TestBackendOffloadFileErrorStopsBeforeStatusWrite(t *testing.T) {
	// given
	fx := newBackendFixture(t)
	fx.files.err = errors.New("file node unreachable")

	// when
	err := fx.Offload(context.Background())

	// then: error surfaces (controller retries), Missing not yet written
	require.Error(t, err)
	assert.Empty(t, fx.indexer.removed)
	assert.Empty(t, fx.view.localWrites)
}

func TestBackendJoin(t *testing.T) {
	// given: a joining space with a recorded acl head
	fx := newBackendFixture(t)
	fx.view.persistent.SetAccountStatus(spaceinfo.AccountStatusJoining).
		SetAclHeadId("aclHead1")

	// when
	require.NoError(t, fx.Join(context.Background()))

	// then: participant status + local Unknown published, waiter ran with the head
	assert.Equal(t, []model.ParticipantStatus{model.ParticipantStatus_Joining}, fx.view.participantWrites)
	assert.Equal(t, []spaceinfo.LocalStatus{spaceinfo.LocalStatusUnknown}, fx.view.localStatuses())
	assert.Equal(t, []string{"aclHead1"}, fx.joinCalls)
}
