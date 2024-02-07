package core

import (
	"context"

	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) getPaymentProcessingService() (pp ppclient.AnyPpClientService, err error) {
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent(ppclient.CName).(ppclient.AnyPpClientService), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) getWallet() (w wallet.Wallet, err error) {
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent(wallet.CName).(wallet.Wallet), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) PaymentsSubscriptionGetStatus(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetStatusRequest) *pb.RpcPaymentsSubscriptionGetStatusResponse {
	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPaymentProcessingService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetStatusResponse{
			Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	w, err := mw.getWallet()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetStatusResponse{
			Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	return subscriptionGetStatus(ctx, pp, w, req)
}

func (mw *Middleware) PaymentsSubscriptionGetPaymentUrl(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) *pb.RpcPaymentsSubscriptionGetPaymentUrlResponse {
	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPaymentProcessingService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	w, err := mw.getWallet()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	return getPaymentURL(ctx, pp, w, req)
}

func (mw *Middleware) PaymentsSubscriptionGetPortalLinkUrl(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) *pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse {
	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPaymentProcessingService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	w, err := mw.getWallet()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	return getPortalLink(ctx, pp, w, req)
}

func (mw *Middleware) PaymentsSubscriptionGetVerificationEmail(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetVerificationEmailRequest) *pb.RpcPaymentsSubscriptionGetVerificationEmailResponse {
	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPaymentProcessingService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetVerificationEmailResponse{
			Error: &pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	w, err := mw.getWallet()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetVerificationEmailResponse{
			Error: &pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	return getVerificationEmail(ctx, pp, w, req)
}

func (mw *Middleware) PaymentsSubscriptionVerifyEmailCode(ctx context.Context, req *pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest) *pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse {
	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see
	pp, err := mw.getPaymentProcessingService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse{
			Error: &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError{
				Code:        pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	w, err := mw.getWallet()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse{
			Error: &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError{
				Code:        pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	return verifyEmailCode(ctx, pp, w, req)
}

func subscriptionGetStatus(ctx context.Context, pp ppclient.AnyPpClientService, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetStatusRequest) *pb.RpcPaymentsSubscriptionGetStatusResponse {
	// 1 - create request
	gsr := psp.GetSubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyID: w.Account().SignKey.GetPublic().Account(),
	}

	// 2 - sign it with the wallet
	payload, err := gsr.Marshal()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetStatusResponse{
			Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	// this is the SignKey
	privKey := w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetStatusResponse{
			Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	reqSigned := psp.GetSubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	// 3 - send request subscription
	status, err := pp.GetSubscriptionStatus(ctx, &reqSigned)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetStatusResponse{
			Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_PAYMENT_NODE_ERROR,
				Description: err.Error(),
			},
		}
	}

	var out pb.RpcPaymentsSubscriptionGetStatusResponse

	out.Tier = pb.RpcPaymentsSubscriptionSubscriptionTier(status.Tier)
	out.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(status.Status)
	out.DateStarted = status.DateStarted
	out.DateEnds = status.DateEnds
	out.IsAutoRenew = status.IsAutoRenew
	out.NextTier = pb.RpcPaymentsSubscriptionSubscriptionTier(status.NextTier)
	out.NextTierEnds = status.NextTierEnds
	out.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(status.PaymentMethod)
	out.RequestedAnyName = status.RequestedAnyName

	return &out
}

func getPaymentURL(ctx context.Context, pp ppclient.AnyPpClientService, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) *pb.RpcPaymentsSubscriptionGetPaymentUrlResponse {
	// 1 - create request
	bsr := psp.BuySubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: w.Account().SignKey.GetPublic().Account(),

		// including 0x
		OwnerEthAddress: w.GetAccountEthAddress().Hex(),

		RequestedTier: psp.SubscriptionTier(req.RequestedTier),
		PaymentMethod: psp.PaymentMethod(req.PaymentMethod),

		RequestedAnyName: req.RequestedAnyName,
	}

	// 2 - sign it with the wallet
	payload, err := bsr.Marshal()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	privKey := w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	reqSigned := psp.BuySubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	bsRet, err := pp.BuySubscription(ctx, &reqSigned)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_PAYMENT_NODE_ERROR,
				Description: err.Error(),
			},
		}
	}

	// return out
	var out pb.RpcPaymentsSubscriptionGetPaymentUrlResponse
	out.PaymentUrl = bsRet.PaymentUrl

	return &out
}

func getPortalLink(ctx context.Context, pp ppclient.AnyPpClientService, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) *pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse {
	// 1 - create request
	bsr := psp.GetSubscriptionPortalLinkRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: w.Account().SignKey.GetPublic().Account(),
	}

	// 2 - sign it with the wallet
	payload, err := bsr.Marshal()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	privKey := w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	reqSigned := psp.GetSubscriptionPortalLinkRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	bsRet, err := pp.GetSubscriptionPortalLink(ctx, &reqSigned)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError_PAYMENT_NODE_ERROR,
				Description: err.Error(),
			},
		}
	}

	// return out
	var out pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse
	out.PortalUrl = bsRet.PortalUrl

	return &out
}

func getVerificationEmail(ctx context.Context, pp ppclient.AnyPpClientService, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetVerificationEmailRequest) *pb.RpcPaymentsSubscriptionGetVerificationEmailResponse {
	// 1 - create request
	bsr := psp.GetVerificationEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:            w.Account().SignKey.GetPublic().Account(),
		Email:                 req.Email,
		SubscribeToNewsletter: req.SubscribeToNewsletter,
	}

	// 2 - sign it with the wallet
	payload, err := bsr.Marshal()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetVerificationEmailResponse{
			Error: &pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	privKey := w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetVerificationEmailResponse{
			Error: &pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	reqSigned := psp.GetVerificationEmailRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	// empty return or error
	_, err = pp.GetVerificationEmail(ctx, &reqSigned)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetVerificationEmailResponse{
			Error: &pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetVerificationEmailResponseError_PAYMENT_NODE_ERROR,
				Description: err.Error(),
			},
		}
	}

	// return out
	var out pb.RpcPaymentsSubscriptionGetVerificationEmailResponse
	return &out
}

func verifyEmailCode(ctx context.Context, pp ppclient.AnyPpClientService, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest) *pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse {
	// 1 - create request
	bsr := psp.VerifyEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:      w.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress: w.GetAccountEthAddress().Hex(),
		Code:            req.Code,
	}

	// 2 - sign it with the wallet
	payload, err := bsr.Marshal()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse{
			Error: &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError{
				Code:        pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	privKey := w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse{
			Error: &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError{
				Code:        pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	reqSigned := psp.VerifyEmailRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	// empty return or error
	_, err = pp.VerifyEmail(ctx, &reqSigned)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse{
			Error: &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError{
				Code:        pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	// return out
	var out pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse
	return &out
}
