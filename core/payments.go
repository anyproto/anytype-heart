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

func (mw *Middleware) PaymentsSubscriptionGetPaymentURL(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPaymentURLRequest) *pb.RpcPaymentsSubscriptionGetPaymentURLResponse {
	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPaymentProcessingService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentURLResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentURLResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentURLResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	w, err := mw.getWallet()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentURLResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentURLResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentURLResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	return getPaymentURL(ctx, pp, w, req)
}

func subscriptionGetStatus(ctx context.Context, pp ppclient.AnyPpClientService, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetStatusRequest) *pb.RpcPaymentsSubscriptionGetStatusResponse {
	// 1 - create request
	gsr := psp.GetSubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyID: w.Account().PeerId,
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

	privKey := w.GetDevicePrivkey()

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

func getPaymentURL(ctx context.Context, pp ppclient.AnyPpClientService, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetPaymentURLRequest) *pb.RpcPaymentsSubscriptionGetPaymentURLResponse {
	// 1 - create request
	bsr := psp.BuySubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyID: w.Account().PeerId,

		// including 0x
		OwnerEthAddress: w.GetAccountEthAddress().Hex(),

		RequestedTier: psp.SubscriptionTier(req.RequestedTier),
		PaymentMethod: psp.PaymentMethod(req.PaymentMethod),

		RequestedAnyName: req.RequestedAnyName,
	}

	// 2 - sign it with the wallet
	payload, err := bsr.Marshal()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentURLResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentURLResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentURLResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	privKey := w.GetDevicePrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentURLResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentURLResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentURLResponseError_UNKNOWN_ERROR,
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
		return &pb.RpcPaymentsSubscriptionGetPaymentURLResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentURLResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentURLResponseError_PAYMENT_NODE_ERROR,
				Description: err.Error(),
			},
		}
	}

	// return out
	var out pb.RpcPaymentsSubscriptionGetPaymentURLResponse
	out.PaymentUrl = bsRet.PaymentUrl

	return &out
}
