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

	ps := mustService[payments.Service](mw)
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
		errStr := getErrorDescription(err)
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
	ps := mustService[payments.Service](mw)
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
		errStr := getErrorDescription(err)
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
	ps := mustService[payments.Service](mw)
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
		errStr := getErrorDescription(err)
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
	ps := mustService[payments.Service](mw)
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
		errStr := getErrorDescription(err)
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
	ps := mustService[payments.Service](mw)
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
		errStr := getErrorDescription(err)
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
	ps := mustService[payments.Service](mw)
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
		errStr := getErrorDescription(err)
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
	ps := mustService[payments.Service](mw)
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
		errStr := getErrorDescription(err)
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

func (mw *Middleware) MembershipGetTiers(cctx context.Context, req *pb.RpcMembershipGetTiersRequest) *pb.RpcMembershipGetTiersResponse {
	onError := func(err error) *pb.RpcMembershipGetTiersResponse {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipGetTiersResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipGetTiersResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipGetTiersResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipGetTiersResponseError_CACHE_ERROR),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipGetTiersResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := getErrorDescription(err)
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
	ps, err := getService[payments.Service](mw)
	if err != nil {
		return onError(err)
	}
	out, err := ps.GetTiers(cctx, req)

	if err != nil {
		return onError(err)
	}

	return out
}

func (mw *Middleware) MembershipVerifyAppStoreReceipt(ctx context.Context, req *pb.RpcMembershipVerifyAppStoreReceiptRequest) *pb.RpcMembershipVerifyAppStoreReceiptResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.VerifyAppStoreReceipt(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipVerifyAppStoreReceiptResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipVerifyAppStoreReceiptResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipVerifyAppStoreReceiptResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipVerifyAppStoreReceiptResponseError_PAYMENT_NODE_ERROR),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipVerifyAppStoreReceiptResponseError_CACHE_ERROR),
			errToCode(proto.ErrUnknown, pb.RpcMembershipVerifyAppStoreReceiptResponseError_UNKNOWN_ERROR),
		)

		return &pb.RpcMembershipVerifyAppStoreReceiptResponse{
			Error: &pb.RpcMembershipVerifyAppStoreReceiptResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipCodeGetInfo(ctx context.Context, req *pb.RpcMembershipCodeGetInfoRequest) *pb.RpcMembershipCodeGetInfoResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.CodeGetInfo(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipCodeGetInfoResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipCodeGetInfoResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipCodeGetInfoResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipCodeGetInfoResponseError_PAYMENT_NODE_ERROR),
			// special errors for this method:
			errToCode(proto.ErrCodeNotFound, pb.RpcMembershipCodeGetInfoResponseError_CODE_NOT_FOUND),
			errToCode(proto.ErrCodeAlreadyUsed, pb.RpcMembershipCodeGetInfoResponseError_CODE_ALREADY_USED),
		)

		return &pb.RpcMembershipCodeGetInfoResponse{
			Error: &pb.RpcMembershipCodeGetInfoResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipCodeRedeem(ctx context.Context, req *pb.RpcMembershipCodeRedeemRequest) *pb.RpcMembershipCodeRedeemResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.CodeRedeem(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipCodeRedeemResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipCodeRedeemResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipCodeRedeemResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipCodeRedeemResponseError_PAYMENT_NODE_ERROR),
			// special errors for this method:
			errToCode(proto.ErrCodeNotFound, pb.RpcMembershipCodeRedeemResponseError_CODE_NOT_FOUND),
			errToCode(proto.ErrCodeAlreadyUsed, pb.RpcMembershipCodeRedeemResponseError_CODE_ALREADY_USED),
		)

		return &pb.RpcMembershipCodeRedeemResponse{
			Error: &pb.RpcMembershipCodeRedeemResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipV2GetPortalLink(ctx context.Context, req *pb.RpcMembershipV2GetPortalLinkRequest) *pb.RpcMembershipV2GetPortalLinkResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.V2GetPortalLink(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipV2GetPortalLinkResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipV2GetPortalLinkResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipV2GetPortalLinkResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipV2GetPortalLinkResponseError_PAYMENT_NODE_ERROR),
		)

		return &pb.RpcMembershipV2GetPortalLinkResponse{
			Error: &pb.RpcMembershipV2GetPortalLinkResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipV2GetProducts(ctx context.Context, req *pb.RpcMembershipV2GetProductsRequest) *pb.RpcMembershipV2GetProductsResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.V2GetProducts(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipV2GetProductsResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipV2GetProductsResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipV2GetProductsResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipV2GetProductsResponseError_PAYMENT_NODE_ERROR),
		)

		return &pb.RpcMembershipV2GetProductsResponse{
			Error: &pb.RpcMembershipV2GetProductsResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipV2GetStatus(ctx context.Context, req *pb.RpcMembershipV2GetStatusRequest) *pb.RpcMembershipV2GetStatusResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.V2GetStatus(ctx, req)

	code := mapErrorCode(err,
		errToCode(proto.ErrInvalidSignature, pb.RpcMembershipV2GetStatusResponseError_NOT_LOGGED_IN),
		errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipV2GetStatusResponseError_NOT_LOGGED_IN),
		errToCode(payments.ErrNoConnection, pb.RpcMembershipV2GetStatusResponseError_PAYMENT_NODE_ERROR),
		errToCode(net.ErrUnableToConnect, pb.RpcMembershipV2GetStatusResponseError_PAYMENT_NODE_ERROR),
	)

	if err != nil {
		return &pb.RpcMembershipV2GetStatusResponse{
			Error: &pb.RpcMembershipV2GetStatusResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipV2AnyNameIsValid(ctx context.Context, req *pb.RpcMembershipV2AnyNameIsValidRequest) *pb.RpcMembershipV2AnyNameIsValidResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.V2AnyNameIsValid(ctx, req)

	// 1 - check the validity first (remote call #1)
	// out will already contain validation Error
	// but if something bad has happened we need to process other errors here too:
	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipV2AnyNameIsValidResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipV2AnyNameIsValidResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipV2AnyNameIsValidResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipV2AnyNameIsValidResponseError_CACHE_ERROR),
			errToCode(payments.ErrNameIsAlreadyReserved, pb.RpcMembershipV2AnyNameIsValidResponseError_NAME_IS_RESERVED),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipV2AnyNameIsValidResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := getErrorDescription(err)
		if code == pb.RpcMembershipV2AnyNameIsValidResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipV2AnyNameIsValidResponse{
			Error: &pb.RpcMembershipV2AnyNameIsValidResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipV2AnyNameAllocate(ctx context.Context, req *pb.RpcMembershipV2AnyNameAllocateRequest) *pb.RpcMembershipV2AnyNameAllocateResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.V2AnyNameAllocate(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipV2AnyNameAllocateResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipV2AnyNameAllocateResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipV2AnyNameAllocateResponseError_CAN_NOT_CONNECT),
			errToCode(payments.ErrCacheProblem, pb.RpcMembershipV2AnyNameAllocateResponseError_CACHE_ERROR),

			errToCode(net.ErrUnableToConnect, pb.RpcMembershipV2AnyNameAllocateResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := getErrorDescription(err)
		if code == pb.RpcMembershipV2AnyNameAllocateResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcMembershipV2AnyNameAllocateResponse{
			Error: &pb.RpcMembershipV2AnyNameAllocateResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipV2CartGet(ctx context.Context, req *pb.RpcMembershipV2CartGetRequest) *pb.RpcMembershipV2CartGetResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.V2CartGet(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipV2CartGetResponseError_UNKNOWN_ERROR),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipV2CartGetResponseError_BAD_INPUT),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipV2CartGetResponseError_CAN_NOT_CONNECT),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipV2CartGetResponseError_CAN_NOT_CONNECT),
		)

		return &pb.RpcMembershipV2CartGetResponse{
			Error: &pb.RpcMembershipV2CartGetResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipV2CartUpdate(ctx context.Context, req *pb.RpcMembershipV2CartUpdateRequest) *pb.RpcMembershipV2CartUpdateResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.V2CartUpdate(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipV2CartUpdateResponseError_UNKNOWN_ERROR),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipV2CartUpdateResponseError_BAD_INPUT),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipV2CartUpdateResponseError_CAN_NOT_CONNECT),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipV2CartUpdateResponseError_CAN_NOT_CONNECT),
		)

		return &pb.RpcMembershipV2CartUpdateResponse{
			Error: &pb.RpcMembershipV2CartUpdateResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}

func (mw *Middleware) MembershipSelectVersion(ctx context.Context, req *pb.RpcMembershipSelectVersionRequest) *pb.RpcMembershipSelectVersionResponse {
	ps := mustService[payments.Service](mw)
	out, err := ps.SelectVersion(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(proto.ErrInvalidSignature, pb.RpcMembershipSelectVersionResponseError_NOT_LOGGED_IN),
			errToCode(proto.ErrEthAddressEmpty, pb.RpcMembershipSelectVersionResponseError_NOT_LOGGED_IN),
			errToCode(payments.ErrNoConnection, pb.RpcMembershipSelectVersionResponseError_PAYMENT_NODE_ERROR),
			errToCode(net.ErrUnableToConnect, pb.RpcMembershipSelectVersionResponseError_PAYMENT_NODE_ERROR),
		)

		return &pb.RpcMembershipSelectVersionResponse{
			Error: &pb.RpcMembershipSelectVersionResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return out
}
