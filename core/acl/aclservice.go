package acl

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	"github.com/mr-tron/base58/base58"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/core/invitestore"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "common.acl.aclservice"

var (
	ErrInviteNotExist       = errors.New("invite doesn't exist")
	ErrPersonalSpace        = errors.New("sharing of personal space is forbidden")
	ErrInviteBadSignature   = errors.New("invite has bad signature")
	ErrIncorrectPermissions = errors.New("incorrect permissions")
	ErrNoSuchUser           = errors.New("no such user")
	ErrAclRequestFailed     = errors.New("acl request failed")
)

type AccountPermissions struct {
	Account     crypto.PubKey
	Permissions model.ParticipantPermissions
}

type AclService interface {
	app.Component
	ViewInvite(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (*InviteView, error)
	Join(ctx context.Context, spaceId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error
	Accept(ctx context.Context, spaceId string, identity crypto.PubKey, permissions model.ParticipantPermissions) error
	Remove(ctx context.Context, spaceId string, identities []crypto.PubKey) (err error)
	Exit(ctx context.Context, spaceId string) (err error)
	Cancel(ctx context.Context, spaceId string) (err error)
	Decline(ctx context.Context, spaceId string, identity crypto.PubKey) (err error)
	ChangePermissions(ctx context.Context, spaceId string, perms []AccountPermissions) (err error)
	GetCurrentInvite(spaceId string) (*InviteInfo, error)
	GenerateInvite(ctx context.Context, spaceId string) (*InviteInfo, error)
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

func (a *aclService) Remove(ctx context.Context, spaceId string, identities []crypto.PubKey) error {
	removeSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	newPrivKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		return err
	}
	cl := removeSpace.CommonSpace().AclClient()
	err = cl.RemoveAccounts(ctx, list.AccountRemovePayload{
		Identities: identities,
		Change: list.ReadKeyChangePayload{
			MetadataKey: newPrivKey,
			ReadKey:     crypto.NewAES(),
		},
	})
	if err != nil {
		return fmt.Errorf("%w, %w", ErrAclRequestFailed, err)
	}
	return nil
}

func (a *aclService) Cancel(ctx context.Context, spaceId string) (err error) {
	// TODO: finish this by implementing space offload for join canceled spaces (?)
	sp, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	cl := sp.CommonSpace().AclClient()
	err = cl.CancelRequest(ctx)
	if err != nil {
		return fmt.Errorf("%w, %w", ErrAclRequestFailed, err)
	}
	return nil
}

func (a *aclService) Decline(ctx context.Context, spaceId string, identity crypto.PubKey) (err error) {
	sp, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	cl := sp.CommonSpace().AclClient()
	err = cl.DeclineRequest(ctx, identity)
	if err != nil {
		return fmt.Errorf("%w, %w", ErrAclRequestFailed, err)
	}
	return nil
}

func (a *aclService) ChangePermissions(ctx context.Context, spaceId string, perms []AccountPermissions) error {
	sp, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	var listPerms []list.PermissionChangePayload
	acl := sp.CommonSpace().Acl()
	acl.RLock()
	for _, perm := range perms {
		var aclPerms list.AclPermissions
		switch perm.Permissions {
		case model.ParticipantPermissions_Reader:
			aclPerms = list.AclPermissionsReader
		case model.ParticipantPermissions_Writer:
			aclPerms = list.AclPermissionsWriter
		default:
			acl.RUnlock()
			return ErrIncorrectPermissions
		}
		curPerms := acl.AclState().Permissions(perm.Account)
		if curPerms.NoPermissions() {
			acl.RUnlock()
			return ErrNoSuchUser
		}
		if curPerms == aclPerms {
			continue
		}
		listPerms = append(listPerms, list.PermissionChangePayload{
			Identity:    perm.Account,
			Permissions: aclPerms,
		})
	}
	acl.RUnlock()
	cl := sp.CommonSpace().AclClient()
	err = cl.ChangePermissions(ctx, list.PermissionChangesPayload{
		Changes: listPerms,
	})
	if err != nil {
		return fmt.Errorf("%w, %w", ErrAclRequestFailed, err)
	}
	return nil
}

func (a *aclService) Exit(ctx context.Context, spaceId string) error {
	removeSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	cl := removeSpace.CommonSpace().AclClient()
	err = cl.RequestSelfRemove(ctx)
	if err != nil {
		return fmt.Errorf("%w, %w", ErrAclRequestFailed, err)
	}
	return nil
}

func (a *aclService) Join(ctx context.Context, spaceId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error {
	invitePayload, err := a.getInvitePayload(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return fmt.Errorf("get invite payload: %w", err)
	}

	inviteKey, err := crypto.UnmarshalEd25519PrivateKeyProto(invitePayload.InviteKey)
	if err != nil {
		return fmt.Errorf("unmarshal invite key: %w", err)
	}

	err = a.joiningClient.RequestJoin(ctx, spaceId, list.RequestJoinPayload{
		InviteKey: inviteKey,
		Metadata:  a.spaceService.AccountMetadataPayload(),
	})
	if err != nil {
		return fmt.Errorf("%w, %w", ErrAclRequestFailed, err)
	}
	return a.spaceService.Join(ctx, spaceId)
}

type InviteView struct {
	SpaceId      string
	SpaceName    string
	SpaceIconCid string
	CreatorName  string
}

func (a *aclService) ViewInvite(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (*InviteView, error) {
	invitePayload, err := a.getInvitePayload(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return nil, fmt.Errorf("get invite payload: %w", err)
	}
	return &InviteView{
		SpaceId:      invitePayload.SpaceId,
		SpaceName:    invitePayload.SpaceName,
		SpaceIconCid: invitePayload.SpaceIconCid,
		CreatorName:  invitePayload.CreatorName,
	}, nil
}

func (a *aclService) getInvitePayload(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (*model.InvitePayload, error) {
	invite, err := a.inviteStore.GetInvite(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return nil, fmt.Errorf("get invite: %w", err)
	}
	var invitePayload model.InvitePayload
	err = proto.Unmarshal(invite.Payload, &invitePayload)
	if err != nil {
		return nil, fmt.Errorf("unmarshal invite payload: %w", err)
	}
	creatorIdentity, err := crypto.DecodeAccountAddress(invitePayload.CreatorIdentity)
	if err != nil {
		return nil, fmt.Errorf("decode creator identity: %w", err)
	}

	ok, err := creatorIdentity.Verify(invite.Payload, invite.Signature)
	if err != nil {
		return nil, fmt.Errorf("verify invite signature: %w", err)
	}
	if !ok {
		return nil, ErrInviteBadSignature
	}

	err = a.fileAcl.StoreFileKeys(domain.FileId(invitePayload.SpaceIconCid), invitePayload.SpaceIconEncryptionKeys)
	if err != nil {
		return nil, fmt.Errorf("store icon keys: %w", err)
	}

	return &invitePayload, nil
}

func (a *aclService) Accept(ctx context.Context, spaceId string, identity crypto.PubKey, permissions model.ParticipantPermissions) error {
	validPerms := permissions == model.ParticipantPermissions_Reader || permissions == model.ParticipantPermissions_Writer
	if !validPerms {
		return ErrIncorrectPermissions
	}
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
		return fmt.Errorf("%w with identity: %s", ErrNoSuchUser, identity.Account())
	}
	cl := acceptSpace.CommonSpace().AclClient()
	var aclPerms list.AclPermissions
	switch permissions {
	case model.ParticipantPermissions_Reader:
		aclPerms = list.AclPermissionsReader
	case model.ParticipantPermissions_Writer:
		aclPerms = list.AclPermissionsWriter
	}
	err = cl.AcceptRequest(ctx, list.RequestAcceptPayload{
		RequestRecordId: recId,
		Permissions:     aclPerms,
	})
	if err != nil {
		return fmt.Errorf("%w, %w", ErrAclRequestFailed, err)
	}
	return nil
}

type InviteInfo struct {
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
	profile, err := a.accountService.ProfileInfo()
	if err != nil {
		return nil, fmt.Errorf("get profile info: %w", err)
	}
	rawInviteKey, err := inviteKey.Marshall()
	if err != nil {
		return nil, fmt.Errorf("marshal invite priv key: %w", err)
	}
	invitePayload := &model.InvitePayload{
		SpaceId:         space.Id(),
		CreatorIdentity: a.accountService.AccountID(),
		CreatorName:     profile.Name,
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

func (a *aclService) GetCurrentInvite(spaceId string) (*InviteInfo, error) {
	spaceViewId, err := a.spaceService.SpaceViewId(spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space view id: %w", err)
	}
	fileCid, fileKey, err := a.getExistingInviteFileInfo(spaceViewId)
	if err != nil {
		return nil, fmt.Errorf("get existing invite file info: %w", err)
	}
	if fileCid == "" {
		return nil, ErrInviteNotExist
	}
	return &InviteInfo{
		InviteFileCid: fileCid,
		InviteFileKey: fileKey,
	}, nil
}

func (a *aclService) GenerateInvite(ctx context.Context, spaceId string) (result *InviteInfo, err error) {
	if spaceId == a.accountService.PersonalSpaceID() {
		return nil, ErrPersonalSpace
	}
	spaceViewId, err := a.spaceService.SpaceViewId(spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space view id: %w", err)
	}
	fileCid, fileKey, err := a.getExistingInviteFileInfo(spaceViewId)
	if err != nil {
		return nil, fmt.Errorf("get existing invite file info: %w", err)
	}
	if fileCid != "" {
		return &InviteInfo{
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
		return nil, fmt.Errorf("%w, %w", ErrAclRequestFailed, err)
	}

	invite, err := a.buildInvite(ctx, acceptSpace, res.InviteKey)
	if err != nil {
		return nil, fmt.Errorf("build invite: %w", err)
	}
	inviteFileCid, inviteFileKey, err := a.inviteStore.StoreInvite(ctx, spaceId, invite)
	if err != nil {
		return nil, fmt.Errorf("store invite in ipfs: %w", err)
	}
	inviteFileKeyRaw, err := EncodeKeyToBase58(inviteFileKey)
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

	return &InviteInfo{
		InviteFileCid: inviteFileCid.String(),
		InviteFileKey: inviteFileKeyRaw,
	}, err
}

func EncodeKeyToBase58(key crypto.SymKey) (string, error) {
	raw, err := key.Raw()
	if err != nil {
		return "", err
	}
	return base58.Encode(raw), nil
}

func DecodeKeyFromBase58(rawString string) (crypto.SymKey, error) {
	raw, err := base58.Decode(rawString)
	if err != nil {
		return nil, err
	}
	return crypto.UnmarshallAESKey(raw)
}
