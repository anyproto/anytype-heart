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
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	inviteCid, inviteFileKey, err := generateInvite(cctx, req.SpaceId, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceInviteGenerateResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceInviteGenerateResponseError_NO_SUCH_SPACE),
	)
	return &pb.RpcSpaceInviteGenerateResponse{
		InviteCid:     inviteCid,
		InviteFileKey: inviteFileKey,
		Error: &pb.RpcSpaceInviteGenerateResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceInviteGetCurrent(cctx context.Context, req *pb.RpcSpaceInviteGetCurrentRequest) *pb.RpcSpaceInviteGetCurrentResponse {
	return &pb.RpcSpaceInviteGetCurrentResponse{
		Error: &pb.RpcSpaceInviteGetCurrentResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
		},
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
	return &pb.RpcSpaceInviteViewResponse{
		Error: &pb.RpcSpaceInviteViewResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
		},
	}
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
	spaceService := mw.applicationService.GetApp().MustComponent(space.CName).(space.Service)
	err := spaceService.Delete(cctx, req.SpaceId)
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

func generateInvite(ctx context.Context, spaceId string, aclService acl.AclService) (inviteCid string, inviteFilekey string, err error) {
	res, err := aclService.GenerateInvite(ctx, spaceId)
	if err != nil {
		return
	}
	inviteFileKey, err := crypto.EncodeKeyToString(res.InviteFileKey)
	if err != nil {
		return
	}
	return res.InviteFileCid.String(), inviteFileKey, nil
}

func join(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceJoinRequest) (err error) {
	inviteFileKey, err := crypto.DecodeKeyFromString(req.InviteFileKey, func(bytes []byte) (crypto.SymKey, error) {
		return crypto.UnmarshallAESKey(bytes)
	}, nil)
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
	key, err := crypto.DecodeKeyFromString(identity, func(bytes []byte) (crypto.PubKey, error) {
		return crypto.NewSigningEd25519PubKeyFromBytes(bytes)
	}, nil)
	if err != nil {
		return
	}
	return aclService.Accept(ctx, spaceId, key)
}
