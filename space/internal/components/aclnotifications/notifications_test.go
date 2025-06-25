package aclnotifications

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

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
		go f.processRecords()
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

		f.identityService.EXPECT().WaitProfile(context.Background(), pubKey.Account()).Return(&model.IdentityProfile{
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
		go f.processRecords()
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
		f.identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("test", nil, nil)

		// when
		go f.processRecords()
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
		f.identityService.EXPECT().GetMyProfileDetails(context.Background()).Return(pubKey.Account(), nil, nil)

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
		go f.processRecords()
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
		go f.processRecords()
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
		go f.processRecords()
		go f.Close(context.Background())

		// then
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
		f.identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("test", nil, nil)

		// when
		go f.processRecords()
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
		f.identityService.EXPECT().GetMyProfileDetails(context.Background()).Return(pubKey.Account(), nil, nil)

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
		go f.processRecords()
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
		f.identityService.EXPECT().GetMyProfileDetails(context.Background()).Return(pubKey.Account(), nil, nil)

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
		go f.processRecords()
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
		f.identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("test", nil, nil)

		// when
		go f.processRecords()
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
		go f.processRecords()
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
		go f.processRecords()
		go f.Close(context.Background())

		// then
		<-f.done
		f.notificationSender.AssertNotCalled(t, "CreateAndSend")
	})
}

func newFixture(t *testing.T) *fixture {
	sender := mock_aclnotifications.NewMockNotificationSender(t)
	identityService := mock_dependencies.NewMockIdentityService(t)
	n := &aclNotificationSender{
		batcher:             mb.New[*aclNotificationRecord](0),
		done:                make(chan struct{}),
		spaceNameGetter:     objectstore.NewStoreFixture(t),
		notificationService: sender,
		identityService:     identityService,
	}
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
