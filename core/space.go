package core

import (
	"context"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
)

func (mw *Middleware) SpaceDelete(cctx context.Context, req *pb.RpcSpaceDeleteRequest) *pb.RpcSpaceDeleteResponse {
	spaceService := mw.applicationService.GetApp().MustComponent(space.CName).(space.SpaceService)
	_, err := spaceService.Delete(cctx, req.SpaceId)
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

func (mw *Middleware) SpaceOffload(cctx context.Context, req *pb.RpcSpaceOffloadRequest) *pb.RpcSpaceOffloadResponse {
	spaceService := mw.applicationService.GetApp().MustComponent(space.CName).(space.SpaceService)
	err := spaceService.Offload(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceOffloadResponseError_NO_SUCH_SPACE),
	)
	return &pb.RpcSpaceOffloadResponse{
		Error: &pb.RpcSpaceOffloadResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceDownload(cctx context.Context, req *pb.RpcSpaceDownloadRequest) *pb.RpcSpaceDownloadResponse {
	spaceService := mw.applicationService.GetApp().MustComponent(space.CName).(space.SpaceService)
	err := spaceService.Download(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceDownloadResponseError_NO_SUCH_SPACE),
	)
	return &pb.RpcSpaceDownloadResponse{
		Error: &pb.RpcSpaceDownloadResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
