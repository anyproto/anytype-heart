package core

import (
	"context"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
)

func (mw *Middleware) LinkPreview(req *pb.RpcLinkPreviewRequest) *pb.RpcLinkPreviewResponse {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	url, err := uri.ProcessURI(req.Url)
	if err != nil {
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code:        pb.RpcLinkPreviewResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}
	
	data, err := mw.linkPreview.Fetch(ctx, url)
	if err != nil {
		// trim the actual url from the error
		errTrimmed := strings.Replace(err.Error(), url, "<url>", -1)
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code:        pb.RpcLinkPreviewResponseError_UNKNOWN_ERROR,
				Description: errTrimmed,
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
