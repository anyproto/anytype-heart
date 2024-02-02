package notifications

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"

	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const Cname = "spaceNotification"

type AclNotification interface {
	SendNotification(aclRecord *list.AclRecord, spaceId string)
}

type notificationSender struct {
	identityService     dependencies.IdentityService
	notificationService notifications.Notifications
}

func newNotificationSender() AclNotification {
	return &notificationSender{}
}

func (n *notificationSender) Init(a *app.App) (err error) {
	n.identityService = app.MustComponent[dependencies.IdentityService](a)
	n.notificationService = app.MustComponent[notifications.Notifications](a)
	return nil
}

func (n *notificationSender) Name() (name string) {
	return Cname
}

func (n *notificationSender) SendNotification(aclRecord *list.AclRecord, spaceId string) {
	_, _, details := n.identityService.GetMyProfileDetails()
	permission := model.ParticipantPermissions(pbtypes.GetInt64(details, bundle.RelationKeyParticipantPermissions.String()))
	if aclData, ok := aclRecord.Model.(*aclrecordproto.AclData); ok {
		for _, content := range aclData.AclContent {
			if n.sendJoinRequest(content, permission, spaceId) {
				return
			}
		}
	}
}

func (n *notificationSender) sendJoinRequest(content *aclrecordproto.AclContentValue, permission model.ParticipantPermissions, spaceId string) bool {
	if reqJoin := content.GetRequestJoin(); reqJoin != nil && permission == model.ParticipantPermissions_Owner {
		identity := n.identityService.GetIdentity(string(reqJoin.InviteIdentity))
		err := n.notificationService.CreateAndSend(&model.Notification{
			Id:      reqJoin.InviteRecordId,
			IsLocal: false,
			Payload: &model.NotificationPayloadOfRequestToJoin{RequestToJoin: &model.NotificationRequestToJoin{
				SpaceId:      spaceId,
				Identity:     string(reqJoin.InviteIdentity),
				IdentityName: identity.Name,
				IdentityIcon: identity.IconCid,
			}},
			Space: spaceId,
		})
		if err != nil {
			return true
		}
	}
	return false
}
