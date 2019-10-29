package core

import (
	"context"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	linkPreviewTimeout = time.Second * 5
)

func (mw *Middleware) LinkPreview(req *pb.LinkPreviewRequest) *pb.LinkPreviewResponse {
	ctx, cancel := context.WithTimeout(context.Background(), linkPreviewTimeout)
	defer cancel()
	resp, err := mw.linkPreview.Fetch(ctx, req.Url)
	if err != nil {
		if err == context.DeadlineExceeded {
			return &pb.LinkPreviewResponse{Error: &pb.LinkPreviewResponse_Error{
				Code: pb.LinkPreviewResponse_Error_TIMEOUT,
			}}
		}
		return &pb.LinkPreviewResponse{Error: &pb.LinkPreviewResponse_Error{
			Code: pb.LinkPreviewResponse_Error_UNKNOWN_ERROR,
		}}
	}
	return &resp
}
