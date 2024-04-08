package payments

import (
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/periodicsync"
	"go.uber.org/zap"

	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	proto "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/nameservice"
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

var (
	ErrCanNotSign   = errors.New("can not sign")
	ErrCacheProblem = errors.New("cache problem")
	ErrNoConnection = errors.New("can not connect to payment node")
	ErrNoTiers      = errors.New("can not get tiers")
	ErrNoTierFound  = errors.New("can not find requested tier")
)

type globalNamesUpdater interface {
	UpdateGlobalNames(myIdentityGlobalName string)
}

var paymentMethodMap = map[proto.PaymentMethod]model.MembershipPaymentMethod{
	proto.PaymentMethod_MethodCard:        model.Membership_MethodCard,
	proto.PaymentMethod_MethodCrypto:      model.Membership_MethodCrypto,
	proto.PaymentMethod_MethodAppleInapp:  model.Membership_MethodInappApple,
	proto.PaymentMethod_MethodGoogleInapp: model.Membership_MethodInappGoogle,
}

func PaymentMethodToModel(method proto.PaymentMethod) model.MembershipPaymentMethod {
	if val, ok := paymentMethodMap[method]; ok {
		return val
	}
	return model.Membership_MethodNone
}

func PaymentMethodToProto(method model.MembershipPaymentMethod) proto.PaymentMethod {
	for k, v := range paymentMethodMap {
		if v == method {
			return k
		}
	}

	// default
	return proto.PaymentMethod_MethodCard
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

    x if got no info -> cache it for N days
    x if got into without expiration -> cache it for N days
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
	profileUpdater    globalNamesUpdater
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
	s.profileUpdater = app.MustComponent[globalNamesUpdater](a)
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
	// tiers var. is unused here
	cachedStatus, _, err := s.cache.CacheGet()

	// if NoCache -> skip returning from cache
	if !req.NoCache && (err == nil) && (cachedStatus != nil) && (cachedStatus.Data != nil) {
		log.Debug("returning subscription status from cache", zap.Error(err), zap.Any("cachedStatus", cachedStatus))
		return cachedStatus, nil
	}

	// 2 - send request to PP node
	gsr := proto.GetSubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyID: ownerID,
	}
	payload, err := gsr.Marshal()
	if err != nil {
		log.Error("can not marshal GetSubscriptionRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	// this is the SignKey
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign GetSubscriptionRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.GetSubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	log.Debug("get sub from PP node", zap.Any("cachedStatus", cachedStatus), zap.Bool("noCache", req.NoCache))

	status, err := s.ppclient.GetSubscriptionStatus(ctx, &reqSigned)
	if err != nil {
		log.Info("creating empty subscription in cache because can not get subscription status from payment node")

		// eat error and create empty status ("no tier") so that we will then save it to the cache
		status = &proto.GetSubscriptionResponse{
			Tier:   uint32(proto.SubscriptionTier_TierUnknown),
			Status: proto.SubscriptionStatus_StatusUnknown,
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
	out.Data.PaymentMethod = PaymentMethodToModel(status.PaymentMethod)
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

	// update only status, not tiers
	err = s.cache.CacheSet(&out, nil, cacheExpireTime)
	if err != nil {
		log.Error("can not save subscription status to cache", zap.Error(err))
		return nil, ErrCacheProblem
	}

	isDiffTier := (cachedStatus != nil) && (cachedStatus.Data != nil) && (cachedStatus.Data.Tier != status.Tier)
	isDiffStatus := (cachedStatus != nil) && (cachedStatus.Data != nil) && (cachedStatus.Data.Status != model.MembershipStatus(status.Status))
	isDiffRequestedName := (cachedStatus != nil) && (cachedStatus.Data != nil) && (cachedStatus.Data.RequestedAnyName != status.RequestedAnyName)

	log.Debug("subscription status", zap.Any("from server", status), zap.Any("cached", cachedStatus))

	// 4 - if cache was disabled but the tier is different or status is active
	if cachedStatus == nil || (isDiffTier || isDiffStatus) {
		log.Info("subscription status has changed. sending EventMembershipUpdate",
			zap.Bool("cache was empty", cachedStatus == nil),
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
			log.Error("can not enable cache", zap.Error(err))
			return nil, ErrCacheProblem
		}
	}

	// 5 - if requested any name has changed, then we need to update details of local identity
	if isDiffRequestedName {
		s.profileUpdater.UpdateGlobalNames(status.RequestedAnyName)
	}

	return &out, nil
}

func (s *service) IsNameValid(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) (*pb.RpcMembershipIsNameValidResponse, error) {
	var code proto.IsNameValidResponse_Code
	var desc string

	out := pb.RpcMembershipIsNameValidResponse{}

	/*
		// 1 - send request to PP node and ask her please
		invr := proto.IsNameValidRequest{
			// payment node will check if signature matches with this OwnerAnyID
			RequestedTier:    req.RequestedTier,
			RequestedAnyName: req.RequestedAnyName,
		}

		resp, err := s.ppclient.IsNameValid(ctx, &invr)
		if err != nil {
			return nil, err
		}

		if resp.Code == proto.IsNameValidResponse_Valid {
			// no error
			return &out, nil
		}

		out.Error = &pb.RpcMembershipIsNameValidResponseError{}
		code = resp.Code
		desc = resp.Description
	*/

	// 1 - get all tiers from cache or PP node
	// use getAllTiers instead of GetTiers because we don't care about extra logics with Explorer here
	// and first is much simpler/faster
	tiers, err := s.getAllTiers(ctx, &pb.RpcMembershipTiersGetRequest{
		NoCache: false,
		// TODO: warning! no locale and payment method are passed here!
		// Locale:        "",
		// PaymentMethod: pb.RpcMembershipPaymentMethod_PAYMENT_METHOD_UNKNOWN,
	})
	if err != nil {
		return nil, err
	}
	if tiers.Tiers == nil {
		return nil, ErrNoTiers
	}

	// find req.RequestedTier
	var tier *model.MembershipTierData
	for _, t := range tiers.Tiers {
		if t.Id == req.RequestedTier {
			tier = t
			break
		}
	}
	if tier == nil {
		return nil, ErrNoTierFound
	}

	code = s.validateAnyName(*tier, nameservice.NsNameToFullName(req.NsName, req.NsNameType))

	if code == proto.IsNameValidResponse_Valid {
		// valid
		return &out, nil
	}

	// 2 - convert code to error
	out.Error = &pb.RpcMembershipIsNameValidResponseError{}

	switch code {
	case proto.IsNameValidResponse_NoDotAny:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_BAD_INPUT
		out.Error.Description = "No .any at the end of the name"
	case proto.IsNameValidResponse_TooShort:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_TOO_SHORT
	case proto.IsNameValidResponse_TooLong:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_TOO_LONG
	case proto.IsNameValidResponse_HasInvalidChars:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_HAS_INVALID_CHARS
	case proto.IsNameValidResponse_TierFeatureNoName:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_TIER_FEATURES_NO_NAME
	default:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_UNKNOWN_ERROR
	}

	out.Error.Description = desc
	return &out, nil
}

func (s *service) validateAnyName(tier model.MembershipTierData, name string) proto.IsNameValidResponse_Code {
	if name == "" {
		// empty name means we don't want to register name, and this is valid
		return proto.IsNameValidResponse_Valid
	}

	// if name has no .any postfix -> error
	if len(name) < 4 || name[len(name)-4:] != ".any" {
		return proto.IsNameValidResponse_NoDotAny
	}

	// for extra safety normalize name here too!
	name, err := normalizeAnyName(name)
	if err != nil {
		log.Debug("can not normalize name", zap.Error(err), zap.String("name", name))
		return proto.IsNameValidResponse_HasInvalidChars
	}

	// remove .any postfix
	name = name[:len(name)-4]

	// if minLen is zero - means "no check is required"
	if tier.AnyNamesCountIncluded == 0 {
		return proto.IsNameValidResponse_TierFeatureNoName
	}
	if tier.AnyNameMinLength == 0 {
		return proto.IsNameValidResponse_TierFeatureNoName
	}
	if uint32(utf8.RuneCountInString(name)) < tier.AnyNameMinLength {
		return proto.IsNameValidResponse_TooShort
	}

	// valid
	return proto.IsNameValidResponse_Valid
}

func (s *service) GetPaymentURL(ctx context.Context, req *pb.RpcMembershipGetPaymentUrlRequest) (*pb.RpcMembershipGetPaymentUrlResponse, error) {
	// 1 - send request
	bsr := proto.BuySubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),

		// not SCW address, but EOA address
		// including 0x
		OwnerEthAddress: s.wallet.GetAccountEthAddress().Hex(),

		RequestedTier: req.RequestedTier,
		PaymentMethod: PaymentMethodToProto(req.PaymentMethod),

		RequestedAnyName: nameservice.NsNameToFullName(req.NsName, req.NsNameType),
	}

	payload, err := bsr.Marshal()
	if err != nil {
		log.Error("can not marshal BuySubscriptionRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign BuySubscriptionRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.BuySubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	bsRet, err := s.ppclient.BuySubscription(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	out := pb.RpcMembershipGetPaymentUrlResponse{
		PaymentUrl: bsRet.PaymentUrl,
		BillingId:  bsRet.BillingID,
	}

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after payment URL was received")

	err = s.cache.CacheDisableForNextMinutes(30)
	if err != nil {
		log.Error("can not disable cache", zap.Error(err))
		return nil, ErrCacheProblem
	}

	return &out, nil
}

func (s *service) GetPortalLink(ctx context.Context, req *pb.RpcMembershipGetPortalLinkUrlRequest) (*pb.RpcMembershipGetPortalLinkUrlResponse, error) {
	// 1 - send request
	bsr := proto.GetSubscriptionPortalLinkRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),
	}

	payload, err := bsr.Marshal()
	if err != nil {
		log.Error("can not marshal GetSubscriptionPortalLinkRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign GetSubscriptionPortalLinkRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.GetSubscriptionPortalLinkRequestSigned{
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
		log.Error("can not disable cache", zap.Error(err))
		return nil, ErrCacheProblem
	}

	return &out, nil
}

func (s *service) GetVerificationEmail(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) (*pb.RpcMembershipGetVerificationEmailResponse, error) {
	// 1 - send request
	bsr := proto.GetVerificationEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:            s.wallet.Account().SignKey.GetPublic().Account(),
		Email:                 req.Email,
		SubscribeToNewsletter: req.SubscribeToNewsletter,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		log.Error("can not marshal GetVerificationEmailRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign GetVerificationEmailRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.GetVerificationEmailRequestSigned{
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
	bsr := proto.VerifyEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:      s.wallet.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress: s.wallet.GetAccountEthAddress().Hex(),
		Code:            req.Code,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		log.Error("can not marshal VerifyEmailRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign VerifyEmailRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.VerifyEmailRequestSigned{
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
		log.Error("can not clear cache", zap.Error(err))
		return nil, ErrCacheProblem
	}

	// return out
	var out pb.RpcMembershipVerifyEmailCodeResponse
	return &out, nil
}

func (s *service) FinalizeSubscription(ctx context.Context, req *pb.RpcMembershipFinalizeRequest) (*pb.RpcMembershipFinalizeResponse, error) {
	// 1 - send request
	bsr := proto.FinalizeSubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:       s.wallet.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress:  s.wallet.GetAccountEthAddress().Hex(),
		RequestedAnyName: nameservice.NsNameToFullName(req.NsName, req.NsNameType),
	}

	payload, err := bsr.Marshal()
	if err != nil {
		log.Error("can not marshal FinalizeSubscriptionRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign FinalizeSubscriptionRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.FinalizeSubscriptionRequestSigned{
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
		log.Error("can not clear cache", zap.Error(err))
		return nil, ErrCacheProblem
	}

	// return out
	var out pb.RpcMembershipFinalizeResponse
	return &out, nil
}

func (s *service) GetTiers(ctx context.Context, req *pb.RpcMembershipTiersGetRequest) (*pb.RpcMembershipTiersGetResponse, error) {
	// 1 - get all tiers (including Explorer)
	out, err := s.getAllTiers(ctx, req)
	if err != nil {
		return nil, err
	}

	// 2 - remove explorer
	status, err := s.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
	if err != nil {
		log.Error("can not get subscription status", zap.Error(err))
		return nil, err
	}
	// if your are on 0-tier OR on Explorer -> return full list
	if status.Data.Tier <= uint32(proto.SubscriptionTier_TierExplorer) {
		return out, nil
	}

	// If the current tier is higher than Explorer, show the list without Explorer (downgrading is not allowed)
	filtered := &pb.RpcMembershipTiersGetResponse{
		Tiers: make([]*model.MembershipTierData, 0),
	}
	for _, tier := range out.Tiers {
		if tier.Id != uint32(proto.SubscriptionTier_TierExplorer) {
			filtered.Tiers = append(filtered.Tiers, tier)
		}
	}
	return filtered, nil
}

func (s *service) getAllTiers(ctx context.Context, req *pb.RpcMembershipTiersGetRequest) (*pb.RpcMembershipTiersGetResponse, error) {
	// 1 - check in cache
	// status var. is unused here
	cachedStatus, cachedTiers, err := s.cache.CacheGet()

	// if NoCache -> skip returning from cache
	if !req.NoCache && (err == nil) && (cachedTiers != nil) && (cachedTiers.Tiers != nil) {
		log.Debug("returning tiers from cache", zap.Error(err), zap.Any("cachedTiers", cachedTiers))
		return cachedTiers, nil
	}

	// 2 - send request
	bsr := proto.GetTiersRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),

		// WARNING: we will save to cache data for THIS locale and payment method!!!
		Locale: req.Locale,
	}

	payload, err := bsr.Marshal()
	if err != nil {
		log.Error("can not marshal GetTiersRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign GetTiersRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.GetTiersRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	// empty return or error
	tiers, err := s.ppclient.GetAllTiers(ctx, &reqSigned)
	if err != nil {
		// if error here -> we do not create empty array
		// with GetStatus above the logic is different
		// there we create empty status and save it to cache
		log.Error("can not get tiers from payment node", zap.Error(err))
		return nil, err
	}

	// 3 - return out
	var out pb.RpcMembershipTiersGetResponse

	out.Tiers = make([]*model.MembershipTierData, len(tiers.Tiers))
	for i, tier := range tiers.Tiers {
		out.Tiers[i] = &model.MembershipTierData{
			Id:          tier.Id,
			Name:        tier.Name,
			Description: tier.Description,
			// IsActive:              tier.IsActive,
			IsTest: tier.IsTest,
			// IsHiddenTier:          tier.IsHiddenTier,
			PeriodType:          model.MembershipTierDataPeriodType(tier.PeriodType),
			PeriodValue:         tier.PeriodValue,
			PriceStripeUsdCents: tier.PriceStripeUsdCents,
			// also in feature list
			AnyNamesCountIncluded: tier.AnyNamesCountIncluded,
			AnyNameMinLength:      tier.AnyNameMinLength,
			ColorStr:              tier.ColorStr,
		}

		// copy all features
		out.Tiers[i].Features = make([]string, len(tier.Features))

		for j, feature := range tier.Features {
			out.Tiers[i].Features[j] = feature.Description
		}
	}

	// 3 - update tiers, not status
	var cacheExpireTime time.Time
	if cachedStatus != nil {
		cacheExpireTime = time.Unix(int64(cachedStatus.Data.DateEnds), 0)
	} else {
		log.Debug("setting tiers cache to +1 day")

		timeNow := time.Now().UTC()
		cacheExpireTime = timeNow.Add(1 * 24 * time.Hour)
	}

	err = s.cache.CacheSet(nil, &out, cacheExpireTime)
	if err != nil {
		log.Error("can not save tiers to cache", zap.Error(err))
		return nil, ErrCacheProblem
	}

	return &out, nil
}
