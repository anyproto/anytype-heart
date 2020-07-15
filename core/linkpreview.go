package core

import (
	"context"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
)

func (mw *Middleware) LinkPreview(req *pb.RpcLinkPreviewRequest) *pb.RpcLinkPreviewResponse {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	url, _ := uri.ProcessURI(req.Url)
	data, err := mw.linkPreview.Fetch(ctx, url)
	if err != nil {
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code:        pb.RpcLinkPreviewResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
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
