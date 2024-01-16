package core

import (
	"context"

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

func (mw *Middleware) SpaceRequestJoin(cctx context.Context, req *pb.RpcSpaceRequestJoinRequest) *pb.RpcSpaceRequestJoinResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := join(cctx, aclService, req)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceRequestJoinResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceRequestJoinResponseError_NO_SUCH_SPACE),
	)
	return &pb.RpcSpaceRequestJoinResponse{
		Error: &pb.RpcSpaceRequestJoinResponseError{
			Code:        code,
			Description: getErrorDescription(err),
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

func join(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceRequestJoinRequest) (err error) {
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
