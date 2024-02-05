package aclnotifications

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"golang.org/x/net/context"

	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const Cname = "spaceNotification"

type AclNotification interface {
	SendNotification(aclRecord *list.AclRecord, spaceId string) error
}

type AclNotificationSender struct {
	app.Component
	identityService     dependencies.IdentityService
	notificationService notifications.Notifications
}

func NewAclNotificationSender() *AclNotificationSender {
	return &AclNotificationSender{}
}

func (n *AclNotificationSender) Init(a *app.App) (err error) {
	n.identityService = app.MustComponent[dependencies.IdentityService](a)
	n.notificationService = app.MustComponent[notifications.Notifications](a)
	return nil
}

func (n *AclNotificationSender) Name() (name string) {
	return Cname
}

func (n *AclNotificationSender) SendNotification(aclRecord *list.AclRecord, spaceId string) error {
	_, _, details := n.identityService.GetMyProfileDetails()
	permission := model.ParticipantPermissions(pbtypes.GetInt64(details, bundle.RelationKeyParticipantPermissions.String()))
	if aclData, ok := aclRecord.Model.(*aclrecordproto.AclData); ok {
		for _, content := range aclData.AclContent {
			if err := n.sendJoinRequest(content, permission, spaceId); err != nil {
				return err
			}
			if err := n.sendParticipantRequestApprove(content, spaceId); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *AclNotificationSender) sendJoinRequest(content *aclrecordproto.AclContentValue, permission model.ParticipantPermissions, spaceId string) error {
	if reqJoin := content.GetRequestJoin(); reqJoin != nil && permission == model.ParticipantPermissions_Owner {
		details, err := n.identityService.GetDetails(context.Background(), string(reqJoin.InviteIdentity))
		if err != nil {
			return err
		}
		name := pbtypes.GetString(details, bundle.RelationKeyName.String())
		image := pbtypes.GetString(details, bundle.RelationKeyIconImage.String())
		err = n.notificationService.CreateAndSend(&model.Notification{
			Id:      reqJoin.InviteRecordId,
			IsLocal: false,
			Payload: &model.NotificationPayloadOfRequestToJoin{RequestToJoin: &model.NotificationRequestToJoin{
				SpaceId:      spaceId,
				Identity:     string(reqJoin.InviteIdentity),
				IdentityName: name,
				IdentityIcon: image,
			}},
			Space: spaceId,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *AclNotificationSender) sendParticipantRequestApprove(content *aclrecordproto.AclContentValue, spaceId string) error {
	if reqApprove := content.GetRequestAccept(); reqApprove != nil {
		identity, _, _ := n.identityService.GetMyProfileDetails()
		if string(reqApprove.Identity) != identity {
			return nil
		}
		err := n.notificationService.CreateAndSend(&model.Notification{
			// TODO Id:      reqApprove.RequestRecordId,
			IsLocal: false,
			Payload: &model.NotificationPayloadOfParticipantRequestApproved{
				ParticipantRequestApproved: &model.NotificationParticipantRequestApproved{
					SpaceID:    spaceId,
					Permission: mapProtoPermissionToAcl(reqApprove.Permissions),
				},
			},
			Space: spaceId,
		})
		if err != nil {
			return err
		}
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
