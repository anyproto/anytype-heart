package aclnotifications

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/cheggaaa/mb"
	"golang.org/x/net/context"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "common.components.aclnotifications"

var logger = logging.Logger("acl-notifications")

type aclNotificationRecord struct {
	record        list.AclRecord
	permissions   list.AclPermissions
	spaceId       string
	aclId         string
	accountStatus spaceinfo.AccountStatus
	spaceName     string
}

type NotificationSender interface {
	CreateAndSend(notification *model.Notification) error
	GetLastNotificationId(acl string) string
}

type AclNotification interface {
	app.ComponentRunnable
	AddRecords(acl list.AclList, permissions list.AclPermissions, spaceId string, accountStatus spaceinfo.AccountStatus)
}

type aclNotificationSender struct {
	identityService     dependencies.IdentityService
	notificationService NotificationSender
	batcher             *mb.MB
	objectStore         objectstore.ObjectStore
}

func NewAclNotificationSender() AclNotification {
	return &aclNotificationSender{batcher: mb.New(0)}
}

func (n *aclNotificationSender) Init(a *app.App) (err error) {
	n.identityService = app.MustComponent[dependencies.IdentityService](a)
	n.notificationService = app.MustComponent[NotificationSender](a)
	n.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (n *aclNotificationSender) Name() (name string) {
	return CName
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

func (n *aclNotificationSender) AddRecords(acl list.AclList,
	permissions list.AclPermissions,
	spaceId string,
	accountStatus spaceinfo.AccountStatus,
) {
	spaceName := n.getSpaceName(spaceId)
	lastNotificationId := n.notificationService.GetLastNotificationId(acl.Id())
	if lastNotificationId != "" {
		acl.IterateFrom(lastNotificationId, func(record *list.AclRecord) (IsContinue bool) {
			err := n.batcher.Add(&aclNotificationRecord{
				record:        *record,
				permissions:   permissions,
				spaceId:       spaceId,
				aclId:         acl.Id(),
				accountStatus: accountStatus,
				spaceName:     spaceName,
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
			record:        *record,
			permissions:   permissions,
			spaceId:       spaceId,
			aclId:         acl.Id(),
			accountStatus: accountStatus,
			spaceName:     spaceName,
		})
		if err != nil {
			logger.Errorf("failed to add acl record, %s", err)
		}
	}
}

func (n *aclNotificationSender) getSpaceName(spaceId string) string {
	records, _, err := n.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyTargetSpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
		},
	})
	if err != nil {
		logger.Errorf("failed to get details for acl record, %s", err)
	}
	var spaceName string
	if len(records) > 0 {
		spaceName = pbtypes.GetString(records[0].Details, bundle.RelationKeyName.String())
	}
	return spaceName
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
	for i, content := range aclData.AclContent {
		notificationId := provideNotificationId(aclNotificationRecord.record.Id, i)
		if aclNotificationRecord.permissions.CanManageAccounts() {
			err := n.handleOwnerNotifications(ctx, aclNotificationRecord, content, notificationId)
			if err != nil {
				return err
			}
		}
		err := n.handleSpaceMemberNotifications(ctx, aclNotificationRecord, content, notificationId)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *aclNotificationSender) handleSpaceMemberNotifications(ctx context.Context,
	aclNotificationRecord *aclNotificationRecord,
	content *aclrecordproto.AclContentValue,
	notificationId string,
) error {
	if reqApprove := content.GetRequestAccept(); reqApprove != nil {
		if err := n.sendParticipantRequestApprove(reqApprove, aclNotificationRecord, notificationId); err != nil {
			return err

		}
	}
	if accRemove := content.GetAccountRemove(); accRemove != nil {
		if err := n.sendAccountRemove(ctx, aclNotificationRecord, notificationId, accRemove.Identities); err != nil {
			return err
		}
	}
	if reqDecline := content.GetRequestDecline(); reqDecline != nil {
		if err := n.sendParticipantRequestDecline(aclNotificationRecord, notificationId); err != nil {
			return err
		}
	}
	if reqPermissionChanges := content.GetPermissionChanges(); reqPermissionChanges != nil {
		if err := n.sendParticipantPermissionChanges(reqPermissionChanges, aclNotificationRecord, notificationId); err != nil {
			return err
		}
	}
	return nil
}

func provideNotificationId(id string, i int) string {
	if i == 0 {
		return id
	}
	return fmt.Sprintf("%s%d", id, i)
}

func (n *aclNotificationSender) handleOwnerNotifications(ctx context.Context,
	aclNotificationRecord *aclNotificationRecord,
	content *aclrecordproto.AclContentValue,
	notificationId string,
) error {
	if reqJoin := content.GetRequestJoin(); reqJoin != nil {
		if err := n.sendJoinRequest(ctx, reqJoin, aclNotificationRecord, notificationId); err != nil {
			return err
		}
	}
	if reqLeave := content.GetAccountRequestRemove(); reqLeave != nil {
		if err := n.sendAccountRequestRemove(ctx, aclNotificationRecord, notificationId); err != nil {
			return err
		}
	}
	return nil
}

func (n *aclNotificationSender) sendJoinRequest(ctx context.Context,
	reqJoin *aclrecordproto.AclAccountRequestJoin,
	notificationRecord *aclNotificationRecord,
	notificationId string,
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
		Id:      notificationId,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfRequestToJoin{RequestToJoin: &model.NotificationRequestToJoin{
			SpaceId:      notificationRecord.spaceId,
			Identity:     pubKey.Account(),
			IdentityName: name,
			IdentityIcon: iconCid,
			SpaceName:    notificationRecord.spaceName,
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
	notificationId string,
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
		Id:      notificationId,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfParticipantRequestApproved{
			ParticipantRequestApproved: &model.NotificationParticipantRequestApproved{
				SpaceId:     notificationRecord.spaceId,
				Permissions: mapProtoPermissionToAcl(reqApprove.Permissions),
				SpaceName:   notificationRecord.spaceName,
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

func (n *aclNotificationSender) sendAccountRequestRemove(ctx context.Context,
	aclNotificationRecord *aclNotificationRecord,
	notificationId string,
) error {
	var name, iconCid string
	profile := n.identityService.WaitProfile(ctx, aclNotificationRecord.record.Identity.Account())
	if profile != nil {
		name = profile.Name
		iconCid = profile.IconCid
	}
	err := n.notificationService.CreateAndSend(&model.Notification{
		Id:      notificationId,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfRequestToLeave{RequestToLeave: &model.NotificationRequestToLeave{
			SpaceId:      aclNotificationRecord.spaceId,
			Identity:     aclNotificationRecord.record.Identity.Account(),
			IdentityName: name,
			IdentityIcon: iconCid,
			SpaceName:    aclNotificationRecord.spaceName,
		}},
		Space:     aclNotificationRecord.spaceId,
		AclHeadId: aclNotificationRecord.aclId,
	})
	if err != nil {
		return err
	}
	return nil
}

func (n *aclNotificationSender) sendAccountRemove(ctx context.Context,
	aclNotificationRecord *aclNotificationRecord,
	notificationId string,
	identities [][]byte,
) error {
	myProfile, _, _ := n.identityService.GetMyProfileDetails()
	found, err := n.isAccountRemoved(identities, myProfile)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	var name, iconCid string
	profile := n.identityService.WaitProfile(ctx, aclNotificationRecord.record.Identity.Account())
	if profile != nil {
		name = profile.Name
		iconCid = profile.IconCid
	}
	err = n.notificationService.CreateAndSend(&model.Notification{
		Id:      notificationId,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfParticipantRemove{ParticipantRemove: &model.NotificationParticipantRemove{
			SpaceId:      aclNotificationRecord.spaceId,
			Identity:     aclNotificationRecord.record.Identity.Account(),
			IdentityName: name,
			IdentityIcon: iconCid,
			SpaceName:    aclNotificationRecord.spaceName,
		}},
		Space:     aclNotificationRecord.spaceId,
		AclHeadId: aclNotificationRecord.aclId,
	})
	if err != nil {
		return err
	}
	return nil
}

func (n *aclNotificationSender) sendParticipantRequestDecline(aclNotificationRecord *aclNotificationRecord, notificationId string) error {
	if aclNotificationRecord.accountStatus != spaceinfo.AccountStatusDeleted {
		return nil
	}
	return n.notificationService.CreateAndSend(&model.Notification{
		Id:      notificationId,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfParticipantRequestDecline{
			ParticipantRequestDecline: &model.NotificationParticipantRequestDecline{
				SpaceId:   aclNotificationRecord.spaceId,
				SpaceName: aclNotificationRecord.spaceName,
			},
		},
		Space:     aclNotificationRecord.spaceId,
		AclHeadId: aclNotificationRecord.aclId,
	})
}

func (n *aclNotificationSender) sendParticipantPermissionChanges(reqPermissionChanges *aclrecordproto.AclAccountPermissionChanges,
	aclNotificationRecord *aclNotificationRecord,
	notificationId string,
) error {
	var (
		accountFound bool
		err          error
		permissions  aclrecordproto.AclUserPermissions
	)
	myProfile, _, _ := n.identityService.GetMyProfileDetails()
	for _, change := range reqPermissionChanges.GetChanges() {
		accountFound, err = n.findAccount(change.Identity, myProfile)
		if err != nil {
			return err
		}
		if accountFound {
			permissions = change.Permissions
			break
		}
	}
	if !accountFound {
		return nil
	}
	err = n.notificationService.CreateAndSend(&model.Notification{
		Id:      notificationId,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfParticipantPermissionsChange{
			ParticipantPermissionsChange: &model.NotificationParticipantPermissionsChange{
				SpaceId:     aclNotificationRecord.spaceId,
				Permissions: mapProtoPermissionToAcl(permissions),
				SpaceName:   aclNotificationRecord.spaceName,
			},
		},
		Space:     aclNotificationRecord.spaceId,
		AclHeadId: aclNotificationRecord.aclId,
	})
	if err != nil {
		return err
	}
	return nil
}

func (n *aclNotificationSender) isAccountRemoved(identities [][]byte, myProfile string) (bool, error) {
	for _, identity := range identities {
		found, err := n.findAccount(identity, myProfile)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

func (n *aclNotificationSender) findAccount(identity []byte, myProfile string) (bool, error) {
	pubKey, err := crypto.UnmarshalEd25519PublicKeyProto(identity)
	if err != nil {
		return false, err
	}
	account := pubKey.Account()
	if account == myProfile {
		return true, nil
	}
	return false, nil
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
