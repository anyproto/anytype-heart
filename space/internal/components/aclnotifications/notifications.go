package aclnotifications

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/cheggaaa/mb"
	"golang.org/x/net/context"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
)

const Cname = "spaceNotification"

var logger = logging.Logger("acl-notifications")

type aclNotificationRecord struct {
	record      list.AclRecord
	permissions list.AclPermissions
	spaceId     string
	aclId       string
}

type NotificationSender interface {
	CreateAndSend(notification *model.Notification) error
	GetLastNotificationId(acl string) string
}

type AclNotification interface {
	app.ComponentRunnable
	AddRecords(acl syncacl.SyncAcl, permissions list.AclPermissions, spaceId string)
}

type aclNotificationSender struct {
	identityService     dependencies.IdentityService
	notificationService NotificationSender
	batcher             *mb.MB
}

func NewAclNotificationSender() AclNotification {
	return &aclNotificationSender{batcher: mb.New(0)}
}

func (n *aclNotificationSender) Init(a *app.App) (err error) {
	n.identityService = app.MustComponent[dependencies.IdentityService](a)
	n.notificationService = app.MustComponent[NotificationSender](a)
	return nil
}

func (n *aclNotificationSender) Name() (name string) {
	return Cname
}

func (n *aclNotificationSender) Run(ctx context.Context) (err error) {
	go n.processRecords()
	return
}

func (n *aclNotificationSender) Close(ctx context.Context) (err error) {
	if err := n.batcher.Close(); err != nil {
		logger.Errorf("failed to close batcher, %s", err)
	}
	return
}

func (n *aclNotificationSender) AddRecords(acl syncacl.SyncAcl, permissions list.AclPermissions, spaceId string) {
	lastNotificationId := n.notificationService.GetLastNotificationId(acl.Id())
	if lastNotificationId != "" {
		acl.IterateFrom(lastNotificationId, func(record *list.AclRecord) (IsContinue bool) {
			err := n.batcher.Add(&aclNotificationRecord{
				record:      *record,
				permissions: permissions,
				spaceId:     spaceId,
				aclId:       acl.Id(),
			})
			if err != nil {
				logger.Errorf("failed to add acl record, %s", err)
			}
			return true
		})
		return
	}
	for _, record := range acl.Records() {
		err := n.batcher.Add(&aclNotificationRecord{
			record:      *record,
			permissions: permissions,
			spaceId:     spaceId,
			aclId:       acl.Id(),
		})
		if err != nil {
			logger.Errorf("failed to add acl record, %s", err)
		}
	}
}

func (n *aclNotificationSender) sendNotification(ctx context.Context, aclNotificationRecord *aclNotificationRecord) error {
	if aclData, ok := aclNotificationRecord.record.Model.(*aclrecordproto.AclData); ok {
		err := n.iterateAclContent(ctx, aclNotificationRecord, aclData)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *aclNotificationSender) processRecords() {
	for {
		msgs := n.batcher.Wait()
		if len(msgs) == 0 {
			return
		}
		for _, msg := range msgs {
			record, ok := msg.(*aclNotificationRecord)
			if !ok {
				continue
			}
			err := n.sendNotification(context.Background(), record)
			if err != nil {
				return
			}
		}
	}
}

func (n *aclNotificationSender) iterateAclContent(ctx context.Context,
	aclNotificationRecord *aclNotificationRecord,
	aclData *aclrecordproto.AclData,
) error {
	for _, content := range aclData.AclContent {
		if aclNotificationRecord.permissions.CanManageAccounts() {
			err := n.handleOwnerNotifications(ctx, aclNotificationRecord, content)
			if err != nil {
				return err
			}
		}
		if reqApprove := content.GetRequestAccept(); reqApprove != nil {
			if err := n.sendParticipantRequestApprove(reqApprove, aclNotificationRecord); err != nil {
				return err

			}
		}
	}
	return nil
}

func (n *aclNotificationSender) handleOwnerNotifications(ctx context.Context, aclNotificationRecord *aclNotificationRecord, content *aclrecordproto.AclContentValue) error {
	if reqJoin := content.GetRequestJoin(); reqJoin != nil {
		if err := n.sendJoinRequest(ctx, reqJoin, aclNotificationRecord); err != nil {
			return err
		}
	}
	if reqLeave := content.GetAccountRemove(); reqLeave != nil {
		if err := n.sendAccountRemove(ctx, aclNotificationRecord); err != nil {
			return err
		}
	}
	return nil
}

func (n *aclNotificationSender) sendJoinRequest(ctx context.Context,
	reqJoin *aclrecordproto.AclAccountRequestJoin,
	notificationRecord *aclNotificationRecord,
) error {
	var name, iconCid string
	pubKey, err := crypto.UnmarshalEd25519PublicKeyProto(reqJoin.InviteIdentity)
	if err != nil {
		return err
	}
	profile := n.identityService.WaitProfile(ctx, pubKey.Account())
	if profile != nil {
		name = profile.Name
		iconCid = profile.IconCid
	}
	err = n.notificationService.CreateAndSend(&model.Notification{
		Id:      notificationRecord.record.Id,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfRequestToJoin{RequestToJoin: &model.NotificationRequestToJoin{
			SpaceId:      notificationRecord.spaceId,
			Identity:     pubKey.Account(),
			IdentityName: name,
			IdentityIcon: iconCid,
		}},
		Space:     notificationRecord.spaceId,
		AclHeadId: notificationRecord.aclId,
	})
	if err != nil {
		return err
	}
	return nil
}

func (n *aclNotificationSender) sendParticipantRequestApprove(reqApprove *aclrecordproto.AclAccountRequestAccept,
	notificationRecord *aclNotificationRecord,
) error {
	identity, _, _ := n.identityService.GetMyProfileDetails()
	pubKey, err := crypto.UnmarshalEd25519PublicKeyProto(reqApprove.Identity)
	if err != nil {
		return err
	}
	account := pubKey.Account()
	if account != identity {
		return nil
	}
	err = n.notificationService.CreateAndSend(&model.Notification{
		Id:      notificationRecord.record.Id,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfParticipantRequestApproved{
			ParticipantRequestApproved: &model.NotificationParticipantRequestApproved{
				SpaceID:    notificationRecord.spaceId,
				Permission: mapProtoPermissionToAcl(reqApprove.Permissions),
			},
		},
		Space:     notificationRecord.spaceId,
		AclHeadId: notificationRecord.aclId,
	})
	if err != nil {
		return err
	}
	return nil
}

func (n *aclNotificationSender) sendAccountRemove(ctx context.Context, aclNotificationRecord *aclNotificationRecord) error {
	var name, iconCid string
	profile := n.identityService.WaitProfile(ctx, aclNotificationRecord.record.Identity.Account())
	if profile != nil {
		name = profile.Name
		iconCid = profile.IconCid
	}
	err := n.notificationService.CreateAndSend(&model.Notification{
		Id:      aclNotificationRecord.record.Id,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfLeaveRequest{LeaveRequest: &model.NotificationLeaveRequest{
			SpaceId:      aclNotificationRecord.spaceId,
			Identity:     aclNotificationRecord.record.Identity.Account(),
			IdentityName: name,
			IdentityIcon: iconCid,
		}},
		Space:     aclNotificationRecord.spaceId,
		AclHeadId: aclNotificationRecord.aclId,
	})
	if err != nil {
		return err
	}
	return nil
}

func mapProtoPermissionToAcl(permissions aclrecordproto.AclUserPermissions) model.ParticipantPermissions {
	switch permissions {
	case aclrecordproto.AclUserPermissions_Owner:
		return model.ParticipantPermissions_Owner
	case aclrecordproto.AclUserPermissions_None:
		return model.ParticipantPermissions_NoPermissions
	case aclrecordproto.AclUserPermissions_Writer:
		return model.ParticipantPermissions_Writer
	case aclrecordproto.AclUserPermissions_Reader:
		return model.ParticipantPermissions_Reader
	case aclrecordproto.AclUserPermissions_Admin:
		return model.ParticipantPermissions_Owner
	}
	return model.ParticipantPermissions_Reader
}
