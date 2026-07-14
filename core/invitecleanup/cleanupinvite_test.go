package invitecleanup

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore/mock_rpcstore"
	"github.com/anyproto/anytype-heart/core/invitestore"
	"github.com/anyproto/anytype-heart/core/invitestore/mock_invitestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/encode"
)

const testTechSpaceId = "techSpaceId"

// deleterStub records the file objects the cleanup asked to delete.
type deleterStub struct {
	deleted []domain.FullID
}

func (d *deleterStub) DeleteObjectByFullID(id domain.FullID) error {
	d.deleted = append(d.deleted, id)
	return nil
}

type fixture struct {
	*service
	inviteStore *mock_invitestore.MockService
	rpcStore    *mock_rpcstore.MockRpcStore
	fileObjects *mock_fileobject.MockService
	coordinator *mock_coordinatorclient.MockCoordinatorClient
	deleter     *deleterStub
	space       *mock_clientspace.MockSpace
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().TechSpaceId().Return(testTechSpaceId).Maybe()

	sp := mock_clientspace.NewMockSpace(t)
	sp.EXPECT().Id().Return(testSpaceId).Maybe()

	fx := &fixture{
		inviteStore: mock_invitestore.NewMockService(t),
		rpcStore:    mock_rpcstore.NewMockRpcStore(t),
		fileObjects: mock_fileobject.NewMockService(t),
		coordinator: mock_coordinatorclient.NewMockCoordinatorClient(ctrl),
		deleter:     &deleterStub{},
		space:       sp,
	}
	fx.service = &service{
		inviteStore:       fx.inviteStore,
		coordinatorClient: fx.coordinator,
		rpcStore:          fx.rpcStore,
		fileObjectService: fx.fileObjects,
		objectDeleter:     fx.deleter,
		spaceService:      spaceService,
	}
	return fx
}

// noFileObjects makes the file-object lookup come up empty, which is the normal case: invites never
// had file objects.
func (fx *fixture) noFileObjects() {
	fx.fileObjects.EXPECT().GetObjectDetailsByFileId(mock.Anything).
		Return("", nil, filemodels.ErrObjectNotFound).Maybe()
	fx.rpcStore.EXPECT().DeleteFiles(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
}

func (fx *fixture) fileOnNode(c cid.Cid, exists bool) {
	status := fileproto.AvailabilityStatus_NotExists
	if exists {
		status = fileproto.AvailabilityStatus_Exists
	}
	fx.rpcStore.EXPECT().CheckAvailability(mock.Anything, testSpaceId, []cid.Cid{c}).
		Return([]*fileproto.BlockAvailability{{Cid: c.Bytes(), Status: status}}, nil)
}

// storedInvite puts an invite file on the node and returns the candidate that points at it.
func (fx *fixture) storedInvite(t *testing.T, key inviteKey, payload *model.InvitePayload) inviteFile {
	t.Helper()
	raw, err := proto.Marshal(payload)
	require.NoError(t, err)
	fileKey, err := crypto.NewRandomAES()
	require.NoError(t, err)
	fileKeyRaw, err := encode.EncodeKeyToBase58(fileKey)
	require.NoError(t, err)

	c, err := cid.Parse("bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi")
	require.NoError(t, err)
	fx.inviteStore.EXPECT().GetInvite(mock.Anything, c, mock.Anything).
		Return(&model.Invite{Payload: raw}, nil).Maybe()
	fx.fileOnNode(c, true)
	return inviteFile{cid: c.String(), key: fileKeyRaw}
}

func TestCleanupInvite(t *testing.T) {
	ctx := context.Background()
	gone := workspaceView{liveCids: map[string]struct{}{}}

	t.Run("a revoked invite is deleted from the node", func(t *testing.T) {
		// given an invite the acl carries but no longer honours, which the workspace has dropped
		fx := newFixture(t)
		key := newInviteKey(t)
		file := fx.storedInvite(t, key, key.payload())
		fx.noFileObjects()
		fx.coordinator.EXPECT().
			AclDeleteInvite(gomock.Any(), testSpaceId, gomock.Any()).
			Return(nil)

		got, reason := fx.cleanupInvite(ctx, fx.space, revokedAcl(key), gone, file)

		require.NoError(t, reason)
		assert.Equal(t, dispositionResolved, got)
	})

	t.Run("an invite in use is never removed", func(t *testing.T) {
		// The invariant the whole design exists to hold. The coordinator mock has no AclDeleteInvite
		// expectation, so the test fails if the cleanup so much as tries.
		fx := newFixture(t)
		key := newInviteKey(t)
		live := workspaceView{liveCids: map[string]struct{}{}}
		file := fx.storedInvite(t, key, key.payload())
		live.liveCids[file.cid] = struct{}{}

		got, reason := fx.cleanupInvite(ctx, fx.space, liveAcl(key), live, file)

		assert.Equal(t, dispositionResolved, got)
		assert.ErrorIs(t, reason, errInviteLive)
		assert.Empty(t, fx.deleter.deleted, "the file object of an invite in use must not go either")
	})

	t.Run("an invite the acl has never seen is not deleted", func(t *testing.T) {
		// its record may have been accepted by the node without syncing back, so it may be live
		fx := newFixture(t)
		key, other := newInviteKey(t), newInviteKey(t)
		file := fx.storedInvite(t, key, key.payload())

		got, reason := fx.cleanupInvite(ctx, fx.space, revokedAcl(other), gone, file)

		assert.Equal(t, dispositionDefer, got)
		assert.ErrorIs(t, reason, errInviteUnknown)
	})

	t.Run("a file already gone from the node resolves without a fetch", func(t *testing.T) {
		fx := newFixture(t)
		c, err := cid.Parse("bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi")
		require.NoError(t, err)
		fileKey, err := crypto.NewRandomAES()
		require.NoError(t, err)
		raw, err := encode.EncodeKeyToBase58(fileKey)
		require.NoError(t, err)
		fx.fileOnNode(c, false)
		fx.noFileObjects()

		got, reason := fx.cleanupInvite(ctx, fx.space, aclView{isOwner: true}, gone, inviteFile{cid: c.String(), key: raw})

		require.NoError(t, reason)
		assert.Equal(t, dispositionResolved, got)
	})

	t.Run("an unreadable invite file is given up on, not retried forever", func(t *testing.T) {
		// an unreadable file never becomes readable. Deferring on it would rescan the space on every
		// launch for the rest of time.
		fx := newFixture(t)
		c, err := cid.Parse("bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi")
		require.NoError(t, err)
		fileKey, err := crypto.NewRandomAES()
		require.NoError(t, err)
		raw, err := encode.EncodeKeyToBase58(fileKey)
		require.NoError(t, err)
		fx.fileOnNode(c, true)
		fx.inviteStore.EXPECT().GetInvite(mock.Anything, c, mock.Anything).
			Return(nil, fmt.Errorf("%w: decrypt data", invitestore.ErrInviteUnreadable))

		got, _ := fx.cleanupInvite(ctx, fx.space, aclView{isOwner: true}, gone, inviteFile{cid: c.String(), key: raw})

		assert.Equal(t, dispositionSkip, got)
	})
}
