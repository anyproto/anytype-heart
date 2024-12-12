package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/contact"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ContactCreate(cctx context.Context, req *pb.RpcContactCreateRequest) *pb.RpcContactCreateResponse {
	contactService := getService[contact.Service](mw)
	err := contactService.SaveContact(cctx, req.Identity, req.ProfileSymKey)
	code := mapErrorCode[pb.RpcContactCreateResponseErrorCode](err)
	return &pb.RpcContactCreateResponse{
		Error: &pb.RpcContactCreateResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
