package aclnotifications

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"golang.org/x/net/context"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const Cname = "spaceNotification"

type NotificationSender interface {
	CreateAndSend(notification *model.Notification) error
}

type AclNotification interface {
	SendNotification(ctx context.Context, aclRecord *list.AclRecord, space clientspace.Space, permissions list.AclPermissions) error
}

type AclNotificationSender struct {
	app.Component
	identityService     dependencies.IdentityService
	notificationService NotificationSender
}

func NewAclNotificationSender() *AclNotificationSender {
	return &AclNotificationSender{}
}

func (n *AclNotificationSender) Init(a *app.App) (err error) {
	n.identityService = app.MustComponent[dependencies.IdentityService](a)
	n.notificationService = app.MustComponent[NotificationSender](a)
	return nil
}

func (n *AclNotificationSender) Name() (name string) {
	return Cname
}

func (n *AclNotificationSender) SendNotification(ctx context.Context, aclRecord *list.AclRecord, space clientspace.Space, permissions list.AclPermissions) error {
	if aclData, ok := aclRecord.Model.(*aclrecordproto.AclData); ok {
		for _, content := range aclData.AclContent {
			if err := n.sendJoinRequest(ctx, content, permissions, space); err != nil {
				return err
			}
			if err := n.sendParticipantRequestApprove(content, space); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *AclNotificationSender) sendJoinRequest(ctx context.Context,
	content *aclrecordproto.AclContentValue,
	permission list.AclPermissions,
	space clientspace.Space,
) error {
	if reqJoin := content.GetRequestJoin(); reqJoin != nil && permission.IsOwner() {
		pubKey, err := crypto.UnmarshalEd25519PublicKeyProto(reqJoin.InviteIdentity)
		if err != nil {
			return err
		}
		participantId := domain.NewParticipantId(space.Id(), pubKey.Account())
		identity, err := space.GetObjectWithTimeout(ctx, participantId)
		if err != nil {
			return err
		}
		name := pbtypes.GetString(identity.Details(), bundle.RelationKeyName.String())
		image := pbtypes.GetString(identity.Details(), bundle.RelationKeyIconImage.String())
		err = n.notificationService.CreateAndSend(&model.Notification{
			Id:      reqJoin.InviteRecordId,
			IsLocal: false,
			Payload: &model.NotificationPayloadOfRequestToJoin{RequestToJoin: &model.NotificationRequestToJoin{
				SpaceId:      space.Id(),
				Identity:     string(reqJoin.InviteIdentity),
				IdentityName: name,
				IdentityIcon: image,
			}},
			Space: space.Id(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *AclNotificationSender) sendParticipantRequestApprove(content *aclrecordproto.AclContentValue, space clientspace.Space) error {
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
					SpaceID:    space.Id(),
					Permission: mapProtoPermissionToAcl(reqApprove.Permissions),
				},
			},
			Space: space.Id(),
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
