package acl

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/space"
)

const CName = "common.acl.aclservice"

type AclService interface {
	app.Component
	Join(ctx context.Context, spaceId string, inviteKey crypto.PrivKey) error
	Accept(ctx context.Context, spaceId string, identity crypto.PubKey) error
	GenerateInvite(ctx context.Context, spaceId string) (crypto.PrivKey, error)
}

func New() AclService {
	return &aclService{}
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
	metadata := a.spaceService.AccountMetadata()
	err := a.joiningClient.RequestJoin(ctx, spaceId, list.RequestJoinPayload{
		InviteKey: inviteKey,
		Metadata:  metadata,
	})
	if err != nil {
		return err
	}
	// TODO: check if we already have the space view
	return a.spaceService.Join(ctx, spaceId)
}

func (a *aclService) Accept(ctx context.Context, spaceId string, identity crypto.PubKey) error {
	acceptSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	acl := acceptSpace.CommonSpace().Acl()
	acl.RLock()
	recs, err := acl.AclState().JoinRecords(false)
	if err != nil {
		acl.RUnlock()
		return err
	}
	// TODO: change this logic to use RequestJoin objects
	var recId string
	for _, rec := range recs {
		if rec.RequestIdentity.Equals(identity) {
			recId = rec.RecordId
			break
		}
	}
	acl.RUnlock()
	if recId == "" {
		return fmt.Errorf("no record with requested identity: %s", identity.Account())
	}
	cl := acceptSpace.CommonSpace().AclClient()
	return cl.AcceptRequest(ctx, list.RequestAcceptPayload{
		RequestRecordId: recId,
		Permissions:     list.AclPermissions(aclrecordproto.AclUserPermissions_Writer),
	})
}

func (a *aclService) GenerateInvite(ctx context.Context, spaceId string) (crypto.PrivKey, error) {
	acceptSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	aclClient := acceptSpace.CommonSpace().AclClient()
	res, err := aclClient.GenerateInvite()
	if err != nil {
		return nil, err
	}
	return res.InviteKey, nil
}
