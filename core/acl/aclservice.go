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
	joiningClient aclclient.AclJoiningClient
	spaceService  space.Service
}

func (a *aclService) Init(app *app.App) (err error) {
	a.joiningClient = app.MustComponent(aclclient.CName).(aclclient.AclJoiningClient)
	a.spaceService = app.MustComponent(space.CName).(space.Service)
	return nil
}

func (a *aclService) Name() (name string) {
	return CName
}

func (a *aclService) Join(ctx context.Context, spaceId string, inviteKey crypto.PrivKey) error {
	metadata := a.spaceService.AccountMetadataPayload()
	err := a.joiningClient.RequestJoin(ctx, spaceId, list.RequestJoinPayload{
		InviteKey: inviteKey,
		Metadata:  metadata,
	})
	if err != nil {
		return err
	}
	return a.spaceService.Join(ctx, spaceId)
}
