package participantwatcher

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies/mock_dependencies"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
)

const testSpaceId = "space1"

type watcherFixture struct {
	*participantWatcher
	objectStore     *objectstore.StoreFixture
	space           *mock_clientspace.MockSpace
	techSpaceMock   *mock_techspace.MockTechSpace
	identityService *mock_dependencies.MockIdentityService
	myKeys          *accountdata.AccountKeys
}

func newWatcherFixture(t *testing.T) *watcherFixture {
	ctrl := gomock.NewController(t)
	accountService := mock_accountservice.NewMockService(ctrl)
	myKeys, err := accountdata.NewRandom()
	require.NoError(t, err)
	accountService.EXPECT().Account().Return(myKeys).AnyTimes()

	store := objectstore.NewStoreFixture(t)

	space := mock_clientspace.NewMockSpace(t)
	space.EXPECT().Id().Return(testSpaceId).Maybe()
	space.EXPECT().GetTypeIdByKey(mock.Anything, bundle.TypeKeyParticipant).Return("participantTypeId", nil).Maybe()

	techSpaceMock := mock_techspace.NewMockTechSpace(t)
	identityService := mock_dependencies.NewMockIdentityService(t)

	return &watcherFixture{
		participantWatcher: &participantWatcher{
			accountService:    accountService,
			objectStore:       store,
			techSpace:         techSpaceMock,
			identityService:   identityService,
			addedParticipants: map[string]struct{}{},
		},
		objectStore:     store,
		space:           space,
		techSpaceMock:   techSpaceMock,
		identityService: identityService,
		myKeys:          myKeys,
	}
}

func newAccState(t *testing.T, permissions aclrecordproto.AclUserPermissions, status list.AclStatus) (list.AccountState, crypto.PubKey) {
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	return list.AccountState{
		PubKey:      keys.SignKey.GetPublic(),
		Permissions: list.AclPermissions(permissions),
		Status:      status,
	}, keys.SignKey.GetPublic()
}

func (fx *watcherFixture) participantDetails(t *testing.T, id string) *domain.Details {
	details, err := fx.objectStore.SpaceIndex(testSpaceId).GetDetails(id)
	require.NoError(t, err)
	return details
}

func (fx *watcherFixture) fulltextQueueIds(t *testing.T) []string {
	queued, err := fx.objectStore.ListIdsFromFullTextQueue([]string{testSpaceId}, 100)
	require.NoError(t, err)
	ids := make([]string, 0, len(queued))
	for _, obj := range queued {
		ids = append(ids, obj.ObjectId)
	}
	return ids
}

func TestUpdateParticipantFromAclState(t *testing.T) {
	t.Run("creates store record with base and acl details", func(t *testing.T) {
		// given
		fx := newWatcherFixture(t)
		accState, pubKey := newAccState(t, aclrecordproto.AclUserPermissions_Writer, list.StatusActive)
		id := domain.NewParticipantId(testSpaceId, pubKey.Account())
		want := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:                     domain.String(id),
			bundle.RelationKeyIdentity:               domain.String(pubKey.Account()),
			bundle.RelationKeySpaceId:                domain.String(testSpaceId),
			bundle.RelationKeyLastModifiedBy:         domain.String(id),
			bundle.RelationKeyParticipantPermissions: domain.Int64(model.ParticipantPermissions_Writer),
			bundle.RelationKeyParticipantStatus:      domain.Int64(model.ParticipantStatus_Active),
			bundle.RelationKeyIsHiddenDiscovery:      domain.Bool(false),
			bundle.RelationKeyType:                   domain.String("participantTypeId"),
			bundle.RelationKeyCreator:                domain.String(addr.AnytypeProfileId),
			bundle.RelationKeyResolvedLayout:         domain.Int64(model.ObjectType_participant),
			bundle.RelationKeyLayoutAlign:            domain.Int64(model.Block_AlignCenter),
			bundle.RelationKeyIsReadonly:             domain.Bool(true),
			bundle.RelationKeyIsArchived:             domain.Bool(false),
			bundle.RelationKeyIsHidden:               domain.Bool(false),
		})

		// when
		err := fx.UpdateParticipantFromAclState(context.Background(), fx.space, accState)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, fx.participantDetails(t, id))
		assert.Equal(t, []string{id}, fx.fulltextQueueIds(t))
	})

	t.Run("my own participant gets identityProfileLink", func(t *testing.T) {
		// given
		fx := newWatcherFixture(t)
		fx.techSpaceMock.EXPECT().AccountObjectId().Return("accountObjectId", nil)
		accState := list.AccountState{
			PubKey:      fx.myKeys.SignKey.GetPublic(),
			Permissions: list.AclPermissions(aclrecordproto.AclUserPermissions_Owner),
			Status:      list.StatusActive,
		}
		id := domain.NewParticipantId(testSpaceId, fx.myKeys.SignKey.GetPublic().Account())

		// when
		err := fx.UpdateParticipantFromAclState(context.Background(), fx.space, accState)

		// then
		require.NoError(t, err)
		details := fx.participantDetails(t, id)
		assert.Equal(t, "accountObjectId", details.GetString(bundle.RelationKeyIdentityProfileLink))
		assert.Equal(t, int64(model.ParticipantPermissions_Owner), details.GetInt64(bundle.RelationKeyParticipantPermissions))
	})

	t.Run("non-active status is hidden from discovery", func(t *testing.T) {
		// given
		fx := newWatcherFixture(t)
		accState, pubKey := newAccState(t, aclrecordproto.AclUserPermissions_Reader, list.StatusRemoving)
		id := domain.NewParticipantId(testSpaceId, pubKey.Account())

		// when
		err := fx.UpdateParticipantFromAclState(context.Background(), fx.space, accState)

		// then
		require.NoError(t, err)
		details := fx.participantDetails(t, id)
		assert.True(t, details.GetBool(bundle.RelationKeyIsHiddenDiscovery))
		assert.Equal(t, int64(model.ParticipantStatus_Removing), details.GetInt64(bundle.RelationKeyParticipantStatus))
	})

	t.Run("identical update does not enqueue fulltext again", func(t *testing.T) {
		// given
		fx := newWatcherFixture(t)
		accState, _ := newAccState(t, aclrecordproto.AclUserPermissions_Writer, list.StatusActive)
		require.NoError(t, fx.UpdateParticipantFromAclState(context.Background(), fx.space, accState))
		require.NoError(t, fx.objectStore.ClearFullTextQueue(nil))

		// when
		err := fx.UpdateParticipantFromAclState(context.Background(), fx.space, accState)

		// then
		require.NoError(t, err)
		assert.Empty(t, fx.fulltextQueueIds(t))
	})

	t.Run("watch persisted participants registers identities from store", func(t *testing.T) {
		// given
		fx := newWatcherFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(domain.NewParticipantId(testSpaceId, "identity1")),
				bundle.RelationKeyIdentity:       domain.String("identity1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
			},
			{
				bundle.RelationKeyId:             domain.String(domain.NewParticipantId(testSpaceId, "identity2")),
				bundle.RelationKeyIdentity:       domain.String("identity2"),
				bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
			},
			{
				bundle.RelationKeyId:             domain.String("ordinaryObject"),
				bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_basic),
			},
		})
		fx.identityService.EXPECT().RegisterIdentity(testSpaceId, "identity1", nil).Return(nil).Once()
		fx.identityService.EXPECT().RegisterIdentity(testSpaceId, "identity2", nil).Return(nil).Once()

		// when
		err := fx.WatchPersistedParticipants(context.Background(), fx.space)
		// second call is a no-op: identities are already tracked
		errAgain := fx.WatchPersistedParticipants(context.Background(), fx.space)

		// then
		require.NoError(t, err)
		require.NoError(t, errAgain)
	})

	t.Run("watch persisted participants fails when a key is missing", func(t *testing.T) {
		// given
		fx := newWatcherFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String(domain.NewParticipantId(testSpaceId, "identity1")),
			bundle.RelationKeyIdentity:       domain.String("identity1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_participant),
		}})
		fx.identityService.EXPECT().RegisterIdentity(testSpaceId, "identity1", nil).Return(assert.AnError).Once()

		// when
		err := fx.WatchPersistedParticipants(context.Background(), fx.space)

		// then
		require.Error(t, err)
	})

	t.Run("processed acl head marker roundtrip", func(t *testing.T) {
		// given
		fx := newWatcherFixture(t)
		assert.Empty(t, fx.GetProcessedAclHeadId(context.Background(), fx.space))

		// when
		err := fx.SetProcessedAclHeadId(context.Background(), fx.space, "headId")

		// then
		require.NoError(t, err)
		assert.Equal(t, "headId", fx.GetProcessedAclHeadId(context.Background(), fx.space))
	})

	t.Run("permission change updates record but does not enqueue fulltext", func(t *testing.T) {
		// given
		fx := newWatcherFixture(t)
		accState, pubKey := newAccState(t, aclrecordproto.AclUserPermissions_Reader, list.StatusActive)
		id := domain.NewParticipantId(testSpaceId, pubKey.Account())
		require.NoError(t, fx.UpdateParticipantFromAclState(context.Background(), fx.space, accState))
		require.NoError(t, fx.objectStore.ClearFullTextQueue(nil))
		accState.Permissions = list.AclPermissions(aclrecordproto.AclUserPermissions_Writer)

		// when
		err := fx.UpdateParticipantFromAclState(context.Background(), fx.space, accState)

		// then
		require.NoError(t, err)
		details := fx.participantDetails(t, id)
		assert.Equal(t, int64(model.ParticipantPermissions_Writer), details.GetInt64(bundle.RelationKeyParticipantPermissions))
		assert.Empty(t, fx.fulltextQueueIds(t))
	})
}

func TestConvertPermissions_Admin(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Admin, convertPermissions(list.AclPermissionsAdmin))
}

func TestConvertPermissions_Owner(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Owner, convertPermissions(list.AclPermissionsOwner))
}

func TestConvertPermissions_Writer(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Writer, convertPermissions(list.AclPermissionsWriter))
}

func TestConvertPermissions_Reader(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Reader, convertPermissions(list.AclPermissionsReader))
}

func TestConvertPermissions_None(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_NoPermissions, convertPermissions(list.AclPermissionsNone))
}
