package payments

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"

	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
)

const CName = "payments"

var log = logger.NewNamed(CName)

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
type Service interface {
	GetSubscriptionStatus(ctx context.Context) (*pb.RpcPaymentsSubscriptionGetStatusResponse, error)

	GetPaymentURL(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) (*pb.RpcPaymentsSubscriptionGetPaymentUrlResponse, error)
	GetPortalLink(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) (*pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse, error)

	GetVerificationEmail(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetVerificationEmailRequest) (*pb.RpcPaymentsSubscriptionGetVerificationEmailResponse, error)
	VerifyEmailCode(ctx context.Context, req *pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest) (*pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse, error)

	app.Component
}

func New() Service {
	return &service{}
}

type service struct {
	c  cache.CacheService
	pp ppclient.AnyPpClientService
	w  wallet.Wallet
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.c = app.MustComponent[cache.CacheService](a)
	s.pp = app.MustComponent[ppclient.AnyPpClientService](a)
	s.w = app.MustComponent[wallet.Wallet](a)

	return nil
}

func (s *service) Run(_ context.Context) (err error) {
	return nil
}

func (s *service) Close(_ context.Context) (err error) {
	return nil
}

func (s *service) GetSubscriptionStatus(ctx context.Context) (*pb.RpcPaymentsSubscriptionGetStatusResponse, error) {
	ownerID := s.w.Account().SignKey.GetPublic().Account()
	privKey := s.w.GetAccountPrivkey()

	// 1 - check in cache
	cached, err := s.c.CacheGet()
	if err == nil {
		return cached, nil
	}

	// 2 - send request to PP node
	gsr := psp.GetSubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyID: ownerID,
	}

	payload, err := gsr.Marshal()
	if err != nil {
		return nil, err
	}

	// this is the SignKey
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.GetSubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	status, err := s.pp.GetSubscriptionStatus(ctx, &reqSigned)
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

	// 3 - save into cache
	// truncate nseconds here
	var cacheExpireTime time.Time = time.Unix(int64(status.DateEnds), 0)

	// if subscription DateEns is null - then default expire time is in 10 days
	// or until user clicks on a “Pay by card/crypto” or “Manage” button
	if status.DateEnds == 0 {
		log.Debug("setting cache to 10 days because subscription DateEnds is null")

		timeNow := time.Now().UTC()
		cacheExpireTime = timeNow.Add(10 * 24 * time.Hour)
	}

	err = s.c.CacheSet(&out, cacheExpireTime)
	if err != nil {
		return nil, err
	}

	// 4 - if cache was disabled but the tier is different -> enable cache again (we have received new data)
	if !s.c.IsCacheEnabled() {
		// only when tier changed
		isDiffTier := (cached != nil) && (cached.Tier != pb.RpcPaymentsSubscriptionSubscriptionTier(status.Tier))

		// only when received active state (finally)
		isActive := (status.Status == psp.SubscriptionStatus(pb.RpcPaymentsSubscription_StatusActive))

		if cached == nil || (isDiffTier && isActive) {
			log.Debug("enabling cache again")

			// or it will be automatically enabled after N minutes of DisableForNextMinutes() call
			err := s.c.CacheEnable()
			if err != nil {
				return nil, err
			}
		}
	}

	return &out, nil
}

func (s *service) GetPaymentURL(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) (*pb.RpcPaymentsSubscriptionGetPaymentUrlResponse, error) {
	// 1 - send request
	bsr := psp.BuySubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.w.Account().SignKey.GetPublic().Account(),

		// not SCW address, but EOA address
		// including 0x
		OwnerEthAddress: s.w.GetAccountEthAddress().Hex(),

		RequestedTier: psp.SubscriptionTier(req.RequestedTier),
		PaymentMethod: psp.PaymentMethod(req.PaymentMethod),

		RequestedAnyName: req.RequestedAnyName,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.BuySubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	bsRet, err := s.pp.BuySubscription(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	var out pb.RpcPaymentsSubscriptionGetPaymentUrlResponse
	out.PaymentUrl = bsRet.PaymentUrl

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after payment URL was received")

	err = s.c.CacheDisableForNextMinutes(30)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (s *service) GetPortalLink(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) (*pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse, error) {
	// 1 - send request
	bsr := psp.GetSubscriptionPortalLinkRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.w.Account().SignKey.GetPublic().Account(),
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.GetSubscriptionPortalLinkRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	bsRet, err := s.pp.GetSubscriptionPortalLink(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	var out pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse
	out.PortalUrl = bsRet.PortalUrl

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after portal link was received")
	err = s.c.CacheDisableForNextMinutes(30)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (s *service) GetVerificationEmail(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetVerificationEmailRequest) (*pb.RpcPaymentsSubscriptionGetVerificationEmailResponse, error) {
	// 1 - send request
	bsr := psp.GetVerificationEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:            s.w.Account().SignKey.GetPublic().Account(),
		Email:                 req.Email,
		SubscribeToNewsletter: req.SubscribeToNewsletter,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.GetVerificationEmailRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	_, err = s.pp.GetVerificationEmail(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	var out pb.RpcPaymentsSubscriptionGetVerificationEmailResponse
	return &out, nil
}

func (s *service) VerifyEmailCode(ctx context.Context, req *pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest) (*pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse, error) {
	// 1 - send request
	bsr := psp.VerifyEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:      s.w.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress: s.w.GetAccountEthAddress().Hex(),
		Code:            req.Code,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.w.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.VerifyEmailRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	// empty return or error
	_, err = s.pp.VerifyEmail(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	// 2 - clear cache
	log.Debug("clearing cache after email verification code was confirmed")
	err = s.c.CacheClear()
	if err != nil {
		return nil, err
	}

	// return out
	var out pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse
	return &out, nil
}
