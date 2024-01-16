package core

import (
	"context"

	"github.com/anyproto/any-sync/util/crypto"

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
	key, err := generateInvite(cctx, req.SpaceId, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceInviteGenerateResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceInviteGenerateResponseError_NO_SUCH_SPACE),
	)
	return &pb.RpcSpaceInviteGenerateResponse{
		InviteKey: key,
		Error: &pb.RpcSpaceInviteGenerateResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceRequestJoin(cctx context.Context, req *pb.RpcSpaceRequestJoinRequest) *pb.RpcSpaceRequestJoinResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := join(cctx, req.SpaceId, req.PrivateKey, aclService)
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

func generateInvite(ctx context.Context, spaceId string, aclService acl.AclService) (encKey string, err error) {
	key, err := aclService.GenerateInvite(ctx, spaceId)
	if err != nil {
		return
	}
	return crypto.EncodeKeyToString(key)
}

func join(ctx context.Context, spaceId, encKey string, aclService acl.AclService) (err error) {
	key, err := crypto.DecodeKeyFromString(encKey, func(bytes []byte) (crypto.PrivKey, error) {
		return crypto.NewSigningEd25519PrivKeyFromBytes(bytes)
	}, nil)
	if err != nil {
		return
	}
	return aclService.Join(ctx, spaceId, key)
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
