package core

import (
	"context"

	"go.uber.org/zap"

	"github.com/anyproto/any-sync/net"
	proto "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/payments"
	"github.com/anyproto/anytype-heart/pb"
)

// Semantics in case of NO INTERNET:
//
// If called with req.NoCache -> returns error
// If called without req.NoCache:
//
//	has no fresh data -> returns error
//	has fresh data -> returns data
func (mw *Middleware) MembershipGetStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) *pb.RpcMembershipGetStatusResponse {
	log.Info("payments - client asked to get a subscription status", zap.Any("req", req))

	ps := getService[payments.Service](mw)
	out, err := ps.GetSubscriptionStatus(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipGetStatusResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipGetStatusResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetStatusResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetStatusResponseError_CACHE_ERROR),

			errToCode(proto.ErrSubsNotFound, pb.RpcMembershipGetStatusResponseError_MEMBERSHIP_NOT_FOUND),
			errToCode(proto.ErrSubsWrongState, pb.RpcMembershipGetStatusResponseError_MEMBERSHIP_WRONG_STATE),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipGetStatusResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := err.Error()
		if code == pb.RpcMembershipGetStatusResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipIsNameValid(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) *pb.RpcMembershipIsNameValidResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.IsNameValid(ctx, req)

	// 1 - check the validity first (remote call #1)
	// out will already contain validation Error
	// but if something bad has happened we need to process other errors here too:
	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipIsNameValidResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipIsNameValidResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipIsNameValidResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipIsNameValidResponseError_CACHE_ERROR),
			errToCode(payments.ErrNameIsAlreadyReserved, pb.RpcMembershipIsNameValidResponseError_NAME_IS_RESERVED),

			errToCode(payments.ErrNoTiers, pb.RpcMembershipIsNameValidResponseError_TIER_NOT_FOUND),
			errToCode(payments.ErrNoTierFound, pb.RpcMembershipIsNameValidResponseError_TIER_NOT_FOUND),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipIsNameValidResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := err.Error()
		if code == pb.RpcMembershipIsNameValidResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipIsNameValidResponse{
			Error: &pb.RpcMembershipIsNameValidResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipRegisterPaymentRequest(ctx context.Context, req *pb.RpcMembershipRegisterPaymentRequestRequest) *pb.RpcMembershipRegisterPaymentRequestResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.RegisterPaymentRequest(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipRegisterPaymentRequestResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipRegisterPaymentRequestResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipRegisterPaymentRequestResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipRegisterPaymentRequestResponseError_CACHE_ERROR),

			errToCode(proto.ErrTierNotFound, pb.RpcMembershipRegisterPaymentRequestResponseError_TIER_NOT_FOUND),
			errToCode(proto.ErrTierWrong, pb.RpcMembershipRegisterPaymentRequestResponseError_TIER_INVALID),
			errToCode(proto.ErrPaymentMethodWrong, pb.RpcMembershipRegisterPaymentRequestResponseError_PAYMENT_METHOD_INVALID),
			errToCode(proto.ErrBadAnyName, pb.RpcMembershipRegisterPaymentRequestResponseError_BAD_ANYNAME),
			errToCode(proto.ErrSubsAlreadyActive, pb.RpcMembershipRegisterPaymentRequestResponseError_MEMBERSHIP_ALREADY_EXISTS),
			errToCode(proto.ErrEmailWrongFormat, pb.RpcMembershipRegisterPaymentRequestResponseError_EMAIL_WRONG_FORMAT),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipRegisterPaymentRequestResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := err.Error()
		if code == pb.RpcMembershipRegisterPaymentRequestResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipRegisterPaymentRequestResponse{
			Error: &pb.RpcMembershipRegisterPaymentRequestResponseError{
				Code:        code,
				Description: errStr,
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
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetPortalLinkUrlResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetPortalLinkUrlResponseError_CACHE_ERROR),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipGetPortalLinkUrlResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := err.Error()
		if code == pb.RpcMembershipGetPortalLinkUrlResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipGetPortalLinkUrlResponse{
			Error: &pb.RpcMembershipGetPortalLinkUrlResponseError{
				Code:        code,
				Description: errStr,
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
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetVerificationEmailResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetVerificationEmailResponseError_CACHE_ERROR),

			errToCode(proto.ErrEmailWrongFormat, pb.RpcMembershipGetVerificationEmailResponseError_EMAIL_WRONG_FORMAT),
			errToCode(proto.ErrEmailAlreadyVerified, pb.RpcMembershipGetVerificationEmailResponseError_EMAIL_ALREADY_VERIFIED),
			errToCode(proto.ErrEmailAlreadySent, pb.RpcMembershipGetVerificationEmailResponseError_EMAIL_ALREDY_SENT),
			errToCode(proto.ErrEmailFailedToSend, pb.RpcMembershipGetVerificationEmailResponseError_EMAIL_FAILED_TO_SEND),
			errToCode(proto.ErrSubsAlreadyActive, pb.RpcMembershipGetVerificationEmailResponseError_MEMBERSHIP_ALREADY_EXISTS),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipGetVerificationEmailResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := err.Error()
		if code == pb.RpcMembershipGetVerificationEmailResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipGetVerificationEmailResponse{
			Error: &pb.RpcMembershipGetVerificationEmailResponseError{
				Code:        code,
				Description: errStr,
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
			errToCode(payments.ErrNoConnection, pb.RpcMembershipVerifyEmailCodeResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipVerifyEmailCodeResponseError_CACHE_ERROR),

			errToCode(proto.ErrEmailAlreadyVerified, pb.RpcMembershipVerifyEmailCodeResponseError_EMAIL_ALREADY_VERIFIED),
			errToCode(proto.ErrEmailExpired, pb.RpcMembershipVerifyEmailCodeResponseError_CODE_EXPIRED),
			errToCode(proto.ErrEmailWrongCode, pb.RpcMembershipVerifyEmailCodeResponseError_CODE_WRONG),
			errToCode(proto.ErrSubsNotFound, pb.RpcMembershipVerifyEmailCodeResponseError_MEMBERSHIP_NOT_FOUND),
			errToCode(proto.ErrSubsAlreadyActive, pb.RpcMembershipVerifyEmailCodeResponseError_MEMBERSHIP_ALREADY_ACTIVE),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipVerifyEmailCodeResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := err.Error()
		if code == pb.RpcMembershipVerifyEmailCodeResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipVerifyEmailCodeResponse{
			Error: &pb.RpcMembershipVerifyEmailCodeResponseError{
				Code:        code,
				Description: errStr,
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
			errToCode(payments.ErrNoConnection, pb.RpcMembershipFinalizeResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipFinalizeResponseError_CACHE_ERROR),

			errToCode(proto.ErrSubsNotFound, pb.RpcMembershipFinalizeResponseError_MEMBERSHIP_NOT_FOUND),
			errToCode(proto.ErrSubsWrongState, pb.RpcMembershipFinalizeResponseError_MEMBERSHIP_WRONG_STATE),

			errToCode(proto.ErrBadAnyName, pb.RpcMembershipFinalizeResponseError_BAD_ANYNAME),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipFinalizeResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := err.Error()
		if code == pb.RpcMembershipFinalizeResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipFinalizeResponse{
			Error: &pb.RpcMembershipFinalizeResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipGetTiers(ctx context.Context, req *pb.RpcMembershipGetTiersRequest) *pb.RpcMembershipGetTiersResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.GetTiers(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipGetTiersResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipGetTiersResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetTiersResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetTiersResponseError_CACHE_ERROR),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipGetTiersResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := err.Error()
		if code == pb.RpcMembershipGetTiersResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipGetTiersResponse{
			Error: &pb.RpcMembershipGetTiersResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipVerifyAppStoreReceipt(ctx context.Context, req *pb.RpcMembershipVerifyAppStoreReceiptRequest) *pb.RpcMembershipVerifyAppStoreReceiptResponse {
	ps := getService[payments.Service](mw)
	out, err := ps.VerifyAppStoreReceipt(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipVerifyAppStoreReceiptResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipVerifyAppStoreReceiptResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipVerifyAppStoreReceiptResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipVerifyAppStoreReceiptResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipVerifyAppStoreReceiptResponseError_CACHE_ERROR),
			errToCode(proto.ErrAppleInvalidReceipt, pb.RpcMembershipVerifyAppStoreReceiptResponseError_INVALID_RECEIPT),
			errToCode(proto.ErrApplePurchaseRegistration, pb.RpcMembershipVerifyAppStoreReceiptResponseError_PURCHASE_REGISTRATION_ERROR),
			errToCode(proto.ErrAppleSubscriptionRenew, pb.RpcMembershipVerifyAppStoreReceiptResponseError_SUBSCRIPTION_RENEW_ERROR),
			errToCode(proto.ErrUnknown, pb.RpcMembershipVerifyAppStoreReceiptResponseError_UNKNOWN_ERROR),
		)

		return &pb.RpcMembershipVerifyAppStoreReceiptResponse{
			Error: &pb.RpcMembershipVerifyAppStoreReceiptResponseError{
				Code:        code,
				Description: err.Error(),
			},
		}
	}

	return out
}
