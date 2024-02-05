package aclnotifications

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl"
	"github.com/anyproto/any-sync/util/crypto"
	"golang.org/x/net/context"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
)

const Cname = "spaceNotification"

type NotificationSender interface {
	CreateAndSend(notification *model.Notification) error
	GetLastNotificationId() string
}

type AclNotification interface {
	SendNotification(ctx context.Context, space clientspace.Space, permissions list.AclPermissions, acl syncacl.SyncAcl, fullScan bool) error
}

type AclNotificationSender struct {
	app.Component
	lastNotificationId  string
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

func (n *AclNotificationSender) SendNotification(ctx context.Context,
	space clientspace.Space,
	permissions list.AclPermissions,
	acl syncacl.SyncAcl,
	fullScan bool,
) error {
	lastNotificationId := n.notificationService.GetLastNotificationId()
	if !fullScan {
		return n.handleAclContent(ctx, acl.Head(), permissions, space)
	}
	var err error
	if lastNotificationId != "" {
		acl.IterateFrom(lastNotificationId, func(record *list.AclRecord) (IsContinue bool) {
			if err = n.handleAclContent(ctx, record, permissions, space); err != nil {
				return false
			}
			return true
		})
		return err
	}
	for _, record := range acl.Records() {
		if err = n.handleAclContent(ctx, record, permissions, space); err != nil {
			return err
		}
	}
	return nil
}

func (n *AclNotificationSender) handleAclContent(ctx context.Context,
	record *list.AclRecord,
	permissions list.AclPermissions,
	space clientspace.Space,
) error {
	if aclData, ok := record.Model.(*aclrecordproto.AclData); ok {
		for _, content := range aclData.AclContent {
			if err := n.sendJoinRequest(ctx, content, permissions, space, record.Id); err != nil {
				return err
			}
			if err := n.sendParticipantRequestApprove(content, space, record.Id); err != nil {
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
	id string,
) error {
	if reqJoin := content.GetRequestJoin(); reqJoin != nil && permission.IsOwner() {
		pubKey, err := crypto.UnmarshalEd25519PublicKeyProto(reqJoin.InviteIdentity)
		if err != nil {
			return err
		}
		identities, err := n.identityService.GetIdentitiesDataFromRepo(ctx, []string{pubKey.Account()})
		if err != nil {
			return err
		}
		var (
			name string
			icon string
		)
		if len(identities) != 0 {
			_, profile, err := n.identityService.GetProfile(identities[0])
			if err != nil {
				return err
			}
			name = profile.Name
			icon = profile.IconCid
		}
		err = n.notificationService.CreateAndSend(&model.Notification{
			Id:      id,
			IsLocal: false,
			Payload: &model.NotificationPayloadOfRequestToJoin{RequestToJoin: &model.NotificationRequestToJoin{
				SpaceId:      space.Id(),
				Identity:     string(reqJoin.InviteIdentity),
				IdentityName: name,
				IdentityIcon: icon,
			}},
			Space: space.Id(),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *AclNotificationSender) sendParticipantRequestApprove(content *aclrecordproto.AclContentValue, space clientspace.Space, id string) error {
	if reqApprove := content.GetRequestAccept(); reqApprove != nil {
		identity, _, _ := n.identityService.GetMyProfileDetails()
		if string(reqApprove.Identity) != identity {
			return nil
		}
		err := n.notificationService.CreateAndSend(&model.Notification{
			Id:      id,
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
