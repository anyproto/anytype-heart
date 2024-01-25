package core

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/acl"
	"github.com/anyproto/anytype-heart/pb"
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
	return &pb.RpcSpaceExitResponse{
		Error: &pb.RpcSpaceExitResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
		},
	}
}

func (mw *Middleware) SpaceRequestApprove(cctx context.Context, req *pb.RpcSpaceRequestApproveRequest) *pb.RpcSpaceRequestApproveResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := accept(cctx, req.SpaceId, req.Identity, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceRequestApproveResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceRequestApproveResponseError_NO_SUCH_SPACE),
	)
	return &pb.RpcSpaceRequestApproveResponse{
		Error: &pb.RpcSpaceRequestApproveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceRequestDecline(cctx context.Context, req *pb.RpcSpaceRequestDeclineRequest) *pb.RpcSpaceRequestDeclineResponse {
	return &pb.RpcSpaceRequestDeclineResponse{
		Error: &pb.RpcSpaceRequestDeclineResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
		},
	}
}

func (mw *Middleware) SpaceParticipantRemove(cctx context.Context, req *pb.RpcSpaceParticipantRemoveRequest) *pb.RpcSpaceParticipantRemoveResponse {
	return &pb.RpcSpaceParticipantRemoveResponse{
		Error: &pb.RpcSpaceParticipantRemoveResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
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

func accept(ctx context.Context, spaceId, identity string, aclService acl.AclService) (err error) {
	key, err := crypto.DecodeAccountAddress(identity)
	if err != nil {
		return
	}
	return aclService.Accept(ctx, spaceId, key)
}
