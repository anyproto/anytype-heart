package acl

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/core/invitestore"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "common.acl.aclservice"

type AclService interface {
	app.Component
	Join(ctx context.Context, spaceId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error
	Accept(ctx context.Context, spaceId string, identity crypto.PubKey) error
	GenerateInvite(ctx context.Context, spaceId string) (*GenerateInviteResult, error)
}

func New() AclService {
	return &aclService{}
}

type aclService struct {
	joiningClient  aclclient.AclJoiningClient
	spaceService   space.Service
	accountService account.Service
	inviteStore    invitestore.Service
	fileAcl        fileacl.Service
}

func (a *aclService) Init(ap *app.App) (err error) {
	a.joiningClient = ap.MustComponent(aclclient.CName).(aclclient.AclJoiningClient)
	a.spaceService = ap.MustComponent(space.CName).(space.Service)
	a.accountService = app.MustComponent[account.Service](ap)
	a.inviteStore = app.MustComponent[invitestore.Service](ap)
	a.fileAcl = app.MustComponent[fileacl.Service](ap)
	return nil
}

func (a *aclService) Name() (name string) {
	return CName
}

func (a *aclService) Join(ctx context.Context, spaceId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error {
	metadata := a.spaceService.AccountMetadataPayload()

	invite, err := a.inviteStore.GetInvite(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return fmt.Errorf("get invite: %w", err)
	}

	var invitePayload model.InvitePayload
	err = proto.Unmarshal(invite.Payload, &invitePayload)
	if err != nil {
		return fmt.Errorf("unmarshal invite payload: %w", err)
	}

	creatorIdentity, err := crypto.DecodeAccountAddress(invitePayload.CreatorIdentity)
	if err != nil {
		return fmt.Errorf("decode creator identity: %w", err)
	}

	ok, err := creatorIdentity.Verify(invite.Payload, invite.Signature)
	if err != nil {
		return fmt.Errorf("verify invite signature: %w", err)
	}
	if !ok {
		return fmt.Errorf("invite signature is invalid")
	}

	// TODO Setup space name and info
	inviteKey, err := crypto.UnmarshalEd25519PrivateKeyProto(invitePayload.InviteKey)
	if err != nil {
		return fmt.Errorf("unmarshal invite key: %w", err)
	}
	err = a.joiningClient.RequestJoin(ctx, spaceId, list.RequestJoinPayload{
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

type GenerateInviteResult struct {
	InviteKey     crypto.PrivKey
	InviteFileCid cid.Cid
	InviteFileKey crypto.SymKey
}

func (a *aclService) GenerateInvite(ctx context.Context, spaceId string) (result *GenerateInviteResult, err error) {
	acceptSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	aclClient := acceptSpace.CommonSpace().AclClient()
	res, err := aclClient.GenerateInvite()
	if err != nil {
		return nil, err
	}
	err = aclClient.AddRecord(ctx, res.InviteRec)

	rawInviteKey, err := res.InviteKey.Marshall()
	if err != nil {
		return nil, err
	}
	invitePayload := &model.InvitePayload{
		CreatorIdentity: a.accountService.AccountID(),
		InviteKey:       rawInviteKey,
	}

	err = acceptSpace.Do(acceptSpace.DerivedIDs().Workspace, func(sb smartblock.SmartBlock) error {
		details := sb.Details()
		invitePayload.SpaceName = pbtypes.GetString(details, bundle.RelationKeyName.String())
		iconObjectId := pbtypes.GetString(details, bundle.RelationKeyIconImage.String())
		if iconObjectId != "" {
			iconCid, iconEncryptionKeys, err := a.fileAcl.GetInfoForFileSharing(ctx, iconObjectId)
			if err != nil {
				return fmt.Errorf("get icon info: %w", err)
			}
			invitePayload.SpaceIconCid = iconCid
			invitePayload.SpaceIconEncryptionKeys = iconEncryptionKeys
		}
		return nil
	})

	invitePayloadRaw, err := proto.Marshal(invitePayload)
	if err != nil {
		return nil, fmt.Errorf("marshal invite payload: %w", err)
	}
	invitePayloadSignature, err := a.accountService.SignData(invitePayloadRaw)
	if err != nil {
		return nil, fmt.Errorf("sign invite payload: %w", err)
	}
	invite := &model.Invite{
		Payload:   invitePayloadRaw,
		Signature: invitePayloadSignature,
	}
	inviteFileCid, inviteFileKey, err := a.inviteStore.StoreInvite(ctx, invite)
	if err != nil {
		return nil, fmt.Errorf("store invite in ipfs: %w", err)
	}

	return &GenerateInviteResult{
		InviteKey:     res.InviteKey,
		InviteFileCid: inviteFileCid,
		InviteFileKey: inviteFileKey,
	}, err
}
