package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/payments"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) PaymentsSubscriptionGetStatus(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetStatusRequest) *pb.RpcPaymentsSubscriptionGetStatusResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetSubscriptionStatus(ctx, req)

	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetStatusResponse{
			Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) PaymentsSubscriptionGetPaymentUrl(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) *pb.RpcPaymentsSubscriptionGetPaymentUrlResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetPaymentURL(ctx, req)

	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) PaymentsSubscriptionGetPortalLinkUrl(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) *pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetPortalLink(ctx, req)

	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) PaymentsSubscriptionGetVerificationEmail(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetVerificationEmailRequest) *pb.RpcPaymentsSubscriptionGetVerificationEmailResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetVerificationEmail(ctx, req)

	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetVerificationEmailResponse{
			Error: &pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) PaymentsSubscriptionVerifyEmailCode(ctx context.Context, req *pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest) *pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.VerifyEmailCode(ctx, req)

	if err != nil {
		return &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse{
			Error: &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError{
				Code:        pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) PaymentsSubscriptionFinalize(ctx context.Context, req *pb.RpcPaymentsSubscriptionFinalizeRequest) *pb.RpcPaymentsSubscriptionFinalizeResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.FinalizeSubscription(ctx, req)

	if err != nil {
		return &pb.RpcPaymentsSubscriptionFinalizeResponse{
			Error: &pb.RpcPaymentsSubscriptionFinalizeResponseError{
				Code:        pb.RpcPaymentsSubscriptionFinalizeResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) PaymentsGetTiers(ctx context.Context, req *pb.RpcPaymentsTiersGetRequest) *pb.RpcPaymentsTiersGetResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetTiers(ctx, req)

	if err != nil {
		return &pb.RpcPaymentsTiersGetResponse{
			Error: &pb.RpcPaymentsTiersGetResponseError{
				Code:        pb.RpcPaymentsTiersGetResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return out
}
