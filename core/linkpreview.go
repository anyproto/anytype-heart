package core

import (
	"context"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
)

func (mw *Middleware) LinkPreview(req *pb.RpcLinkPreviewRequest) *pb.RpcLinkPreviewResponse {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
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

	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code: pb.RpcLinkPreviewResponseError_UNKNOWN_ERROR,
			},
		}
	}
	lp := mw.app.MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
	data, err := lp.Fetch(ctx, url)
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
