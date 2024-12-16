package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/contact"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ContactCreate(cctx context.Context, req *pb.RpcContactCreateRequest) *pb.RpcContactCreateResponse {
	contactService, err := getService[contact.Service](mw)
	if err != nil {
		return &pb.RpcContactCreateResponse{
			Error: &pb.RpcContactCreateResponseError{
				Code:        mapErrorCode[pb.RpcContactCreateResponseErrorCode](err),
				Description: getErrorDescription(err),
			},
		}
	}
	err = contactService.SaveContact(cctx, req.Identity, req.ProfileSymKey)
	return &pb.RpcContactCreateResponse{
		Error: &pb.RpcContactCreateResponseError{
			Code:        mapErrorCode[pb.RpcContactCreateResponseErrorCode](err),
			Description: getErrorDescription(err),
		},
	}
}
