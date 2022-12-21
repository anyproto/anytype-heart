package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
)

func (mw *Middleware) LinkPreview(cctx context.Context, req *pb.RpcLinkPreviewRequest) *pb.RpcLinkPreviewResponse {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	urlStr, err := uri.URIManager.ValidateAndNormalizeURI(req.Url)
	if err != nil {
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code:        pb.RpcLinkPreviewResponseError_UNKNOWN_ERROR,
				Description: fmt.Sprintf("failed to parse url: %v", err),
			},
		}
	}
	u := uri.URIManager.ParseURI(urlStr)

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
	data, err := lp.Fetch(ctx, u.String())
	if err != nil {
		// trim the actual url from the error
		errTrimmed := strings.Replace(err.Error(), u.String(), "<url>", -1)
		errTrimmed = strings.Replace(errTrimmed, u.Hostname(), "<host>", -1) // in case of dns errors

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
