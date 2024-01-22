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
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/core/invitestore"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
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
	objectGetter   getblock.ObjectGetter
}

func (a *aclService) Init(ap *app.App) (err error) {
	a.joiningClient = ap.MustComponent(aclclient.CName).(aclclient.AclJoiningClient)
	a.spaceService = ap.MustComponent(space.CName).(space.Service)
	a.accountService = app.MustComponent[account.Service](ap)
	a.inviteStore = app.MustComponent[invitestore.Service](ap)
	a.fileAcl = app.MustComponent[fileacl.Service](ap)
	a.objectGetter = app.MustComponent[getblock.ObjectGetter](ap)
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
	InviteFileCid string
	InviteFileKey string
}

func (a *aclService) buildInvite(ctx context.Context, space clientspace.Space, inviteKey crypto.PrivKey) (*model.Invite, error) {
	invitePayload, err := a.buildInvitePayload(ctx, space, inviteKey)
	if err != nil {
		return nil, fmt.Errorf("build invite payload: %w", err)
	}
	invitePayloadRaw, err := proto.Marshal(invitePayload)
	if err != nil {
		return nil, fmt.Errorf("marshal invite payload: %w", err)
	}
	invitePayloadSignature, err := a.accountService.SignData(invitePayloadRaw)
	if err != nil {
		return nil, fmt.Errorf("sign invite payload: %w", err)
	}
	return &model.Invite{
		Payload:   invitePayloadRaw,
		Signature: invitePayloadSignature,
	}, nil
}

func (a *aclService) buildInvitePayload(ctx context.Context, space clientspace.Space, inviteKey crypto.PrivKey) (*model.InvitePayload, error) {
	rawInviteKey, err := inviteKey.Marshall()
	if err != nil {
		return nil, fmt.Errorf("marshal invite priv key: %w", err)
	}
	invitePayload := &model.InvitePayload{
		CreatorIdentity: a.accountService.AccountID(),
		InviteKey:       rawInviteKey,
	}
	err = space.Do(space.DerivedIDs().Workspace, func(sb smartblock.SmartBlock) error {
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
	if err != nil {
		return nil, err
	}
	return invitePayload, nil
}

type spaceViewObject interface {
	SetInviteFileInfo(fileCid string, fileKey string) (err error)
}

func (a *aclService) getExistingInviteFileInfo(spaceViewId string) (fileCid string, fileKey string, err error) {
	err = getblock.Do(a.objectGetter, spaceViewId, func(sb smartblock.SmartBlock) error {
		details := sb.Details()
		fileCid = pbtypes.GetString(details, bundle.RelationKeySpaceInviteFileCid.String())
		fileKey = pbtypes.GetString(details, bundle.RelationKeySpaceInviteFileKey.String())
		return nil
	})
	return
}

func (a *aclService) GenerateInvite(ctx context.Context, spaceId string) (result *GenerateInviteResult, err error) {
	spaceViewId, err := a.spaceService.SpaceViewId(spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space view id: %w", err)
	}
	fileCid, fileKey, err := a.getExistingInviteFileInfo(spaceViewId)
	if err != nil {
		return nil, fmt.Errorf("get existing invite file info: %w", err)
	}
	if fileCid != "" {
		return &GenerateInviteResult{
			InviteFileCid: fileCid,
			InviteFileKey: fileKey,
		}, nil
	}

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
	if err != nil {
		return nil, err
	}

	invite, err := a.buildInvite(ctx, acceptSpace, res.InviteKey)
	if err != nil {
		return nil, fmt.Errorf("build invite: %w", err)
	}
	inviteFileCid, inviteFileKey, err := a.inviteStore.StoreInvite(ctx, spaceId, invite)
	if err != nil {
		return nil, fmt.Errorf("store invite in ipfs: %w", err)
	}
	inviteFileKeyRaw, err := crypto.EncodeKeyToString(inviteFileKey)
	if err != nil {
		return nil, fmt.Errorf("encode invite file key: %w", err)
	}

	err = getblock.Do(a.objectGetter, spaceViewId, func(sb smartblock.SmartBlock) error {
		view, ok := sb.(spaceViewObject)
		if !ok {
			return fmt.Errorf("space view object is not implemented")
		}
		return view.SetInviteFileInfo(inviteFileCid.String(), inviteFileKeyRaw)
	})
	if err != nil {
		return nil, fmt.Errorf("set invite file info: %w", err)
	}

	return &GenerateInviteResult{
		InviteFileCid: inviteFileCid.String(),
		InviteFileKey: inviteFileKeyRaw,
	}, err
}