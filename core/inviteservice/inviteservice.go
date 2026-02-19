package inviteservice

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
	RemoveExisting(ctx context.Context, spaceId string) error
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
		SpaceUxType:     model.SpaceUxType(invitePayload.SpaceUxType),
		SpaceIconOption: int(invitePayload.SpaceIconOption),
		CreatorName:     invitePayload.CreatorName,
		CreatorIconCid:  invitePayload.CreatorIconCid,
		AclKey:          invitePayload.AclKey,
		GuestKey:        invitePayload.GuestKey,
		InviteType:      domain.InviteType(invitePayload.InviteType),
	}, nil
}

func (i *inviteService) Change(ctx context.Context, spaceId string, permissions list.AclPermissions) error {
	return i.doInviteObject(ctx, spaceId, func(obj domain.InviteObject) error {
		info := obj.GetExistingInviteInfo()
		info.Permissions = permissions
		return obj.SetInviteFileInfo(info)
	})
}

func (i *inviteService) GetCurrent(ctx context.Context, spaceId string) (info domain.InviteInfo, err error) {
	err = i.doInviteObject(ctx, spaceId, func(obj domain.InviteObject) error {
		info = obj.GetExistingInviteInfo()
		return nil
	})
	if err != nil {
		err = getInviteError("get existing invite info", err)
		return
	}
	if info.InviteFileCid == "" {
		err = ErrInviteNotExists
		return
	}
	return
}

func (i *inviteService) RemoveExisting(ctx context.Context, spaceId string) (err error) {
	var info domain.InviteInfo
	err = i.doInviteObject(ctx, spaceId, func(obj domain.InviteObject) error {
		info, err = obj.RemoveExistingInviteInfo()
		return err
	})
	if err != nil {
		return removeInviteError("remove existing invite info", err)
	}
	if len(info.InviteFileCid) == 0 {
		return nil
	}
	invCid, err := cid.Decode(info.InviteFileCid)
	if err != nil {
		return removeInviteError("decode invite cid", err)
	}
	err = i.inviteStore.RemoveInvite(ctx, invCid)
	if err != nil {
		return removeInviteError("remove invite from store", err)
	}
	return
}

func (i *inviteService) doInviteObject(ctx context.Context, spaceId string, f func(object domain.InviteObject) error) error {
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

func (i *inviteService) GenerateGuestUserInvite(ctx context.Context, spaceId string, guestUserKey crypto.PrivKey) (domain.InviteInfo, error) {
	return i.generateGuestInvite(ctx, spaceId, guestUserKey)
}

func (i *inviteService) Generate(ctx context.Context, params GenerateInviteParams, sendInvite func() error) (result domain.InviteInfo, err error) {
	spaceId := params.SpaceId
	if spaceId == i.accountService.PersonalSpaceID() {
		return domain.InviteInfo{}, ErrPersonalSpace
	}
	err = i.doInviteObject(ctx, spaceId, func(obj domain.InviteObject) error {
		result = obj.GetExistingInviteInfo()
		return nil
	})
	if err != nil {
		return domain.InviteInfo{}, generateInviteError("get existing invite info", err)
	}
	if result.InviteFileCid != "" && result.InviteType == params.InviteType {
		return result, nil
	}
	invite, err := i.buildInvite(ctx, params)
	if err != nil {
		return domain.InviteInfo{}, generateInviteError("build invite", err)
	}
	inviteFileCid, inviteFileKey, err := i.inviteStore.StoreInvite(ctx, invite)
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
	}
	err = i.doInviteObject(ctx, spaceId, func(obj domain.InviteObject) error {
		return obj.SetInviteFileInfo(inviteInfo)
	})
	if err != nil {
		removeInviteFile()
		return domain.InviteInfo{}, generateInviteError("set invite file info", err)
	}
	err = sendInvite()
	if err != nil {
		removeErr := i.RemoveExisting(ctx, spaceId)
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
	inviteFileCid, inviteFileKey, err := i.inviteStore.StoreInvite(ctx, invite)
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
	err = i.doInviteObject(ctx, spaceId, func(obj domain.InviteObject) error {
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
	invitePayload.SpaceUxType = uint32(description.SpaceUxType)
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
	var spaceType model.SpaceUxType
	err = i.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		spaceType = spaceView.GetSpaceDescription().SpaceUxType
		return nil
	})
	if err != nil {
		return domain.InviteInfo{}, getInviteError("get space type", err)
	}
	if spaceType != model.SpaceUxType_Stream {
		return domain.InviteInfo{}, ErrInvalidSpaceType
	}
	err = i.doInviteObject(ctx, spaceId, func(obj domain.InviteObject) error {
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
