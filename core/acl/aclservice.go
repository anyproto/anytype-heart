package acl

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/space"
)

const CName = "common.acl.aclservice"

type AclService interface {
	app.Component
	Join(ctx context.Context, spaceId string, inviteKey crypto.PrivKey) error
}

type aclService struct {
	invitingClient aclclient.AclInvitingClient
	spaceService   space.Service
}

func (a *aclService) Init(app *app.App) (err error) {
	a.invitingClient = app.MustComponent(aclclient.CName).(aclclient.AclInvitingClient)
	a.spaceService = app.MustComponent(space.CName).(space.Service)
	return nil
}

func (a *aclService) Name() (name string) {
	return CName
}

func (a *aclService) Join(ctx context.Context, spaceId string, inviteKey crypto.PrivKey) error {
	metadata := a.spaceService.AccountMetadata()
	err := a.invitingClient.RequestJoin(ctx, spaceId, list.RequestJoinPayload{
		InviteKey: inviteKey,
		Metadata:  metadata,
	})
	if err != nil {
		return err
	}
	return a.spaceService.Join(ctx, spaceId)
}
