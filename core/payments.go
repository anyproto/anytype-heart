package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/payments"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) MembershipGetStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) *pb.RpcMembershipGetStatusResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetSubscriptionStatus(ctx, req)

	if err != nil {
		return &pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{
				Code:        pb.RpcMembershipGetStatusResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipGetPaymentUrl(ctx context.Context, req *pb.RpcMembershipGetPaymentUrlRequest) *pb.RpcMembershipGetPaymentUrlResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetPaymentURL(ctx, req)

	if err != nil {
		return &pb.RpcMembershipGetPaymentUrlResponse{
			Error: &pb.RpcMembershipGetPaymentUrlResponseError{
				Code:        pb.RpcMembershipGetPaymentUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipGetPortalLinkUrl(ctx context.Context, req *pb.RpcMembershipGetPortalLinkUrlRequest) *pb.RpcMembershipGetPortalLinkUrlResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetPortalLink(ctx, req)

	if err != nil {
		return &pb.RpcMembershipGetPortalLinkUrlResponse{
			Error: &pb.RpcMembershipGetPortalLinkUrlResponseError{
				Code:        pb.RpcMembershipGetPortalLinkUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipGetVerificationEmail(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) *pb.RpcMembershipGetVerificationEmailResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetVerificationEmail(ctx, req)

	if err != nil {
		return &pb.RpcMembershipGetVerificationEmailResponse{
			Error: &pb.RpcMembershipGetVerificationEmailResponseError{
				Code:        pb.RpcMembershipGetVerificationEmailResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipVerifyEmailCode(ctx context.Context, req *pb.RpcMembershipVerifyEmailCodeRequest) *pb.RpcMembershipVerifyEmailCodeResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.VerifyEmailCode(ctx, req)

	if err != nil {
		return &pb.RpcMembershipVerifyEmailCodeResponse{
			Error: &pb.RpcMembershipVerifyEmailCodeResponseError{
				Code:        pb.RpcMembershipVerifyEmailCodeResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipFinalize(ctx context.Context, req *pb.RpcMembershipFinalizeRequest) *pb.RpcMembershipFinalizeResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.FinalizeSubscription(ctx, req)

	if err != nil {
		return &pb.RpcMembershipFinalizeResponse{
			Error: &pb.RpcMembershipFinalizeResponseError{
				Code:        pb.RpcMembershipFinalizeResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipGetTiers(ctx context.Context, req *pb.RpcMembershipTiersGetRequest) *pb.RpcMembershipTiersGetResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetTiers(ctx, req)

	if err != nil {
		return &pb.RpcMembershipTiersGetResponse{
			Error: &pb.RpcMembershipTiersGetResponseError{
				Code:        pb.RpcMembershipTiersGetResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}
