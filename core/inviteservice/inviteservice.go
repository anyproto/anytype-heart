package inviteservice

/*
AI generated

Name: Space Invite Generator
Scope: global

## Responsibility
- Generate, retrieve, view, and remove space invites
- Build signed invite payloads with space/creator metadata and icon encryption keys
- Support multiple invite types: member (default), guest, and without-approval

## Documentation
Invite lifecycle:
1. Generate: build payload -> sign with account key -> store via invitestore -> persist info in workspace smartblock
2. Remove: clear from workspace smartblock -> remove from invitestore
3. View/GetPayload: fetch from invitestore -> verify signature -> optionally store icon encryption keys for rendering

Guest invites are stored separately from regular invites and only work for Stream-type spaces.
*/

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/core/invitestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/encode"
)

const CName = "common.core.inviteservice"

var log = logger.NewNamed(CName)

type InviteService interface {
	app.ComponentRunnable
	GetPayload(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (*model.InvitePayload, error)
	View(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (domain.InviteView, error)
	// RemoveExisting clears the space's invite info and returns the invite it removed
	RemoveExisting(ctx context.Context, spaceId string) (domain.InviteInfo, error)
	Generate(ctx context.Context, params GenerateInviteParams, sendInvite func() error) (domain.InviteInfo, error)
	Change(ctx context.Context, spaceId string, permissions list.AclPermissions) error
	GetCurrent(ctx context.Context, spaceId string) (domain.InviteInfo, error)
	GetExistingGuestUserInvite(ctx context.Context, spaceId string) (domain.InviteInfo, error)
	GenerateGuestUserInvite(ctx context.Context, spaceId string, guestKey crypto.PrivKey) (domain.InviteInfo, error)
}

var _ InviteService = (*inviteService)(nil)

var ErrInvalidSpaceType = fmt.Errorf("invalid space type")

type GenerateInviteParams struct {
	SpaceId     string
	Key         crypto.PrivKey
	InviteType  domain.InviteType
	Permissions list.AclPermissions
	// ShareWithinSpace stores the invite in the workspace, where every member of the space can read it
	// and share the link. Otherwise it is stored in the owner's space view and only they can share it.
	ShareWithinSpace bool
}

type inviteService struct {
	inviteStore    invitestore.Service
	fileAcl        fileacl.Service
	accountService account.Service
	spaceService   space.Service
}

func New() InviteService {
	return &inviteService{}
}

func (i *inviteService) Init(a *app.App) (err error) {
	i.inviteStore = app.MustComponent[invitestore.Service](a)
	i.fileAcl = app.MustComponent[fileacl.Service](a)
	i.accountService = app.MustComponent[account.Service](a)
	i.spaceService = app.MustComponent[space.Service](a)
	return
}

func (i *inviteService) Name() (name string) {
	return CName
}

func (i *inviteService) Run(ctx context.Context) (err error) {
	return
}

func (i *inviteService) Close(ctx context.Context) (err error) {
	return
}

func (i *inviteService) View(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (domain.InviteView, error) {
	invitePayload, err := i.GetPayload(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return domain.InviteView{}, err
	}
	return domain.InviteView{
		SpaceId:         invitePayload.SpaceId,
		SpaceName:       invitePayload.SpaceName,
		SpaceIconCid:    invitePayload.SpaceIconCid,
		SpaceUxType:     model.SpaceUxType(invitePayload.SpaceUxType), // nolint:gosec
		SpaceType:       model.SpaceType(invitePayload.SpaceType),     // nolint:gosec
		SpaceIconOption: int(invitePayload.SpaceIconOption),
		CreatorName:     invitePayload.CreatorName,
		CreatorIconCid:  invitePayload.CreatorIconCid,
		AclKey:          invitePayload.AclKey,
		GuestKey:        invitePayload.GuestKey,
		InviteType:      domain.InviteType(invitePayload.InviteType),
	}, nil
}

func (i *inviteService) Change(ctx context.Context, spaceId string, permissions list.AclPermissions) error {
	info, err := i.getExisting(ctx, spaceId)
	if err != nil {
		return getInviteError("get existing invite info", err)
	}
	if info.InviteFileCid == "" {
		// the invite is held by the owner and this is not their device: it cannot be changed from here
		return ErrInviteNotExists
	}
	info.Permissions = permissions
	return i.setInviteInfo(ctx, spaceId, info)
}

func (i *inviteService) GetCurrent(ctx context.Context, spaceId string) (info domain.InviteInfo, err error) {
	info, err = i.getExisting(ctx, spaceId)
	if err != nil {
		return domain.InviteInfo{}, getInviteError("get existing invite info", err)
	}
	// an invite held by the owner is reported to their members without its cid and key: the marker is
	// all they get, and their client turns it into "ask the owner for the link"
	if info.InviteFileCid == "" && !info.HeldByOwner {
		return domain.InviteInfo{}, ErrInviteNotExists
	}
	return info, nil
}

// getExisting reports what this device knows about the space's current invite. The space view is
// asked first: an owner-held invite exists nowhere else, and the owner syncs it across their own
// devices. The workspace then answers for everyone else — with a shared invite, or with the marker
// of an owner-held one.
func (i *inviteService) getExisting(ctx context.Context, spaceId string) (info domain.InviteInfo, err error) {
	err = i.doSpaceView(ctx, spaceId, func(obj domain.InviteInfoObject) error {
		info = obj.GetExistingInviteInfo()
		return nil
	})
	if err != nil {
		return domain.InviteInfo{}, fmt.Errorf("read invite info from space view: %w", err)
	}
	if info.InviteFileCid != "" {
		return info, nil
	}
	err = i.doWorkspace(ctx, spaceId, func(obj domain.InviteObject) error {
		info = obj.GetExistingInviteInfo()
		return nil
	})
	if err != nil {
		return domain.InviteInfo{}, fmt.Errorf("read invite info from workspace: %w", err)
	}
	return info, nil
}

// setInviteInfo writes the invite to the object that holds it and updates the other one, so that
// switching a space between a shared invite and an owner-held one can never leave the previous
// invite readable where it used to live.
func (i *inviteService) setInviteInfo(ctx context.Context, spaceId string, info domain.InviteInfo) error {
	err := i.doSpaceView(ctx, spaceId, func(obj domain.InviteInfoObject) error {
		if info.HeldByOwner {
			return obj.SetInviteFileInfo(info)
		}
		_, err := obj.RemoveExistingInviteInfo()
		return err
	})
	if err != nil {
		return fmt.Errorf("set invite info in space view: %w", err)
	}
	// the workspace takes the full invite when it is shared within the space, and the marker alone
	// when the owner holds it
	err = i.doWorkspace(ctx, spaceId, func(obj domain.InviteObject) error {
		return obj.SetInviteFileInfo(info)
	})
	if err != nil {
		return fmt.Errorf("set invite info in workspace: %w", err)
	}
	return nil
}

func (i *inviteService) RemoveExisting(ctx context.Context, spaceId string) (info domain.InviteInfo, err error) {
	// both objects are cleared: the invite lives in one of them, and the other carries the marker or
	// a pair left behind by an earlier invite of the other kind
	err = i.doSpaceView(ctx, spaceId, func(obj domain.InviteInfoObject) error {
		info, err = obj.RemoveExistingInviteInfo()
		return err
	})
	if err != nil {
		return domain.InviteInfo{}, removeInviteError("remove existing invite info from space view", err)
	}
	err = i.doWorkspace(ctx, spaceId, func(obj domain.InviteObject) error {
		workspaceInfo, err := obj.RemoveExistingInviteInfo()
		if err != nil {
			return err
		}
		if info.InviteFileCid == "" {
			info = workspaceInfo
		}
		return nil
	})
	if err != nil {
		return domain.InviteInfo{}, removeInviteError("remove existing invite info from workspace", err)
	}
	if len(info.InviteFileCid) == 0 {
		return info, nil
	}
	invCid, err := cid.Decode(info.InviteFileCid)
	if err != nil {
		return info, removeInviteError("decode invite cid", err)
	}
	err = i.inviteStore.RemoveInvite(ctx, invCid)
	if err != nil {
		return info, removeInviteError("remove invite from store", err)
	}
	return info, nil
}

func (i *inviteService) doWorkspace(ctx context.Context, spaceId string, f func(object domain.InviteObject) error) error {
	sp, err := i.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	return sp.Do(sp.DerivedIDs().Workspace, func(sb smartblock.SmartBlock) error {
		invObject, ok := sb.(domain.InviteObject)
		if !ok {
			return fmt.Errorf("space is not invite object")
		}
		return f(invObject)
	})
}

func (i *inviteService) doSpaceView(ctx context.Context, spaceId string, f func(object domain.InviteInfoObject) error) error {
	return i.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		return f(spaceView)
	})
}

func (i *inviteService) GenerateGuestUserInvite(ctx context.Context, spaceId string, guestUserKey crypto.PrivKey) (domain.InviteInfo, error) {
	return i.generateGuestInvite(ctx, spaceId, guestUserKey)
}

func (i *inviteService) Generate(ctx context.Context, params GenerateInviteParams, sendInvite func() error) (result domain.InviteInfo, err error) {
	spaceId := params.SpaceId
	if spaceId == i.accountService.PersonalSpaceID() {
		return domain.InviteInfo{}, ErrPersonalSpace
	}
	result, err = i.getExisting(ctx, spaceId)
	if err != nil {
		return domain.InviteInfo{}, generateInviteError("get existing invite info", err)
	}
	heldByOwner := !params.ShareWithinSpace
	// an invite that differs in where it is held is replaced like one that differs in type: the point
	// of the switch is that the invite the other side could read stops working
	if result.InviteFileCid != "" && result.InviteType == params.InviteType && result.HeldByOwner == heldByOwner {
		return result, nil
	}
	invite, err := i.buildInvite(ctx, params)
	if err != nil {
		return domain.InviteInfo{}, generateInviteError("build invite", err)
	}
	// the invite's public key binds the file to the invite on the coordinator, which is what lets it
	// verify a later deletion against the acl
	inviteFileCid, inviteFileKey, err := i.inviteStore.StoreInvite(ctx, spaceId, params.Key.GetPublic(), invite)
	if err != nil {
		return domain.InviteInfo{}, generateInviteError("store invite in ipfs", err)
	}
	removeInviteFile := func() {
		err := i.inviteStore.RemoveInvite(ctx, inviteFileCid)
		if err != nil {
			log.Error("remove invite file", zap.Error(err))
		}
	}
	inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteFileKey)
	if err != nil {
		removeInviteFile()
		return domain.InviteInfo{}, generateInviteError("encode invite file key", err)
	}
	inviteInfo := domain.InviteInfo{
		InviteFileCid: inviteFileCid.String(),
		InviteFileKey: inviteFileKeyRaw,
		InviteType:    params.InviteType,
		Permissions:   params.Permissions,
		HeldByOwner:   heldByOwner,
	}
	err = i.setInviteInfo(ctx, spaceId, inviteInfo)
	if err != nil {
		removeInviteFile()
		return domain.InviteInfo{}, generateInviteError("set invite file info", err)
	}
	err = sendInvite()
	if err != nil {
		_, removeErr := i.RemoveExisting(ctx, spaceId)
		if removeErr != nil {
			log.Error("remove existing invite", zap.Error(removeErr))
		}
		return domain.InviteInfo{}, generateInviteError("send invite", err)
	}
	return inviteInfo, err
}

func (i *inviteService) generateGuestInvite(ctx context.Context, spaceId string, guestUserKey crypto.PrivKey) (result domain.InviteInfo, err error) {
	if spaceId == i.accountService.PersonalSpaceID() {
		return domain.InviteInfo{}, ErrPersonalSpace
	}
	invite, err := i.buildInvite(ctx, GenerateInviteParams{
		SpaceId:    spaceId,
		Key:        guestUserKey,
		InviteType: domain.InviteTypeGuest,
	})
	if err != nil {
		return domain.InviteInfo{}, generateInviteError("build invite", err)
	}
	inviteFileCid, inviteFileKey, err := i.inviteStore.StoreInvite(ctx, spaceId, guestUserKey.GetPublic(), invite)
	if err != nil {
		return domain.InviteInfo{}, generateInviteError("store invite in ipfs", err)
	}
	removeInviteFile := func() {
		err := i.inviteStore.RemoveInvite(ctx, inviteFileCid)
		if err != nil {
			log.Error("remove invite file", zap.Error(err))
		}
	}
	inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteFileKey)
	if err != nil {
		removeInviteFile()
		return domain.InviteInfo{}, generateInviteError("encode invite file key", err)
	}
	inviteInfo := domain.InviteInfo{
		InviteFileCid: inviteFileCid.String(),
		InviteFileKey: inviteFileKeyRaw,
		InviteType:    domain.InviteTypeGuest,
	}
	err = i.doWorkspace(ctx, spaceId, func(obj domain.InviteObject) error {
		return obj.SetGuestInviteFileInfo(inviteFileCid.String(), inviteFileKeyRaw)
	})
	if err != nil {
		removeInviteFile()
		return domain.InviteInfo{}, generateInviteError("set invite file info", err)
	}

	return inviteInfo, err
}

func (i *inviteService) GetPayload(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (md *model.InvitePayload, err error) {
	invite, err := i.inviteStore.GetInvite(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return nil, getInviteError("get invite from store", err)
	}
	var invitePayload model.InvitePayload
	err = proto.Unmarshal(invite.Payload, &invitePayload)
	if err != nil {
		return nil, badContentError("unmarshal invite payload", err)
	}
	creatorIdentity, err := crypto.DecodeAccountAddress(invitePayload.CreatorIdentity)
	if err != nil {
		return nil, badContentError("decode creator identity", err)
	}
	ok, err := creatorIdentity.Verify(invite.Payload, invite.Signature)
	if err != nil {
		return nil, badContentError("verify creator identity", err)
	}
	if !ok {
		return nil, badContentError("verify creator identity", fmt.Errorf("signature is invalid"))
	}
	if invitePayload.SpaceIconCid != "" {
		err = i.fileAcl.StoreFileKeys(domain.FileId(invitePayload.SpaceIconCid), invitePayload.SpaceIconEncryptionKeys)
		if err != nil {
			return nil, getInviteError("store space icon encryption keys", err)
		}
	}

	if invitePayload.CreatorIconCid != "" {
		err = i.fileAcl.StoreFileKeys(domain.FileId(invitePayload.CreatorIconCid), invitePayload.CreatorIconEncryptionKeys)
		if err != nil {
			return nil, getInviteError("store creator icon encryption keys", err)
		}
	}

	// Backward compatibility: derive spaceType from spaceUxType for old invites
	if model.SpaceType(invitePayload.SpaceType) == model.SpaceType_SpaceTypeUnknown { // nolint:gosec
		switch model.SpaceUxType(invitePayload.SpaceUxType) { // nolint:gosec
		case model.SpaceUxType_Chat, model.SpaceUxType_Data:
			invitePayload.SpaceType = uint32(model.SpaceType_SpaceTypeRegular)
		case model.SpaceUxType_Stream:
			invitePayload.SpaceType = uint32(model.SpaceType_SpaceTypeChat)
		case model.SpaceUxType_OneToOne:
			invitePayload.SpaceType = uint32(model.SpaceType_SpaceTypeOneToOne)
		default:
			invitePayload.SpaceType = uint32(model.SpaceType_SpaceTypeRegular)
		}
	}

	return &invitePayload, nil
}

func (i *inviteService) buildInvite(ctx context.Context, params GenerateInviteParams) (*model.Invite, error) {
	if params.Key == nil {
		return nil, fmt.Errorf("you should provide either acl key or guest user key")
	}
	invitePayload, err := i.buildInvitePayload(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("build invite payload: %w", err)
	}
	invitePayloadRaw, err := proto.Marshal(invitePayload)
	if err != nil {
		return nil, fmt.Errorf("marshal invite payload: %w", err)
	}
	invitePayloadSignature, err := i.accountService.SignData(invitePayloadRaw)
	if err != nil {
		return nil, fmt.Errorf("sign invite payload: %w", err)
	}
	return &model.Invite{
		Payload:   invitePayloadRaw,
		Signature: invitePayloadSignature,
	}, nil
}

func (i *inviteService) buildInvitePayload(ctx context.Context, params GenerateInviteParams) (*model.InvitePayload, error) {
	profile, err := i.accountService.ProfileInfo()
	if err != nil {
		return nil, fmt.Errorf("get profile info: %w", err)
	}

	invitePayload := &model.InvitePayload{
		SpaceId:         params.SpaceId,
		CreatorIdentity: i.accountService.AccountID(),
		CreatorName:     profile.Name,
	}
	rawKey, err := params.Key.Marshall()
	if err != nil {
		return nil, fmt.Errorf("marshal invite priv key: %w", err)
	}
	switch params.InviteType {
	case domain.InviteTypeGuest:
		invitePayload.GuestKey = rawKey
		invitePayload.InviteType = model.InviteType_Guest
	case domain.InviteTypeAnyone:
		invitePayload.AclKey = rawKey
		invitePayload.InviteType = model.InviteType_WithoutApprove
	case domain.InviteTypeDefault:
		invitePayload.AclKey = rawKey
		invitePayload.InviteType = model.InviteType_Member
	}

	var description spaceinfo.SpaceDescription
	err = i.spaceService.TechSpace().DoSpaceView(ctx, params.SpaceId, func(spaceView techspace.SpaceView) error {
		description = spaceView.GetSpaceDescription()
		return nil
	})
	invitePayload.SpaceName = description.Name
	if err != nil {
		return nil, fmt.Errorf("get space description: %w", err)
	}
	invitePayload.SpaceIconOption = uint32(description.IconOption)
	invitePayload.SpaceUxType = uint32(description.SpaceUxType) // nolint:gosec
	invitePayload.SpaceType = uint32(description.SpaceType)     // nolint:gosec
	if description.IconImage != "" {
		iconCid, iconEncryptionKeys, err := i.fileAcl.GetInfoForFileSharing(description.IconImage)
		if err == nil {
			invitePayload.SpaceIconCid = iconCid
			invitePayload.SpaceIconEncryptionKeys = iconEncryptionKeys
		} else {
			log.Error("get space icon info", zap.Error(err))
		}
	}

	if profile.IconImage != "" {
		iconCid, iconEncryptionKeys, err := i.fileAcl.GetInfoForFileSharing(profile.IconImage)
		if err == nil {
			invitePayload.CreatorIconCid = iconCid
			invitePayload.CreatorIconEncryptionKeys = iconEncryptionKeys
		} else {
			log.Error("get creator icon info", zap.Error(err))
		}
	}
	return invitePayload, nil
}

func (i *inviteService) GetExistingGuestUserInvite(ctx context.Context, spaceId string) (info domain.InviteInfo, err error) {
	var fileCid, fileKey string
	var spaceType model.SpaceType
	var spaceUxType model.SpaceUxType
	err = i.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		desc := spaceView.GetSpaceDescription()
		spaceType = desc.SpaceType
		spaceUxType = desc.SpaceUxType
		return nil
	})
	if err != nil {
		return domain.InviteInfo{}, getInviteError("get space type", err)
	}
	// Check if this is a chat space (Stream in old UX type, Chat in new SpaceType)
	isChat := spaceType == model.SpaceType_SpaceTypeChat ||
		(spaceType == model.SpaceType_SpaceTypeUnknown && spaceUxType == model.SpaceUxType_Stream)
	if !isChat {
		return domain.InviteInfo{}, ErrInvalidSpaceType
	}
	err = i.doWorkspace(ctx, spaceId, func(obj domain.InviteObject) error {
		fileCid, fileKey = obj.GetExistingGuestInviteInfo()
		return nil
	})
	if err != nil {
		return domain.InviteInfo{}, generateInviteError("get existing invite info", err)
	}
	if fileCid != "" {
		return domain.InviteInfo{
			InviteFileCid: fileCid,
			InviteFileKey: fileKey,
			InviteType:    domain.InviteTypeGuest,
		}, nil
	}
	return domain.InviteInfo{}, ErrInviteNotExists
}
