package core

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/linkpreview"
	"github.com/anyproto/anytype-heart/util/uri"
)

func (mw *Middleware) LinkPreview(cctx context.Context, req *pb.RpcLinkPreviewRequest) *pb.RpcLinkPreviewResponse {
	ctx, cancel := context.WithTimeout(cctx, time.Second*5)
	defer cancel()

	u, err := uri.NormalizeAndParseURI(req.Url)
	if err != nil {
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code:        pb.RpcLinkPreviewResponseError_UNKNOWN_ERROR,
				Description: fmt.Sprintf("failed to parse url: %v", getErrorDescription(err)),
			},
		}
	}

	data, _, _, err := mustService[linkpreview.LinkPreview](mw).Fetch(ctx, u.String())
	if err != nil {
		code := mapErrorCode(err,
			errToCode(linkpreview.ErrPrivateLink, pb.RpcLinkPreviewResponseError_PRIVATE_LINK),
		)
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return &pb.RpcLinkPreviewResponse{
		Error: &pb.RpcLinkPreviewResponseError{
			Code: pb.RpcLinkPreviewResponseError_NULL,
		},
		LinkPreview: &data,
	}
}
