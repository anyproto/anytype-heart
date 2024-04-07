package inviteservice

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/core/invitestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/encode"
)

var (
	ErrInviteNotExists    = errors.New("invite not exists")
	ErrInviteBadSignature = errors.New("invite bad signature")
	ErrPersonalSpace      = errors.New("personal space")
)

type InviteInfo struct {
	InviteFileCid string
	InviteFileKey string
}

const CName = "common.core.inviteservice"

var log = logger.NewNamed(CName)

type InviteView struct {
	SpaceId      string
	SpaceName    string
	SpaceIconCid string
	CreatorName  string
}

type InviteService interface {
	app.ComponentRunnable
	GetPayload(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (*model.InvitePayload, error)
	View(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (InviteView, error)
	RemoveExisting(ctx context.Context, spaceId string) error
	Generate(ctx context.Context, spaceId string, inviteKey crypto.PrivKey, sendInvite func() error) (InviteInfo, error)
	GetCurrent(ctx context.Context, spaceId string) (InviteInfo, error)
}

var _ InviteService = (*inviteService)(nil)

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

func (i *inviteService) View(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (InviteView, error) {
	invitePayload, err := i.GetPayload(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return InviteView{}, fmt.Errorf("get invite payload: %w", err)
	}
	return InviteView{
		SpaceId:      invitePayload.SpaceId,
		SpaceName:    invitePayload.SpaceName,
		SpaceIconCid: invitePayload.SpaceIconCid,
		CreatorName:  invitePayload.CreatorName,
	}, nil
}

func (i *inviteService) GetCurrent(ctx context.Context, spaceId string) (info InviteInfo, err error) {
	var (
		fileCid, fileKey string
	)
	err = i.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		fileCid, fileKey = spaceView.GetExistingInviteInfo()
		return nil
	})
	if err != nil {
		err = fmt.Errorf("get existing invite file info: %w", err)
		return
	}
	if fileCid == "" {
		err = ErrInviteNotExists
		return
	}
	info.InviteFileCid = fileCid
	info.InviteFileKey = fileKey
	return
}

func (i *inviteService) RemoveExisting(ctx context.Context, spaceId string) (err error) {
	var fileCid string
	err = i.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		fileCid, err = spaceView.RemoveExistingInviteInfo()
		return err
	})
	if err != nil {
		return fmt.Errorf("remove existing invite: %w", err)
	}
	invCid, err := cid.Decode(fileCid)
	if err != nil {
		return fmt.Errorf("decode file cid: %w", err)
	}
	return i.inviteStore.RemoveInvite(ctx, invCid)
}

func (i *inviteService) Generate(ctx context.Context, spaceId string, inviteKey crypto.PrivKey, sendInvite func() error) (result InviteInfo, err error) {
	if spaceId == i.accountService.PersonalSpaceID() {
		return InviteInfo{}, ErrPersonalSpace
	}
	var fileCid, fileKey string
	err = i.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		fileCid, fileKey = spaceView.GetExistingInviteInfo()
		return nil
	})
	if err != nil {
		return InviteInfo{}, fmt.Errorf("get space view id: %w", err)
	}
	if fileCid != "" {
		return InviteInfo{
			InviteFileCid: fileCid,
			InviteFileKey: fileKey,
		}, nil
	}
	invite, err := i.buildInvite(ctx, spaceId, inviteKey)
	if err != nil {
		return InviteInfo{}, fmt.Errorf("build invite: %w", err)
	}
	inviteFileCid, inviteFileKey, err := i.inviteStore.StoreInvite(ctx, invite)
	if err != nil {
		return InviteInfo{}, fmt.Errorf("store invite in ipfs: %w", err)
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
		return InviteInfo{}, fmt.Errorf("encode invite file key: %w", err)
	}
	err = i.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.SetInviteFileInfo(inviteFileCid.String(), inviteFileKeyRaw)
	})
	if err != nil {
		removeInviteFile()
		return InviteInfo{}, fmt.Errorf("set invite file info: %w", err)
	}
	err = sendInvite()
	if err != nil {
		_ = i.RemoveExisting(ctx, spaceId)
		return InviteInfo{}, fmt.Errorf("failed to send invite: %w", err)
	}
	return InviteInfo{
		InviteFileCid: inviteFileCid.String(),
		InviteFileKey: inviteFileKeyRaw,
	}, err
}

func (i *inviteService) buildInvite(ctx context.Context, spaceId string, inviteKey crypto.PrivKey) (*model.Invite, error) {
	invitePayload, err := i.buildInvitePayload(ctx, spaceId, inviteKey)
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

func (i *inviteService) buildInvitePayload(ctx context.Context, spaceId string, inviteKey crypto.PrivKey) (*model.InvitePayload, error) {
	profile, err := i.accountService.ProfileInfo()
	if err != nil {
		return nil, fmt.Errorf("get profile info: %w", err)
	}
	rawInviteKey, err := inviteKey.Marshall()
	if err != nil {
		return nil, fmt.Errorf("marshal invite priv key: %w", err)
	}
	invitePayload := &model.InvitePayload{
		SpaceId:         spaceId,
		CreatorIdentity: i.accountService.AccountID(),
		CreatorName:     profile.Name,
		InviteKey:       rawInviteKey,
	}
	var description spaceinfo.SpaceDescription
	err = i.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		description = spaceView.GetSpaceDescription()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get space description: %w", err)
	}
	if description.IconImage != "" {
		iconCid, iconEncryptionKeys, err := i.fileAcl.GetInfoForFileSharing(ctx, description.IconImage)
		if err == nil {
			invitePayload.SpaceIconCid = iconCid
			invitePayload.SpaceIconEncryptionKeys = iconEncryptionKeys
		} else {
			log.Error("get space icon info", zap.Error(err))
		}
	}
	return invitePayload, nil
}

func (i *inviteService) GetPayload(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (*model.InvitePayload, error) {
	invite, err := i.inviteStore.GetInvite(ctx, inviteCid, inviteFileKey)
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
	err = i.fileAcl.StoreFileKeys(domain.FileId(invitePayload.SpaceIconCid), invitePayload.SpaceIconEncryptionKeys)
	if err != nil {
		return nil, fmt.Errorf("store icon keys: %w", err)
	}
	return &invitePayload, nil
}
