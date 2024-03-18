package payments

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"

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
	GetSubscriptionStatus(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetStatusRequest) (*pb.RpcPaymentsSubscriptionGetStatusResponse, error)

	GetPaymentURL(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) (*pb.RpcPaymentsSubscriptionGetPaymentUrlResponse, error)
	GetPortalLink(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) (*pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse, error)

	GetVerificationEmail(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetVerificationEmailRequest) (*pb.RpcPaymentsSubscriptionGetVerificationEmailResponse, error)
	VerifyEmailCode(ctx context.Context, req *pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest) (*pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse, error)

	FinalizeSubscription(ctx context.Context, req *pb.RpcPaymentsSubscriptionFinalizeRequest) (*pb.RpcPaymentsSubscriptionFinalizeResponse, error)

	GetTiers(ctx context.Context, req *pb.RpcPaymentsTiersGetRequest) (*pb.RpcPaymentsTiersGetResponse, error)

	app.Component
}

func New() Service {
	return &service{}
}

type service struct {
	cache        cache.CacheService
	ppclient     ppclient.AnyPpClientService
	wallet       wallet.Wallet
	spaceService space.Service
	account      account.Service
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.cache = app.MustComponent[cache.CacheService](a)
	s.ppclient = app.MustComponent[ppclient.AnyPpClientService](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.account = app.MustComponent[account.Service](a)

	return nil
}

func (s *service) Run(_ context.Context) (err error) {
	return nil
}

func (s *service) Close(_ context.Context) (err error) {
	return nil
}

func (s *service) GetSubscriptionStatus(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetStatusRequest) (*pb.RpcPaymentsSubscriptionGetStatusResponse, error) {
	ownerID := s.wallet.Account().SignKey.GetPublic().Account()
	privKey := s.wallet.GetAccountPrivkey()

	saveGlobalName := func(globalName string) {
		spc, err := s.spaceService.Get(context.Background(), s.account.PersonalSpaceID())
		if err != nil {
			log.Error("failed to get personal space id:" + err.Error())
			return
		}
		if err = spc.Do(s.account.MyParticipantId(s.account.PersonalSpaceID()), func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetDetailAndBundledRelation(bundle.RelationKeyGlobalName, pbtypes.String(globalName))
			return sb.Apply(st, smartblock.NoRestrictions)
		}); err != nil {
			log.Error("failed to set global name to profile object:" + err.Error())
		}
	}

	// 1 - check in cache
	cached, err := s.cache.CacheGet()
	// if NoCache -> skip returning from cache
	if err == nil && !req.NoCache {
		saveGlobalName(cached.RequestedAnyName)
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

	status, err := s.ppclient.GetSubscriptionStatus(ctx, &reqSigned)
	if err != nil {
		log.Info("creating empty subscription in cache because can not get subscription status from payment node")

		// eat error and create empty status ("no tier") so that we will then save it to the cache
		status = &psp.GetSubscriptionResponse{
			Tier:   int32(psp.SubscriptionTier_TierUnknown),
			Status: psp.SubscriptionStatus_StatusUnknown,
		}
	}

	var out pb.RpcPaymentsSubscriptionGetStatusResponse

	out.Tier = status.Tier
	out.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(status.Status)
	out.DateStarted = status.DateStarted
	out.DateEnds = status.DateEnds
	out.IsAutoRenew = status.IsAutoRenew
	out.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(status.PaymentMethod)
	out.RequestedAnyName = status.RequestedAnyName
	out.UserEmail = status.UserEmail
	out.SubscribeToNewsletter = status.SubscribeToNewsletter

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

	err = s.cache.CacheSet(&out, cacheExpireTime)
	if err != nil {
		return nil, err
	}

	// 4 - if cache was disabled but the tier is different or status is active -> enable cache again (we have received new data)
	if !s.cache.IsCacheEnabled() {
		// only when tier haschanged
		isDiffTier := (cached != nil) && (cached.Tier != status.Tier)
		isActive := (status.Status == psp.SubscriptionStatus(pb.RpcPaymentsSubscription_StatusActive))

		log.Debug("checking if payment cache should be enabled again", zap.Bool("isDiffTier", isDiffTier), zap.Bool("isActive", isActive))

		if cached == nil || (isDiffTier || isActive) {
			log.Debug("enabling cache again")

			// or it will be automatically enabled after N minutes of DisableForNextMinutes() call
			err := s.cache.CacheEnable()
			if err != nil {
				return nil, err
			}
		}
	}

	// 5 - save RequestedAnyName to details of local identity object
	saveGlobalName(status.RequestedAnyName)

	return &out, nil
}

func (s *service) GetPaymentURL(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPaymentUrlRequest) (*pb.RpcPaymentsSubscriptionGetPaymentUrlResponse, error) {
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

	var out pb.RpcPaymentsSubscriptionGetPaymentUrlResponse
	out.PaymentUrl = bsRet.PaymentUrl

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after payment URL was received")

	err = s.cache.CacheDisableForNextMinutes(30)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (s *service) GetPortalLink(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest) (*pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse, error) {
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

	var out pb.RpcPaymentsSubscriptionGetPortalLinkUrlResponse
	out.PortalUrl = bsRet.PortalUrl

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after portal link was received")
	err = s.cache.CacheDisableForNextMinutes(30)
	if err != nil {
		return nil, err
	}

	return &out, nil
}

func (s *service) GetVerificationEmail(ctx context.Context, req *pb.RpcPaymentsSubscriptionGetVerificationEmailRequest) (*pb.RpcPaymentsSubscriptionGetVerificationEmailResponse, error) {
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

	var out pb.RpcPaymentsSubscriptionGetVerificationEmailResponse
	return &out, nil
}

func (s *service) VerifyEmailCode(ctx context.Context, req *pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest) (*pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse, error) {
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
	var out pb.RpcPaymentsSubscriptionVerifyEmailCodeResponse
	return &out, nil
}

func (s *service) FinalizeSubscription(ctx context.Context, req *pb.RpcPaymentsSubscriptionFinalizeRequest) (*pb.RpcPaymentsSubscriptionFinalizeResponse, error) {
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
	var out pb.RpcPaymentsSubscriptionFinalizeResponse
	return &out, nil
}

func (s *service) GetTiers(ctx context.Context, req *pb.RpcPaymentsTiersGetRequest) (*pb.RpcPaymentsTiersGetResponse, error) {
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
	var out pb.RpcPaymentsTiersGetResponse

	out.Tiers = make([]*model.SubscriptionTierData, len(tiers.Tiers))
	for i, tier := range tiers.Tiers {
		out.Tiers[i] = &model.SubscriptionTierData{
			Id:                    tier.Id,
			Name:                  tier.Name,
			Description:           tier.Description,
			IsActive:              tier.IsActive,
			IsTest:                tier.IsTest,
			IsHiddenTier:          tier.IsHiddenTier,
			PeriodType:            model.SubscriptionTierDataPeriodType(tier.PeriodType),
			PeriodValue:           tier.PeriodValue,
			PriceStripeUsdCents:   tier.PriceStripeUsdCents,
			AnyNamesCountIncluded: tier.AnyNamesCountIncluded,
			AnyNameMinLength:      tier.AnyNameMinLength,
		}

		// copy all features
		out.Tiers[i].Features = make(map[string]*model.SubscriptionTierDataFeature)
		for k, v := range tier.Features {
			out.Tiers[i].Features[k] = &model.SubscriptionTierDataFeature{
				ValueStr:  v.ValueStr,
				ValueUint: v.ValueUint,
			}
		}
	}

	return &out, nil
}
