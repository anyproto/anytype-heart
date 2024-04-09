package core

import (
	"context"

	"go.uber.org/zap"

	"github.com/anyproto/any-sync/net"
	proto "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/payments"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) MembershipGetStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) *pb.RpcMembershipGetStatusResponse {
	log.Info("payments - client asked to get a subscription status", zap.Any("req", req))

	ps := getService[payments.Service](mw)
	out, err := ps.GetSubscriptionStatus(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipGetStatusResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipGetStatusResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetStatusResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetStatusResponseError_CACHE_ERROR),

			errToCode(proto.ErrSubsNotFound, pb.RpcMembershipGetStatusResponseError_MEMBERSHIP_NOT_FOUND),
			errToCode(proto.ErrSubsWrongState, pb.RpcMembershipGetStatusResponseError_MEMBERSHIP_WRONG_STATE),
		)
		return &pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{
				Code:        code,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipIsNameValid(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) *pb.RpcMembershipIsNameValidResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.IsNameValid(ctx, req)

	// out will already contain validation Error
	// but if something bad has happened we need to process other errors here too:
	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipIsNameValidResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipIsNameValidResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipIsNameValidResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipIsNameValidResponseError_CACHE_ERROR),

			errToCode(payments.ErrNoTiers, pb.RpcMembershipIsNameValidResponseError_TIER_NOT_FOUND),
			errToCode(payments.ErrNoTierFound, pb.RpcMembershipIsNameValidResponseError_TIER_NOT_FOUND),
		)
		return &pb.RpcMembershipIsNameValidResponse{
			Error: &pb.RpcMembershipIsNameValidResponseError{
				Code:        code,
				Description: err.Error(),
			},
		}
	}

	// out.Error will contain validation error if something is wrong with the name
	return out
}

func (mw *Middleware) MembershipGetPaymentUrl(ctx context.Context, req *pb.RpcMembershipGetPaymentUrlRequest) *pb.RpcMembershipGetPaymentUrlResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetPaymentURL(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipGetPaymentUrlResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipGetPaymentUrlResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetPaymentUrlResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipGetPaymentUrlResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetPaymentUrlResponseError_CACHE_ERROR),

			errToCode(proto.ErrTierNotFound, pb.RpcMembershipGetPaymentUrlResponseError_TIER_NOT_FOUND),
			errToCode(proto.ErrTierWrong, pb.RpcMembershipGetPaymentUrlResponseError_TIER_INVALID),
			errToCode(proto.ErrPaymentMethodWrong, pb.RpcMembershipGetPaymentUrlResponseError_PAYMENT_METHOD_INVALID),
			errToCode(proto.ErrBadAnyName, pb.RpcMembershipGetPaymentUrlResponseError_BAD_ANYNAME),
			errToCode(proto.ErrSubsAlreadyActive, pb.RpcMembershipGetPaymentUrlResponseError_MEMBERSHIP_ALREADY_EXISTS),
		)
		return &pb.RpcMembershipGetPaymentUrlResponse{
			Error: &pb.RpcMembershipGetPaymentUrlResponseError{
				Code:        code,
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
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipGetPortalLinkUrlResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipGetPortalLinkUrlResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetPortalLinkUrlResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipGetPortalLinkUrlResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetPortalLinkUrlResponseError_CACHE_ERROR),
		)
		return &pb.RpcMembershipGetPortalLinkUrlResponse{
			Error: &pb.RpcMembershipGetPortalLinkUrlResponseError{
				Code:        code,
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
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipGetVerificationEmailResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipGetVerificationEmailResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetVerificationEmailResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipGetVerificationEmailResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetVerificationEmailResponseError_CACHE_ERROR),

			errToCode(proto.ErrEmailWrongFormat, pb.RpcMembershipGetVerificationEmailResponseError_EMAIL_WRONG_FORMAT),
			errToCode(proto.ErrEmailAlreadyVerified, pb.RpcMembershipGetVerificationEmailResponseError_EMAIL_ALREADY_VERIFIED),
			errToCode(proto.ErrEmailAlreadySent, pb.RpcMembershipGetVerificationEmailResponseError_EMAIL_ALREDY_SENT),
			errToCode(proto.ErrEmailFailedToSend, pb.RpcMembershipGetVerificationEmailResponseError_EMAIL_FAILED_TO_SEND),
			errToCode(proto.ErrSubsAlreadyActive, pb.RpcMembershipGetVerificationEmailResponseError_MEMBERSHIP_ALREADY_EXISTS),
		)

		return &pb.RpcMembershipGetVerificationEmailResponse{
			Error: &pb.RpcMembershipGetVerificationEmailResponseError{
				Code:        code,
				Description: err.Error(),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipGetVerificationEmailStatus(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailStatusRequest) *pb.RpcMembershipGetVerificationEmailStatusResponse {
	// TODO:
	return &pb.RpcMembershipGetVerificationEmailStatusResponse{
		Error: &pb.RpcMembershipGetVerificationEmailStatusResponseError{
			Code:        pb.RpcMembershipGetVerificationEmailStatusResponseError_UNKNOWN_ERROR,
			Description: "TODO - not implemented yet",
		},
	}
}

func (mw *Middleware) MembershipVerifyEmailCode(ctx context.Context, req *pb.RpcMembershipVerifyEmailCodeRequest) *pb.RpcMembershipVerifyEmailCodeResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.VerifyEmailCode(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipVerifyEmailCodeResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipVerifyEmailCodeResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipVerifyEmailCodeResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipVerifyEmailCodeResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipVerifyEmailCodeResponseError_CACHE_ERROR),

			errToCode(proto.ErrEmailAlreadyVerified, pb.RpcMembershipVerifyEmailCodeResponseError_EMAIL_ALREADY_VERIFIED),
			errToCode(proto.ErrEmailExpired, pb.RpcMembershipVerifyEmailCodeResponseError_CODE_EXPIRED),
			errToCode(proto.ErrEmailWrongCode, pb.RpcMembershipVerifyEmailCodeResponseError_CODE_WRONG),
			errToCode(proto.ErrSubsNotFound, pb.RpcMembershipVerifyEmailCodeResponseError_MEMBERSHIP_NOT_FOUND),
			errToCode(proto.ErrSubsAlreadyActive, pb.RpcMembershipVerifyEmailCodeResponseError_MEMBERSHIP_ALREADY_ACTIVE),
		)

		return &pb.RpcMembershipVerifyEmailCodeResponse{
			Error: &pb.RpcMembershipVerifyEmailCodeResponseError{
				Code:        code,
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
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipFinalizeResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipFinalizeResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipFinalizeResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipFinalizeResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipFinalizeResponseError_CACHE_ERROR),

			errToCode(proto.ErrSubsNotFound, pb.RpcMembershipFinalizeResponseError_MEMBERSHIP_NOT_FOUND),
			errToCode(proto.ErrSubsWrongState, pb.RpcMembershipFinalizeResponseError_MEMBERSHIP_WRONG_STATE),

			errToCode(proto.ErrBadAnyName, pb.RpcMembershipFinalizeResponseError_BAD_ANYNAME),
		)

		return &pb.RpcMembershipFinalizeResponse{
			Error: &pb.RpcMembershipFinalizeResponseError{
				Code:        code,
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
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipTiersGetResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipTiersGetResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipTiersGetResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipTiersGetResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipTiersGetResponseError_CACHE_ERROR),
		)

		return &pb.RpcMembershipTiersGetResponse{
			Error: &pb.RpcMembershipTiersGetResponseError{
				Code:        code,
				Description: err.Error(),
			},
		}
	}

	return out
}
