package core

import (
	"context"
	"time"

	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/payments"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) getPpClient() (pp ppclient.AnyPpClientService, err error) {
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent(ppclient.CName).(ppclient.AnyPpClientService), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) getPaymentsService() (ps payments.Service, err error) {
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent(payments.CName).(payments.Service), nil
	}
	return nil, ErrNotLoggedIn
}

func (mw *Middleware) getWallet() (w wallet.Wallet, err error) {
	if a := mw.applicationService.GetApp(); a != nil {
		return a.MustComponent(wallet.CName).(wallet.Wallet), nil
	}
	return nil, ErrNotLoggedIn
}

/*
CACHE LOGICS:
 1. User installs Anytype
    -> cache is clean

2. client gets his subscription from MW

  - if cache is disabled and 30 minutes elapsed
    -> enable cache again

  - if cache is disabled or cache is clean or cache is expired
    -> ask from PP node, then save to cache:

    x if got no info -> cache it for 10 days
    x if got into without expiration -> cache it for 10 days
    x if got info -> cache it for until it expires
    x if cache was disabled before and tier has changed -> enable cache again
    x if can not connect to PP node -> return error
    x if can not write to cache -> return error

  - if we have it in cache
    -> return from cache

    3. User clicks on a “Pay by card/crypto” or “Manage” button:
    -> disable cache for 30 minutes (so we always get from PP node)

    4. User confirms his e-mail code
    -> clear cache (it will cause getting again from PP node next)
*/
func (mw *Middleware) PaymentsSubscriptionGetStatus(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetStatusRequest) *pb.RpcPaymentsSubscriptionGetStatusResponse {
	ps, err := mw.getPaymentsService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetStatusResponse{
			Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPpClient()
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

	return getStatus(ctx, pp, ps, w, req)
}

func (mw *Middleware) PaymentsSubscriptionGetPaymentUrl(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) *pb.RpcPaymentsSubscriptionGetPaymentUrlResponse {
	ps, err := mw.getPaymentsService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPpClient()
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

	return getPaymentURL(ctx, pp, ps, w, req)
}

func (mw *Middleware) PaymentsSubscriptionGetPortalLinkUrl(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) *pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse {
	ps, err := mw.getPaymentsService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPpClient()
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

	return getPortalLink(ctx, pp, ps, w, req)
}

func (mw *Middleware) PaymentsSubscriptionGetVerificationEmail(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetVerificationEmailRequest) *pb.RpcPaymentsSubscriptionGetVerificationEmailResponse {
	// Get name service object that connects to the remote "paymentProcessingNode"
	// in order for that to work, we need to have a "paymentProcessingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-sync/paymentservice/ for example
	pp, err := mw.getPpClient()
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
	pp, err := mw.getPpClient()
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

	ps, err := mw.getPaymentsService()
	if err != nil {
		return &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse{
			Error: &pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError{
				Code:        pb.RpcPaymentsSubscriptionVerifyEmailCodeResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	return verifyEmailCode(ctx, pp, ps, w, req)
}

func getStatus(ctx context.Context, pp ppclient.AnyPpClientService, ps payments.Service, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetStatusRequest) *pb.RpcPaymentsSubscriptionGetStatusResponse {
	// 1 - check in cache
	cached, err := ps.CacheGet()
	if err == nil {
		return cached
	}

	// 2 - create request to PP node
	gsr := psp.GetSubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyID: w.Account().SignKey.GetPublic().Account(),
	}

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
		log.Info("creating empty subscription in cache because can not get subscription status from payment node")

		// eat error and create empty status ("no tier") so that we will then save it to the cache
		status = &psp.GetSubscriptionResponse{
			Tier:   psp.SubscriptionTier_TierUnknown,
			Status: psp.SubscriptionStatus_StatusUnknown,
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
	out.UserEmail = status.UserEmail
	// TODO:
	//out.SubscribeToNewsletter = status.SubscribeToNewsletter

	// 4 - save into cache
	// truncate nseconds here
	var cacheExpireTime time.Time = time.Unix(int64(status.DateEnds), 0)

	// if subscription DateEns is null - then default expire time is in 10 days
	// or until user clicks on a “Pay by card/crypto” or “Manage” button
	if status.DateEnds == 0 {
		log.Debug("setting cache to 10 days because subscription DateEnds is null")

		timeNow := time.Now().UTC()
		cacheExpireTime = timeNow.Add(10 * 24 * time.Hour)
	}

	err = ps.CacheSet(&out, cacheExpireTime)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetStatusResponse{
			Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	// 5 - if cache was disabled but the tier is different -> enable cache again (we have received new data)
	if !ps.IsCacheEnabled() {
		// only when tier changed
		isDiffTier := (cached != nil) && (cached.Tier != pb.RpcPaymentsSubscriptionSubscriptionTier(status.Tier))

		// only when received active state (finally)
		isActive := (status.Status == psp.SubscriptionStatus(pb.RpcPaymentsSubscription_StatusActive))

		if cached == nil || (isDiffTier && isActive) {
			log.Debug("enabling cache again")

			// or it will be automatically enabled after N minutes of DisableForNextMinutes() call
			err := ps.CacheEnable()
			if err != nil {
				return &pb.RpcPaymentsSubscriptionGetStatusResponse{
					Error: &pb.RpcPaymentsSubscriptionGetStatusResponseError{
						Code:        pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR,
						Description: err.Error(),
					},
				}
			}
		}
	}

	return &out
}

func getPaymentURL(ctx context.Context, pp ppclient.AnyPpClientService, ps payments.Service, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) *pb.RpcPaymentsSubscriptionGetPaymentUrlResponse {
	// 1 - create request
	bsr := psp.BuySubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: w.Account().SignKey.GetPublic().Account(),

		// not SCW address, but EOA address
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

	// 3 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after payment URL was received")

	err = ps.CacheDisableForNextMinutes(30)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPaymentUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPaymentUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}
	return &out
}

func getPortalLink(ctx context.Context, pp ppclient.AnyPpClientService, ps payments.Service, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) *pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse {
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

	// 3 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after portal link was received")
	err = ps.CacheDisableForNextMinutes(30)
	if err != nil {
		return &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse{
			Error: &pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError{
				Code:        pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

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

func verifyEmailCode(ctx context.Context, pp ppclient.AnyPpClientService, ps payments.Service, w wallet.Wallet, req *pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest) *pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse {
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

	// 3 - clear cache
	log.Debug("clearing cache after email verification code was confirmed")
	err = ps.CacheClear()
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
