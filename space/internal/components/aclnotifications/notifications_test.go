package aclnotifications

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/aclnotifications/mock_aclnotifications"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies/mock_dependencies"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func TestAclNotificationSender_AddRecords(t *testing.T) {
	t.Run("join notification, user not owner", func(t *testing.T) {
		// given
		f := newFixture(t)
		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_RequestJoin{
			RequestJoin: &aclrecordproto.AclAccountRequestJoin{},
		}}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:    "recordId",
				Model: aclData,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusActive, 0)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertNotCalled(t, "CreateAndSend")
	})
	t.Run("join notification, user owner", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_RequestJoin{
			RequestJoin: &aclrecordproto.AclAccountRequestJoin{
				InviteIdentity: pubKeyRaw,
			},
		}}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:    "recordId",
				Model: aclData,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, 0)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)

		f.identityService.EXPECT().WaitProfile(f.ctx, pubKey.Account()).Return(&model.IdentityProfile{
			Name:    "test",
			IconCid: "test",
		})

		f.notificationSender.EXPECT().CreateAndSend(&model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfRequestToJoin{
				RequestToJoin: &model.NotificationRequestToJoin{
					SpaceId:      "spaceId",
					Identity:     pubKey.Account(),
					IdentityName: "test",
					IdentityIcon: "test",
				},
			},
			Space: "spaceId",
		}).Return(nil)
		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertCalled(t, "CreateAndSend", &model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfRequestToJoin{
				RequestToJoin: &model.NotificationRequestToJoin{
					SpaceId:      "spaceId",
					Identity:     pubKey.Account(),
					IdentityName: "test",
					IdentityIcon: "test",
				},
			},
			Space: "spaceId",
		})
	})
	t.Run("remove member notification", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_AccountRemove{
			AccountRemove: &aclrecordproto.AclAccountRemove{
				Identities: [][]byte{pubKeyRaw},
			},
		}}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:    "recordId",
				Model: aclData,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		f.identityService.EXPECT().GetMyProfileDetails(f.ctx).Return("test", nil, nil)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertNotCalled(t, "CreateAndSend")
	})
	t.Run("remove member notification - current user was removed and space is loaded", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_AccountRemove{
			AccountRemove: &aclrecordproto.AclAccountRemove{
				Identities: [][]byte{pubKeyRaw},
			},
		}}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:    "recordId",
				Model: aclData,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		f.identityService.EXPECT().GetMyProfileDetails(f.ctx).Return(pubKey.Account(), nil, nil)

		f.notificationSender.EXPECT().CreateAndSend(&model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantRemove{
				ParticipantRemove: &model.NotificationParticipantRemove{
					SpaceId:  "spaceId",
					Identity: pubKey.Account(),
				},
			},
			Space: "spaceId",
		}).Return(nil)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertCalled(t, "CreateAndSend", &model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantRemove{
				ParticipantRemove: &model.NotificationParticipantRemove{
					SpaceId:  "spaceId",
					Identity: pubKey.Account(),
				},
			},
			Space: "spaceId",
		})
	})
	t.Run("remove member notification - current user was removed, but space is offloaded", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_AccountRemove{
			AccountRemove: &aclrecordproto.AclAccountRemove{
				Identities: [][]byte{pubKeyRaw},
			},
		}}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:    "recordId",
				Model: aclData,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusUnknown)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertNotCalled(t, "CreateAndSend")
	})
	t.Run("leave space notification, user not owner", func(t *testing.T) {
		// given
		f := newFixture(t)
		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_AccountRequestRemove{
			AccountRequestRemove: &aclrecordproto.AclAccountRequestRemove{},
		}}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:    "recordId",
				Model: aclData,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusActive, 0)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertNotCalled(t, "CreateAndSend")
	})
	t.Run("change permissions notification not for current user", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_PermissionChanges{
			PermissionChanges: &aclrecordproto.AclAccountPermissionChanges{
				Changes: []*aclrecordproto.AclAccountPermissionChange{
					{
						Identity:    pubKeyRaw,
						Permissions: aclrecordproto.AclUserPermissions_Reader,
					},
				},
			},
		}}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:       "recordId",
				Model:    aclData,
				Identity: pubKey,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusActive, 0)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		f.identityService.EXPECT().GetMyProfileDetails(f.ctx).Return("test", nil, nil)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertNotCalled(t, "CreateAndSend")
	})
	t.Run("change permissions notification for current user", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_PermissionChanges{
			PermissionChanges: &aclrecordproto.AclAccountPermissionChanges{
				Changes: []*aclrecordproto.AclAccountPermissionChange{
					{
						Identity:    pubKeyRaw,
						Permissions: aclrecordproto.AclUserPermissions_Reader,
					},
				},
			},
		}}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:       "recordId",
				Model:    aclData,
				Identity: pubKey,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusActive, 0)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		f.identityService.EXPECT().GetMyProfileDetails(f.ctx).Return(pubKey.Account(), nil, nil)

		f.notificationSender.EXPECT().CreateAndSend(&model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantPermissionsChange{
				ParticipantPermissionsChange: &model.NotificationParticipantPermissionsChange{
					SpaceId:     "spaceId",
					Permissions: model.ParticipantPermissions_Reader,
				},
			},
			Space: "spaceId",
		}).Return(nil)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertCalled(t, "CreateAndSend", &model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantPermissionsChange{
				ParticipantPermissionsChange: &model.NotificationParticipantPermissionsChange{
					SpaceId:     "spaceId",
					Permissions: model.ParticipantPermissions_Reader,
				},
			},
			Space: "spaceId",
		})
	})
	t.Run("join request approved notification for current user", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_RequestAccept{
			RequestAccept: &aclrecordproto.AclAccountRequestAccept{
				Identity:    pubKeyRaw,
				Permissions: aclrecordproto.AclUserPermissions_Reader,
			},
		},
		}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:       "recordId",
				Model:    aclData,
				Identity: pubKey,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusActive, 0)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		f.identityService.EXPECT().GetMyProfileDetails(f.ctx).Return(pubKey.Account(), nil, nil)

		f.notificationSender.EXPECT().CreateAndSend(&model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantRequestApproved{
				ParticipantRequestApproved: &model.NotificationParticipantRequestApproved{
					SpaceId:     "spaceId",
					Permissions: model.ParticipantPermissions_Reader,
				},
			},
			Space: "spaceId",
		}).Return(nil)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertCalled(t, "CreateAndSend", &model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantRequestApproved{
				ParticipantRequestApproved: &model.NotificationParticipantRequestApproved{
					SpaceId:     "spaceId",
					Permissions: model.ParticipantPermissions_Reader,
				},
			},
			Space: "spaceId",
		})
	})
	t.Run("join request approved notification not for current user", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_RequestAccept{
			RequestAccept: &aclrecordproto.AclAccountRequestAccept{
				Identity:    pubKeyRaw,
				Permissions: aclrecordproto.AclUserPermissions_Reader,
			},
		},
		}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &aclListStub{records: []*list.AclRecord{
			{
				Id:       "recordId",
				Model:    aclData,
				Identity: pubKey,
			},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusActive, 0)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		f.identityService.EXPECT().GetMyProfileDetails(f.ctx).Return("test", nil, nil)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertNotCalled(t, "CreateAndSend")
	})
}

func TestAclNotificationSender_AddSingleRecord(t *testing.T) {
	t.Run("join request declined notification for current user", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_RequestDecline{
			RequestDecline: &aclrecordproto.AclAccountRequestDecline{},
		},
		}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &list.AclRecord{
			Id:       "recordId",
			Identity: pubKey,
			Model:    aclData,
		}

		f.AddSingleRecord("aclId", acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusDeleted)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)

		f.notificationSender.EXPECT().CreateAndSend(&model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantRequestDecline{
				ParticipantRequestDecline: &model.NotificationParticipantRequestDecline{
					SpaceId: "spaceId",
				},
			},
			Space:     "spaceId",
			AclHeadId: "aclId",
		}).Return(nil)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertCalled(t, "CreateAndSend", &model.Notification{
			Id:      "recordId",
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantRequestDecline{
				ParticipantRequestDecline: &model.NotificationParticipantRequestDecline{
					SpaceId: "spaceId",
				},
			},
			Space:     "spaceId",
			AclHeadId: "aclId",
		})
	})
	t.Run("join request declined notification not for current user", func(t *testing.T) {
		// given
		f := newFixture(t)
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		assert.Nil(t, err)

		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_RequestDecline{
			RequestDecline: &aclrecordproto.AclAccountRequestDecline{},
		},
		}
		aclData := &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}
		acl := &list.AclRecord{
			Id:       "recordId",
			Identity: pubKey,
			Model:    aclData,
		}

		f.AddSingleRecord("aclId", acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusActive)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)

		// when
		require.NoError(t, f.Run(context.Background()))
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertNotCalled(t, "CreateAndSend")
	})
}

func TestAclNotificationSender_Close(t *testing.T) {
	t.Run("returns when the notification service never finishes loading", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.closeGrace = 50 * time.Millisecond
		f.notificationSender.EXPECT().LoadFinish().Return(make(chan struct{}))
		require.NoError(t, f.Run(context.Background()))

		// when, then
		requireReturns(t, func() { _ = f.Close(context.Background()) })
	})
	t.Run("returns when a queued record blocks on the identity service", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.closeGrace = 50 * time.Millisecond
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		require.NoError(t, err)
		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_AccountRemove{
			AccountRemove: &aclrecordproto.AclAccountRemove{Identities: [][]byte{pubKeyRaw}},
		}}
		acl := &aclListStub{records: []*list.AclRecord{
			{Id: "recordId", Model: &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsWriter, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		// queued behind it, and would be sent if the worker carried on after Close
		f.AddSingleRecord("aclId", declineRecord(), 0, "spaceId", spaceinfo.AccountStatusDeleted)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		// the identity service never got to run, so it never answers
		entered := make(chan struct{})
		f.identityService.EXPECT().GetMyProfileDetails(f.ctx).
			RunAndReturn(func(ctx context.Context) (string, crypto.SymKey, *domain.Details) {
				close(entered)
				<-ctx.Done()
				return "", nil, nil
			})
		require.NoError(t, f.Run(context.Background()))
		requireReturns(t, func() { <-entered })

		// when, then
		requireReturns(t, func() { _ = f.Close(context.Background()) })
		f.notificationSender.AssertNumberOfCalls(t, "CreateAndSend", 0)
	})
	t.Run("returns when a join request waits for a profile that never arrives", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.closeGrace = 50 * time.Millisecond
		_, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		pubKeyRaw, err := pubKey.Marshall()
		require.NoError(t, err)
		aclRecord := &aclrecordproto.AclContentValue{Value: &aclrecordproto.AclContentValue_RequestJoin{
			RequestJoin: &aclrecordproto.AclAccountRequestJoin{InviteIdentity: pubKeyRaw},
		}}
		acl := &aclListStub{records: []*list.AclRecord{
			{Id: "recordId", Model: &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{aclRecord}}},
		}}
		f.notificationSender.EXPECT().GetLastNotificationId(mock.Anything).Return("")
		f.AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)

		loadChan := make(chan struct{})
		close(loadChan)
		f.notificationSender.EXPECT().LoadFinish().Return(loadChan)
		// the requester's profile is not cached and never arrives
		entered := make(chan struct{})
		f.identityService.EXPECT().WaitProfile(f.ctx, pubKey.Account()).
			RunAndReturn(func(ctx context.Context, _ string) *model.IdentityProfile {
				close(entered)
				<-ctx.Done()
				return nil
			})
		// saved without a name, as an account stop always has: holding it back
		// would leave it to block the worker again on every start
		f.notificationSender.EXPECT().CreateAndSend(mock.MatchedBy(func(n *model.Notification) bool {
			return n.GetRequestToJoin().GetIdentity() == pubKey.Account() && n.GetRequestToJoin().GetIdentityName() == ""
		})).Return(nil)
		require.NoError(t, f.Run(context.Background()))
		requireReturns(t, func() { <-entered })

		// when, then
		requireReturns(t, func() { _ = f.Close(context.Background()) })
	})
	t.Run("returns when Run was never called", func(t *testing.T) {
		f := newFixture(t)
		requireReturns(t, func() { _ = f.Close(context.Background()) })
	})
}

// declineRecord is a join-request rejection, which the worker sends without
// waiting on any other service.
func declineRecord() *list.AclRecord {
	return &list.AclRecord{Id: "declineId", Model: &aclrecordproto.AclData{AclContent: []*aclrecordproto.AclContentValue{
		{Value: &aclrecordproto.AclContentValue_RequestDecline{RequestDecline: &aclrecordproto.AclAccountRequestDecline{}}},
	}}}
}

// requireReturns fails the test if fn has not returned within a few seconds.
func requireReturns(t *testing.T, fn func()) {
	t.Helper()
	returned := make(chan struct{})
	go func() {
		fn()
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("did not return in 5s")
	}
}

func newFixture(t *testing.T) *fixture {
	sender := mock_aclnotifications.NewMockNotificationSender(t)
	identityService := mock_dependencies.NewMockIdentityService(t)
	n := NewAclNotificationSender().(*aclNotificationSender)
	n.spaceNameGetter = objectstore.NewStoreFixture(t)
	n.notificationService = sender
	n.identityService = identityService
	fx := &fixture{
		aclNotificationSender: n,
		notificationSender:    sender,
		identityService:       identityService,
	}
	return fx
}

type fixture struct {
	*aclNotificationSender
	notificationSender *mock_aclnotifications.MockNotificationSender
	identityService    *mock_dependencies.MockIdentityService
}

type aclListStub struct {
	records []*list.AclRecord
}

func (a *aclListStub) Lock() {}

func (a *aclListStub) Unlock() {}

func (a *aclListStub) RLock() {}

func (a *aclListStub) RUnlock() {}

func (a *aclListStub) Id() string { return "" }

func (a *aclListStub) Root() *consensusproto.RawRecordWithId { return nil }

func (a *aclListStub) Records() []*list.AclRecord {
	return a.records
}

func (a *aclListStub) AclState() *list.AclState { return nil }

func (a *aclListStub) IsAfter(first string, second string) (bool, error) { return false, nil }

func (a *aclListStub) HasHead(head string) bool { return false }

func (a *aclListStub) Head() *list.AclRecord { return nil }

func (a *aclListStub) RecordsAfter(ctx context.Context, id string) (records []*consensusproto.RawRecordWithId, err error) {
	return nil, nil
}

func (a *aclListStub) RecordsBefore(ctx context.Context, headId string) (records []*consensusproto.RawRecordWithId, err error) {
	return nil, nil
}

func (a *aclListStub) Get(id string) (*list.AclRecord, error) {
	return nil, nil
}

func (a *aclListStub) GetIndex(idx int) (*list.AclRecord, error) {
	return nil, nil
}

func (a *aclListStub) Iterate(iterFunc list.IterFunc) {}

func (a *aclListStub) IterateFrom(startId string, iterFunc list.IterFunc) {
	startIdx := len(a.records)
	for i, record := range a.records {
		if record.Id == startId {
			startIdx = i
		}
		if i >= startIdx {
			iterFunc(record)
		}
	}
	return
}

func (a *aclListStub) KeyStorage() crypto.KeyStorage { return nil }

func (a *aclListStub) RecordBuilder() list.AclRecordBuilder {
	return nil
}

func (a *aclListStub) ValidateRawRecord(rawRec *consensusproto.RawRecord, afterValid func(state *list.AclState) error) (err error) {
	return err
}

func (a *aclListStub) AddRawRecord(rawRec *consensusproto.RawRecordWithId) (err error) { return err }

func (a *aclListStub) AddRawRecords(rawRecords []*consensusproto.RawRecordWithId) (err error) {
	return err
}

func (a *aclListStub) Close(ctx context.Context) (err error) { return err }

func (a *aclListStub) GetRecordIndex(recordId string) (idx int) {
	return
}

func TestMapProtoPermissionToAcl_Admin(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Admin, mapProtoPermissionToAcl(aclrecordproto.AclUserPermissions_Admin))
}

func TestMapProtoPermissionToAcl_Owner(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Owner, mapProtoPermissionToAcl(aclrecordproto.AclUserPermissions_Owner))
}

func TestMapProtoPermissionToAcl_Writer(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Writer, mapProtoPermissionToAcl(aclrecordproto.AclUserPermissions_Writer))
}

func TestMapProtoPermissionToAcl_Reader(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Reader, mapProtoPermissionToAcl(aclrecordproto.AclUserPermissions_Reader))
}

func TestMapProtoPermissionToAcl_None(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_NoPermissions, mapProtoPermissionToAcl(aclrecordproto.AclUserPermissions_None))
}
