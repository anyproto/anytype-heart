package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/acl"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/export"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

func (mw *Middleware) SpaceDelete(cctx context.Context, req *pb.RpcSpaceDeleteRequest) *pb.RpcSpaceDeleteResponse {
	spaceService := mw.applicationService.GetApp().MustComponent(space.CName).(space.Service)
	err := spaceService.Delete(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceDeleteResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceDeleteResponseError_NO_SUCH_SPACE),
	)
	return &pb.RpcSpaceDeleteResponse{
		Error: &pb.RpcSpaceDeleteResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceInviteGenerate(cctx context.Context, req *pb.RpcSpaceInviteGenerateRequest) *pb.RpcSpaceInviteGenerateResponse {
	aclService := getService[acl.AclService](mw)
	inviteInfo, err := aclService.GenerateInvite(cctx, req.SpaceId)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(space.ErrSpaceDeleted, pb.RpcSpaceInviteGenerateResponseError_SPACE_IS_DELETED),
			errToCode(space.ErrSpaceNotExists, pb.RpcSpaceInviteGenerateResponseError_NO_SUCH_SPACE),
			errToCode(acl.ErrPersonalSpace, pb.RpcSpaceInviteGenerateResponseError_BAD_INPUT),
			errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceInviteGenerateResponseError_REQUEST_FAILED),
		)
		return &pb.RpcSpaceInviteGenerateResponse{
			Error: &pb.RpcSpaceInviteGenerateResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteGenerateResponse{
		InviteCid:     inviteInfo.InviteFileCid,
		InviteFileKey: inviteInfo.InviteFileKey,
	}
}

func (mw *Middleware) SpaceInviteGetCurrent(cctx context.Context, req *pb.RpcSpaceInviteGetCurrentRequest) *pb.RpcSpaceInviteGetCurrentResponse {
	aclService := getService[acl.AclService](mw)
	inviteInfo, err := aclService.GetCurrentInvite(req.SpaceId)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(acl.ErrInviteNotExist, pb.RpcSpaceInviteGetCurrentResponseError_NO_ACTIVE_INVITE),
		)
		return &pb.RpcSpaceInviteGetCurrentResponse{
			Error: &pb.RpcSpaceInviteGetCurrentResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteGetCurrentResponse{
		InviteCid:     inviteInfo.InviteFileCid,
		InviteFileKey: inviteInfo.InviteFileKey,
	}
}

func (mw *Middleware) SpaceInviteRevoke(cctx context.Context, req *pb.RpcSpaceInviteRevokeRequest) *pb.RpcSpaceInviteRevokeResponse {
	return &pb.RpcSpaceInviteRevokeResponse{
		Error: &pb.RpcSpaceInviteRevokeResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
		},
	}
}

func (mw *Middleware) SpaceInviteView(cctx context.Context, req *pb.RpcSpaceInviteViewRequest) *pb.RpcSpaceInviteViewResponse {
	aclService := getService[acl.AclService](mw)
	inviteView, err := viewInvite(cctx, aclService, req)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(acl.ErrInviteBadSignature, pb.RpcSpaceInviteViewResponseError_INVITE_BAD_SIGNATURE),
		)
		return &pb.RpcSpaceInviteViewResponse{
			Error: &pb.RpcSpaceInviteViewResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteViewResponse{
		CreatorName:  inviteView.CreatorName,
		SpaceId:      inviteView.SpaceId,
		SpaceName:    inviteView.SpaceName,
		SpaceIconCid: inviteView.SpaceIconCid,
	}
}

func viewInvite(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceInviteViewRequest) (*acl.InviteView, error) {
	inviteFileKey, err := acl.DecodeKeyFromBase58(req.InviteFileKey)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	inviteCid, err := cid.Decode(req.InviteCid)
	if err != nil {
		return nil, err
	}
	return aclService.ViewInvite(ctx, inviteCid, inviteFileKey)
}

func (mw *Middleware) SpaceJoin(cctx context.Context, req *pb.RpcSpaceJoinRequest) *pb.RpcSpaceJoinResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := join(cctx, aclService, req)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceJoinResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceJoinResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceJoinResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpaceJoinResponse{
		Error: &pb.RpcSpaceJoinResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceJoinCancel(cctx context.Context, req *pb.RpcSpaceJoinCancelRequest) *pb.RpcSpaceJoinCancelResponse {
	return &pb.RpcSpaceJoinCancelResponse{
		Error: &pb.RpcSpaceJoinCancelResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
		},
	}
}

func (mw *Middleware) SpaceExit(cctx context.Context, req *pb.RpcSpaceExitRequest) *pb.RpcSpaceExitResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := aclService.Exit(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceExitResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceExitResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceExitResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpaceExitResponse{
		Error: &pb.RpcSpaceExitResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceRequestApprove(cctx context.Context, req *pb.RpcSpaceRequestApproveRequest) *pb.RpcSpaceRequestApproveResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := accept(cctx, req.SpaceId, req.Identity, req.Permissions, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceRequestApproveResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceRequestApproveResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrNoSuchUser, pb.RpcSpaceRequestApproveResponseError_NO_SUCH_IDENTITY),
		errToCode(acl.ErrIncorrectPermissions, pb.RpcSpaceRequestApproveResponseError_INCORRECT_PERMISSIONS),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceRequestApproveResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpaceRequestApproveResponse{
		Error: &pb.RpcSpaceRequestApproveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceRequestDecline(cctx context.Context, req *pb.RpcSpaceRequestDeclineRequest) *pb.RpcSpaceRequestDeclineResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := decline(cctx, req.SpaceId, req.Identity, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceRequestDeclineResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceRequestDeclineResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceRequestDeclineResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpaceRequestDeclineResponse{
		Error: &pb.RpcSpaceRequestDeclineResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceGuestAdd(cctx context.Context, req *pb.RpcSpaceGuestAddRequest) *pb.RpcSpaceGuestAddResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := addGuest(cctx, req.SpaceId, req.Identity, req.Metadata, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceGuestAddResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceGuestAddResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceGuestAddResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpaceGuestAddResponse{
		Error: &pb.RpcSpaceGuestAddResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpacePublicAdd(cctx context.Context, req *pb.RpcSpacePublicAddRequest) *pb.RpcSpacePublicAddResponse {
	spaceService := mw.applicationService.GetApp().MustComponent(space.CName).(space.Service)
	err := publicAdd(cctx, req.SpaceId, req.GuestKey, spaceService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpacePublicAddResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpacePublicAddResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpacePublicAddResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpacePublicAddResponse{
		Error: &pb.RpcSpacePublicAddResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpacePublish(cctx context.Context, req *pb.RpcSpacePublishRequest) *pb.RpcSpacePublishResponse {
	walletService := mw.applicationService.GetApp().MustComponent(wallet.CName).(wallet.Wallet)
	publishTempDir := filepath.Join(walletService.RepoPath(), "publishTemp")
	err := os.MkdirAll(publishTempDir, 0755)
	if err != nil {
		return &pb.RpcSpacePublishResponse{
			Error: &pb.RpcSpacePublishResponseError{
				Code:        1,
				Description: getErrorDescription(err),
			},
		}
	}
	defer os.RemoveAll(publishTempDir)

	err = mw.doBlockService(func(bs *block.Service) error {
		es := mw.applicationService.GetApp().MustComponent(export.CName).(export.Export)
		path, succeed, err := es.Export(cctx, pb.RpcObjectListExportRequest{SpaceId: req.FromSpace, Path: publishTempDir, Format: model.Export_Protobuf})
		if err != nil {
			return err
		}
		fmt.Printf("Exported %d objects to %s\n", succeed, path)

		return err
	})
	if err != nil {
		return &pb.RpcSpacePublishResponse{
			Error: &pb.RpcSpacePublishResponseError{
				Code:        1,
				Description: getErrorDescription(err),
			},
		}
	}

	originImport := objectorigin.Import(model.ImportType(model.Import_Pb))
	_, _, err = getService[importer.Importer](mw).Import(cctx, &pb.RpcObjectImportRequest{
		SpaceId:               req.ToSpace,
		UpdateExistingObjects: true,
		NoProgress:            false,
		Type:                  model.Import_Pb,
		Params: &pb.RpcObjectImportRequestParamsOfPbParams{
			PbParams: &pb.RpcObjectImportRequestPbParams{
				Path:         []string{publishTempDir},
				NoCollection: true,
				ImportType:   pb.RpcObjectImportRequestPbParams_SPACE,
			},
		},
	}, originImport, nil)

	if err != nil {
		return &pb.RpcSpacePublishResponse{
			Error: &pb.RpcSpacePublishResponseError{
				Code:        1,
				Description: getErrorDescription(err),
			},
		}
	}

	return &pb.RpcSpacePublishResponse{
		Error: &pb.RpcSpacePublishResponseError{
			Code:        0,
			Description: "",
		},
	}
}

func (mw *Middleware) SpaceParticipantRemove(cctx context.Context, req *pb.RpcSpaceParticipantRemoveRequest) *pb.RpcSpaceParticipantRemoveResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := remove(cctx, req.SpaceId, req.Identities, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceParticipantRemoveResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceParticipantRemoveResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceParticipantRemoveResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpaceParticipantRemoveResponse{
		Error: &pb.RpcSpaceParticipantRemoveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceParticipantPermissionsChange(cctx context.Context, req *pb.RpcSpaceParticipantPermissionsChangeRequest) *pb.RpcSpaceParticipantPermissionsChangeResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := permissionsChange(cctx, req.SpaceId, req.Changes, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceParticipantPermissionsChangeResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceParticipantPermissionsChangeResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceParticipantPermissionsChangeResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpaceParticipantPermissionsChangeResponse{
		Error: &pb.RpcSpaceParticipantPermissionsChangeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func join(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceJoinRequest) (err error) {
	inviteFileKey, err := acl.DecodeKeyFromBase58(req.InviteFileKey)
	if err != nil {
		return
	}
	inviteCid, err := cid.Decode(req.InviteCid)
	if err != nil {
		return
	}
	return aclService.Join(ctx, req.SpaceId, inviteCid, inviteFileKey)
}

func accept(ctx context.Context, spaceId, identity string, permissions model.ParticipantPermissions, aclService acl.AclService) (err error) {
	key, err := crypto.DecodeAccountAddress(identity)
	if err != nil {
		return
	}
	return aclService.Accept(ctx, spaceId, key, permissions)
}

func addGuest(ctx context.Context, spaceId, identity, metadata string, aclService acl.AclService) (err error) {
	key, err := crypto.DecodeAccountAddress(identity)
	if err != nil {
		return
	}
	bytes, err := crypto.DecodeBytesFromString(metadata)
	if err != nil {
		return
	}
	return aclService.AddGuest(ctx, spaceId, key, bytes)
}

func publicAdd(ctx context.Context, spaceId, privKey string, spaceService space.Service) (err error) {
	key, err := crypto.DecodeKeyFromString(privKey, func(bytes []byte) (crypto.PrivKey, error) {
		return crypto.NewSigningEd25519PrivKeyFromBytes(bytes)
	}, nil)
	if err != nil {
		return
	}
	return spaceService.AddPublic(ctx, spaceId, key)
}

func decline(ctx context.Context, spaceId, identity string, aclService acl.AclService) (err error) {
	key, err := crypto.DecodeAccountAddress(identity)
	if err != nil {
		return
	}
	return aclService.Decline(ctx, spaceId, key)
}

func remove(ctx context.Context, spaceId string, identities []string, aclService acl.AclService) error {
	keys := make([]crypto.PubKey, 0, len(identities))
	for _, identity := range identities {
		key, err := crypto.DecodeAccountAddress(identity)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	}
	return aclService.Remove(ctx, spaceId, keys)
}

func permissionsChange(ctx context.Context, spaceId string, changes []*model.ParticipantPermissionChange, aclService acl.AclService) error {
	accPermissions := make([]acl.AccountPermissions, 0, len(changes))
	for _, change := range changes {
		key, err := crypto.DecodeAccountAddress(change.Identity)
		if err != nil {
			return err
		}
		accPermissions = append(accPermissions, acl.AccountPermissions{
			Account:     key,
			Permissions: change.Perms,
		})
	}
	return aclService.ChangePermissions(ctx, spaceId, accPermissions)
}
