package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/linkpreview"
	"github.com/anyproto/anytype-heart/util/uri"
)

func (mw *Middleware) LinkPreview(cctx context.Context, req *pb.RpcLinkPreviewRequest) *pb.RpcLinkPreviewResponse {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	u, err := uri.NormalizeAndParseURI(req.Url)
	if err != nil {
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code:        pb.RpcLinkPreviewResponseError_UNKNOWN_ERROR,
				Description: fmt.Sprintf("failed to parse url: %v", err),
			},
		}
	}

	if mw.applicationService.GetApp() == nil {
		return &pb.RpcLinkPreviewResponse{
			Error: &pb.RpcLinkPreviewResponseError{
				Code: pb.RpcLinkPreviewResponseError_UNKNOWN_ERROR,
			},
		}
	}
	lp := mw.applicationService.GetApp().MustComponent(linkpreview.CName).(linkpreview.LinkPreview)
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
