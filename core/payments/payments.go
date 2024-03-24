package payments

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/periodicsync"
	"go.uber.org/zap"

	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "payments"

var log = logging.Logger(CName).Desugar()

const (
	refreshIntervalSecs = 10
	timeout             = 10 * time.Second
	initialStatus       = -1
)

type identitiesUpdater interface {
	UpdateIdentities()
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
    x if cache was disabled before and tier has changed or status is active -> enable cache again
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
	GetSubscriptionStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) (*pb.RpcMembershipGetStatusResponse, error)
	IsNameValid(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) (*pb.RpcMembershipIsNameValidResponse, error)
	GetPaymentURL(ctx context.Context, req *pb.RpcMembershipGetPaymentUrlRequest) (*pb.RpcMembershipGetPaymentUrlResponse, error)
	GetPortalLink(ctx context.Context, req *pb.RpcMembershipGetPortalLinkUrlRequest) (*pb.RpcMembershipGetPortalLinkUrlResponse, error)
	GetVerificationEmail(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) (*pb.RpcMembershipGetVerificationEmailResponse, error)
	VerifyEmailCode(ctx context.Context, req *pb.RpcMembershipVerifyEmailCodeRequest) (*pb.RpcMembershipVerifyEmailCodeResponse, error)
	FinalizeSubscription(ctx context.Context, req *pb.RpcMembershipFinalizeRequest) (*pb.RpcMembershipFinalizeResponse, error)
	GetTiers(ctx context.Context, req *pb.RpcMembershipTiersGetRequest) (*pb.RpcMembershipTiersGetResponse, error)

	app.ComponentRunnable
}

func New() Service {
	return &service{}
}

type service struct {
	cache             cache.CacheService
	ppclient          ppclient.AnyPpClientService
	wallet            wallet.Wallet
	mx                sync.Mutex
	periodicGetStatus periodicsync.PeriodicSync
	eventSender       event.Sender
	profileUpdater    identitiesUpdater
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.cache = app.MustComponent[cache.CacheService](a)
	s.ppclient = app.MustComponent[ppclient.AnyPpClientService](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.periodicGetStatus = periodicsync.NewPeriodicSync(refreshIntervalSecs, timeout, s.getPeriodicStatus, logger.CtxLogger{Logger: log})
	s.profileUpdater = app.MustComponent[identitiesUpdater](a)
	return nil
}

func (s *service) Run(ctx context.Context) (err error) {
	// skip running loop if called from tests
	val := ctx.Value("dontRunPeriodicGetStatus")
	if val != nil && val.(bool) {
		return nil
	}

	s.periodicGetStatus.Run()
	return nil
}

func (s *service) Close(_ context.Context) (err error) {
	s.periodicGetStatus.Close()
	return nil
}

func (s *service) getPeriodicStatus(ctx context.Context) error {
	// get subscription status (from cache or from PP node)
	// if status is changed -> it will send an event
	log.Debug("periodic: getting subscription status from cache/PP node")

	_, err := s.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
	return err
}

func (s *service) sendEvent(status *pb.RpcMembershipGetStatusResponse) {
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfMembershipUpdate{
					MembershipUpdate: &pb.EventMembershipUpdate{
						Data: status.Data,
					},
				},
			},
		},
	})
}

func (s *service) GetSubscriptionStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) (*pb.RpcMembershipGetStatusResponse, error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	ownerID := s.wallet.Account().SignKey.GetPublic().Account()
	privKey := s.wallet.GetAccountPrivkey()

	// 1 - check in cache
	cached, err := s.cache.CacheGet()

	// if NoCache -> skip returning from cache
	if err == nil && !req.NoCache {
		log.Debug("returning subscription status from cache", zap.Error(err), zap.Any("cached", cached))
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

	log.Debug("get sub from PP node", zap.Any("cached", cached), zap.Bool("noCache", req.NoCache))

	status, err := s.ppclient.GetSubscriptionStatus(ctx, &reqSigned)
	if err != nil {
		log.Info("creating empty subscription in cache because can not get subscription status from payment node")

		// eat error and create empty status ("no tier") so that we will then save it to the cache
		status = &psp.GetSubscriptionResponse{
			Tier:   int32(psp.SubscriptionTier_TierUnknown),
			Status: psp.SubscriptionStatus_StatusUnknown,
		}
	}

	out := pb.RpcMembershipGetStatusResponse{
		Data: &model.Membership{},
	}

	out.Data.Tier = status.Tier
	out.Data.Status = model.MembershipStatus(status.Status)
	out.Data.DateStarted = status.DateStarted
	out.Data.DateEnds = status.DateEnds
	out.Data.IsAutoRenew = status.IsAutoRenew
	out.Data.PaymentMethod = model.MembershipPaymentMethod(status.PaymentMethod)
	out.Data.RequestedAnyName = status.RequestedAnyName
	out.Data.UserEmail = status.UserEmail
	out.Data.SubscribeToNewsletter = status.SubscribeToNewsletter

	// 3 - save into cache
	// truncate nseconds here
	var cacheExpireTime time.Time = time.Unix(int64(status.DateEnds), 0)
	isExpired := time.Now().UTC().After(cacheExpireTime)

	// if subscription DateEns is null - then default expire time is in 10 days
	// or until user clicks on a “Pay by card/crypto” or “Manage” button
	if status.DateEnds == 0 || isExpired {
		log.Debug("setting cache to +1 day because subscription is isExpired")

		timeNow := time.Now().UTC()
		cacheExpireTime = timeNow.Add(1 * 24 * time.Hour)
	}

	err = s.cache.CacheSet(&out, cacheExpireTime)
	if err != nil {
		return nil, err
	}

	isDiffTier := (cached != nil) && (cached.Data.Tier != status.Tier)
	isDiffStatus := (cached != nil) && (cached.Data.Status != model.MembershipStatus(status.Status))
	isDiffRequestedName := (cached != nil) && (cached.Data.RequestedAnyName != status.RequestedAnyName)

	log.Debug("subscription status", zap.Any("from server", status), zap.Any("cached", cached))

	// 4 - if cache was disabled but the tier is different or status is active
	if cached == nil || (isDiffTier || isDiffStatus) {
		log.Info("subscription status has changed. sending EventMembershipUpdate",
			zap.Bool("cache was empty", cached == nil),
			zap.Bool("isDiffTier", isDiffTier),
			zap.Bool("isDiffStatus", isDiffStatus),
		)

		// 4.1 - send the event
		s.sendEvent(&out)

		// 4.2 - enable cache again (we have received new data)
		log.Info("enabling cache again")

		// or it will be automatically enabled after N minutes of DisableForNextMinutes() call
		err := s.cache.CacheEnable()
		if err != nil {
			return nil, err
		}
	}

	// 5 - if requested any name has changed, then we need to update details of local identity
	if isDiffRequestedName {
		s.profileUpdater.UpdateIdentities()
	}

	return &out, nil
}

func (s *service) IsNameValid(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) (*pb.RpcMembershipIsNameValidResponse, error) {
	// 1 - send request
	invr := psp.IsNameValidRequest{
		// payment node will check if signature matches with this OwnerAnyID
		RequestedTier:    req.RequestedTier,
		RequestedAnyName: req.RequestedAnyName,
	}

	resp, err := s.ppclient.IsNameValid(ctx, &invr)
	if err != nil {
		return nil, err
	}

	var out pb.RpcMembershipIsNameValidResponse
	out.Code = model.MembershipTierDataNameValidity(resp.Code)
	out.Description = resp.Description
	return &out, nil
}

func (s *service) GetPaymentURL(ctx context.Context, req *pb.RpcMembershipGetPaymentUrlRequest) (*pb.RpcMembershipGetPaymentUrlResponse, error) {
	// 1 - send request
	bsr := psp.BuySubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),

		// not SCW address, but EOA address
		// including 0x
		OwnerEthAddress: s.wallet.GetAccountEthAddress().Hex(),

		RequestedTier: req.RequestedTier,
		PaymentMethod: psp.PaymentMethod(req.PaymentMethod),

		RequestedAnyName: req.RequestedAnyName,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.BuySubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	bsRet, err := s.ppclient.BuySubscription(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	var out pb.RpcMembershipGetPaymentUrlResponse
	out.PaymentUrl = bsRet.PaymentUrl

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after payment URL was received")

	err = s.cache.CacheDisableForNextMinutes(30)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (s *service) GetPortalLink(ctx context.Context, req *pb.RpcMembershipGetPortalLinkUrlRequest) (*pb.RpcMembershipGetPortalLinkUrlResponse, error) {
	// 1 - send request
	bsr := psp.GetSubscriptionPortalLinkRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.GetSubscriptionPortalLinkRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	bsRet, err := s.ppclient.GetSubscriptionPortalLink(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	var out pb.RpcMembershipGetPortalLinkUrlResponse
	out.PortalUrl = bsRet.PortalUrl

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after portal link was received")
	err = s.cache.CacheDisableForNextMinutes(30)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (s *service) GetVerificationEmail(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) (*pb.RpcMembershipGetVerificationEmailResponse, error) {
	// 1 - send request
	bsr := psp.GetVerificationEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:            s.wallet.Account().SignKey.GetPublic().Account(),
		Email:                 req.Email,
		SubscribeToNewsletter: req.SubscribeToNewsletter,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.GetVerificationEmailRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	_, err = s.ppclient.GetVerificationEmail(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	var out pb.RpcMembershipGetVerificationEmailResponse
	return &out, nil
}

func (s *service) VerifyEmailCode(ctx context.Context, req *pb.RpcMembershipVerifyEmailCodeRequest) (*pb.RpcMembershipVerifyEmailCodeResponse, error) {
	// 1 - send request
	bsr := psp.VerifyEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:      s.wallet.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress: s.wallet.GetAccountEthAddress().Hex(),
		Code:            req.Code,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.VerifyEmailRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	// empty return or error
	_, err = s.ppclient.VerifyEmail(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	// 2 - clear cache
	log.Debug("clearing cache after email verification code was confirmed")
	err = s.cache.CacheClear()
	if err != nil {
		return nil, err
	}

	// return out
	var out pb.RpcMembershipVerifyEmailCodeResponse
	return &out, nil
}

func (s *service) FinalizeSubscription(ctx context.Context, req *pb.RpcMembershipFinalizeRequest) (*pb.RpcMembershipFinalizeResponse, error) {
	// 1 - send request
	bsr := psp.FinalizeSubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:       s.wallet.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress:  s.wallet.GetAccountEthAddress().Hex(),
		RequestedAnyName: req.RequestedAnyName,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.FinalizeSubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	// empty return or error
	_, err = s.ppclient.FinalizeSubscription(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	// 2 - clear cache
	log.Debug("clearing cache after subscription was finalized")
	err = s.cache.CacheClear()
	if err != nil {
		return nil, err
	}

	// return out
	var out pb.RpcMembershipFinalizeResponse
	return &out, nil
}

func (s *service) GetTiers(ctx context.Context, req *pb.RpcMembershipTiersGetRequest) (*pb.RpcMembershipTiersGetResponse, error) {
	// 1 - send request
	bsr := psp.GetTiersRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:    s.wallet.Account().SignKey.GetPublic().Account(),
		Locale:        req.Locale,
		PaymentMethod: req.PaymentMethod,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		return nil, err
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		return nil, err
	}

	reqSigned := psp.GetTiersRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	// empty return or error
	tiers, err := s.ppclient.GetAllTiers(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	// return out
	var out pb.RpcMembershipTiersGetResponse

	out.Tiers = make([]*model.MembershipTierData, len(tiers.Tiers))
	for i, tier := range tiers.Tiers {
		out.Tiers[i] = &model.MembershipTierData{
			Id:                    tier.Id,
			Name:                  tier.Name,
			Description:           tier.Description,
			IsActive:              tier.IsActive,
			IsTest:                tier.IsTest,
			IsHiddenTier:          tier.IsHiddenTier,
			PeriodType:            model.MembershipTierDataPeriodType(tier.PeriodType),
			PeriodValue:           tier.PeriodValue,
			PriceStripeUsdCents:   tier.PriceStripeUsdCents,
			AnyNamesCountIncluded: tier.AnyNamesCountIncluded,
			AnyNameMinLength:      tier.AnyNameMinLength,
		}

		// copy all features
		out.Tiers[i].Features = make(map[string]*model.MembershipTierDataFeature)
		for k, v := range tier.Features {
			out.Tiers[i].Features[k] = &model.MembershipTierDataFeature{
				ValueStr:  v.ValueStr,
				ValueUint: v.ValueUint,
			}
		}
	}

	return &out, nil
}
