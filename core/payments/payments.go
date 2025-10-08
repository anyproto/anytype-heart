package payments

//go:generate go run ./generator

import (
	"context"
	"errors"
	"os"
	"time"
	"unicode/utf8"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/quic-go/quic-go"
	"go.uber.org/zap"

	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	proto "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/nameservice"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/payments/emailcollector"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/deletioncontroller"
	"github.com/anyproto/anytype-heart/util/contexthelper"
)

const CName = "payments"

var log = logging.Logger(CName)

const (
	refreshIntervalSecs = 60
	timeout             = 30 * time.Second
	cacheDisableMinutes = 30
)

func isNoCacheEnabled() bool {
	// NoCache is deprecated and only allowed with explicit env var
	return os.Getenv("ANYTYPE_ENABLE_NOCACHE") == "1" || os.Getenv("ANYTYPE_ENABLE_NOCACHE") == "true"
}

var (
	ErrCanNotSign            = errors.New("can not sign")
	ErrCacheProblem          = errors.New("cache problem")
	ErrNoConnection          = errors.New("can not connect to payment node")
	ErrNoTiers               = errors.New("can not get tiers")
	ErrNoTierFound           = errors.New("can not find requested tier")
	ErrNameIsAlreadyReserved = errors.New("name is already reserved")
)

type globalNamesUpdater interface {
	UpdateOwnGlobalName(myIdentityGlobalName string)
}

var paymentMethodMap = map[proto.PaymentMethod]model.MembershipPaymentMethod{
	proto.PaymentMethod_MethodCard:        model.Membership_MethodStripe,
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

 3. User clicks on a "Pay by card/crypto" or "Manage" button:
    -> disable cache for 30 minutes (so we always get from PP node)

 4. User confirms his e-mail code
    -> clear cache (it will cause getting again from PP node next)
*/
type Service interface {
	GetSubscriptionStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) (*pb.RpcMembershipGetStatusResponse, error)
	IsNameValid(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) (*pb.RpcMembershipIsNameValidResponse, error)
	RegisterPaymentRequest(ctx context.Context, req *pb.RpcMembershipRegisterPaymentRequestRequest) (*pb.RpcMembershipRegisterPaymentRequestResponse, error)
	GetPortalLink(ctx context.Context, req *pb.RpcMembershipGetPortalLinkUrlRequest) (*pb.RpcMembershipGetPortalLinkUrlResponse, error)
	GetVerificationEmail(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) (*pb.RpcMembershipGetVerificationEmailResponse, error)
	VerifyEmailCode(ctx context.Context, req *pb.RpcMembershipVerifyEmailCodeRequest) (*pb.RpcMembershipVerifyEmailCodeResponse, error)
	FinalizeSubscription(ctx context.Context, req *pb.RpcMembershipFinalizeRequest) (*pb.RpcMembershipFinalizeResponse, error)
	GetTiers(ctx context.Context, req *pb.RpcMembershipGetTiersRequest) (*pb.RpcMembershipGetTiersResponse, error)
	VerifyAppStoreReceipt(ctx context.Context, req *pb.RpcMembershipVerifyAppStoreReceiptRequest) (*pb.RpcMembershipVerifyAppStoreReceiptResponse, error)
	CodeGetInfo(ctx context.Context, req *pb.RpcMembershipCodeGetInfoRequest) (*pb.RpcMembershipCodeGetInfoResponse, error)
	CodeRedeem(ctx context.Context, req *pb.RpcMembershipCodeRedeemRequest) (*pb.RpcMembershipCodeRedeemResponse, error)

	app.ComponentRunnable
}

func New() Service {
	return &service{}
}

type service struct {
	cfg                    *config.Config
	cache                  cache.CacheService
	ppclient               ppclient.AnyPpClientService
	wallet                 wallet.Wallet
	getSubscriptionLimiter chan struct{}
	periodicGetStatus      periodicsync.PeriodicSync
	eventSender            event.Sender
	profileUpdater         globalNamesUpdater
	ns                     nameservice.Service
	closing                chan struct{}

	multiplayerLimitsUpdater deletioncontroller.DeletionController
	fileLimitsUpdater        filesync.FileSync
	emailCollector           emailcollector.EmailCollector
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.cfg = app.MustComponent[*config.Config](a)
	s.cache = app.MustComponent[cache.CacheService](a)
	s.emailCollector = app.MustComponent[emailcollector.EmailCollector](a)
	s.ppclient = app.MustComponent[ppclient.AnyPpClientService](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.ns = app.MustComponent[nameservice.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.periodicGetStatus = periodicsync.NewPeriodicSync(refreshIntervalSecs, timeout, s.getPeriodicStatus, logger.CtxLogger{Logger: log.Desugar()})
	s.profileUpdater = app.MustComponent[globalNamesUpdater](a)
	s.multiplayerLimitsUpdater = app.MustComponent[deletioncontroller.DeletionController](a)
	s.fileLimitsUpdater = app.MustComponent[filesync.FileSync](a)
	s.getSubscriptionLimiter = make(chan struct{}, 1)
	s.closing = make(chan struct{})
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
	if s.closing != nil {
		close(s.closing)
	}
	if s.periodicGetStatus != nil {
		s.periodicGetStatus.Close()
	}
	return nil
}

func (s *service) getPeriodicStatus(ctx context.Context) error {
	// skip running loop if we are on a custom network or in local-only mode
	if s.cfg.GetNetworkMode() != pb.RpcAccount_DefaultConfig {
		// do not trace to log to prevent spamming
		return nil
	}

	// Background refresh: fetch subscription status from network
	err := s.refreshSubscriptionStatusBackground(ctx)
	if err != nil {
		log.Warn("periodic refresh: subscription status update failed", zap.Error(err))
		// Don't return error - continue with tiers refresh
	}

	// Background refresh: fetch tiers from network
	err = s.refreshTiersBackground(ctx)
	if err != nil {
		log.Warn("periodic refresh: tiers update failed", zap.Error(err))
		// Don't return error - this is background work
	}

	return nil
}

func (s *service) sendMembershipUpdateEvent(status *pb.RpcMembershipGetStatusResponse) {
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfMembershipUpdate{
		MembershipUpdate: &pb.EventMembershipUpdate{
			Data: status.Data,
		},
	}))
}

func (s *service) sendTiersUpdateEvent(tiers *pb.RpcMembershipGetTiersResponse) {
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfMembershipTiersUpdate{
		MembershipTiersUpdate: &pb.EventMembershipTiersUpdate{
			Tiers: tiers.Tiers,
		},
	}))
}

// GetSubscriptionStatus returns subscription status from cache ONLY
// This method NEVER makes network calls and returns immediately
// Background refresh happens via refreshSubscriptionStatusBackground()
func (s *service) GetSubscriptionStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) (*pb.RpcMembershipGetStatusResponse, error) {
	// Check if NoCache is requested but not enabled
	if req.NoCache && !isNoCacheEnabled() {
		log.Warn("NoCache flag is deprecated and ignored. Set ANYTYPE_ENABLE_NOCACHE=1 to enable")
	}

	// ALWAYS try to return from cache (ignore NoCache unless env var is set)
	cachedStatus, _, cacheErr := s.cache.CacheGet()

	// If we have cached data (even if expired/disabled), return it
	if cacheErr == nil && canReturnCachedStatus(cachedStatus) {
		log.Debug("returning subscription status from cache (RPC)", zap.Any("cachedStatus", cachedStatus))
		return cachedStatus, nil
	}

	// Cache is empty - return empty status (background sync will populate it)
	log.Debug("cache is empty, returning empty subscription status")
	emptyStatus := &pb.RpcMembershipGetStatusResponse{
		Data: &model.Membership{
			Tier:   uint32(proto.SubscriptionTier_TierUnknown),
			Status: model.MembershipStatus(proto.SubscriptionStatus_StatusUnknown),
		},
		Error: &pb.RpcMembershipGetStatusResponseError{
			Code: pb.RpcMembershipGetStatusResponseError_NULL,
		},
	}

	return emptyStatus, nil
}

func (s *service) generateRequest() (*proto.GetSubscriptionRequestSigned, error) {
	ownerID := s.wallet.Account().SignKey.GetPublic().Account()
	privKey := s.wallet.GetAccountPrivkey()

	gsr := proto.GetSubscriptionRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyID: ownerID,
	}
	payload, err := gsr.MarshalVT()
	if err != nil {
		log.Error("can not marshal GetSubscriptionRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign GetSubscriptionRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	return &proto.GetSubscriptionRequestSigned{
		Payload:   payload,
		Signature: signature,
	}, nil
}

func isCacheContainsError(s *pb.RpcMembershipGetStatusResponse) bool {
	return s != nil && s.Error != nil && s.Error.Code != pb.RpcMembershipGetStatusResponseError_NULL
}

func canReturnCachedStatus(s *pb.RpcMembershipGetStatusResponse) bool {
	return s != nil && s.Data != nil && (s.Error == nil || s.Error.Code == pb.RpcMembershipGetStatusResponseError_NULL)
}

func tiersChanged(oldTiers, newTiers *pb.RpcMembershipGetTiersResponse) bool {
	// If old cache was empty or had error, treat as changed
	if oldTiers == nil || oldTiers.Tiers == nil || (oldTiers.Error != nil && oldTiers.Error.Code != pb.RpcMembershipGetTiersResponseError_NULL) {
		return true
	}

	// If new data is nil, no change
	if newTiers == nil || newTiers.Tiers == nil {
		return false
	}

	// Check if tier count differs
	if len(oldTiers.Tiers) != len(newTiers.Tiers) {
		log.Debug("tiers changed: different count",
			zap.Int("old", len(oldTiers.Tiers)),
			zap.Int("new", len(newTiers.Tiers)))
		return true
	}

	// Check each tier for changes
	for i, newTier := range newTiers.Tiers {
		oldTier := oldTiers.Tiers[i]

		// Compare key fields that clients care about
		if oldTier.Id != newTier.Id ||
			oldTier.Name != newTier.Name ||
			oldTier.PriceStripeUsdCents != newTier.PriceStripeUsdCents ||
			oldTier.ColorStr != newTier.ColorStr ||
			len(oldTier.Features) != len(newTier.Features) {
			log.Debug("tiers changed: tier details differ", zap.Uint32("tierId", newTier.Id))
			return true
		}
	}

	return false
}

func isUpdateRequired(cacheErr error, isCacheExpired bool, cachedStatus *pb.RpcMembershipGetStatusResponse, status *proto.GetSubscriptionResponse) bool {
	// 1 - If cache was empty or expired
	// -> treat at is if data was different
	isCacheEmpty := cacheErr != nil || cachedStatus == nil || cachedStatus.Data == nil || isCacheExpired
	if isCacheEmpty {
		log.Debug("subscription status treated as changed because cache was empty/expired")
		return true
	}

	// 2 - Extra check that cache contained previous error
	if isCacheContainsError(cachedStatus) {
		log.Debug("subscription status treated as changed because cache contained previous error")
		return true
	}

	// 3 - Check if tier or status has changed
	if status == nil {
		return false
	}

	isDiffTier := cachedStatus.Data.Tier != status.Tier
	isDiffStatus := cachedStatus.Data.Status != model.MembershipStatus(status.Status)
	isEmailDiff := cachedStatus.Data.UserEmail != status.UserEmail

	if !isDiffTier && !isDiffStatus && !isEmailDiff {
		log.Debug("subscription status has NOT changed",
			zap.Bool("cache was empty", cachedStatus == nil),
			zap.Bool("isDiffTier", isDiffTier),
			zap.Bool("isDiffStatus", isDiffStatus),
		)
		return false
	}

	log.Info("subscription status has been changed. sending EventMembershipUpdate",
		zap.Bool("cache was empty", cachedStatus == nil),
		zap.Bool("isDiffTier", isDiffTier),
		zap.Bool("isDiffStatus", isDiffStatus),
		zap.Bool("isEmailDiff", isEmailDiff),
	)
	return true
}

func (s *service) updateStatus(ctx context.Context, status *proto.GetSubscriptionResponse) {
	out := convertMembershipStatus(status)

	// 1 - Broadcast event
	log.Debug("sending EventMembershipUpdate", zap.Any("status", status))
	s.sendMembershipUpdateEvent(&out)

	// 2 - If name has changed -> update global name or own identity
	if status.RequestedAnyName != "" {
		log.Debug("update global name",
			zap.String("requestedAnyName", status.RequestedAnyName),
			zap.Any("status", status))

		s.profileUpdater.UpdateOwnGlobalName(status.RequestedAnyName)
	}

	// 3 - Update limits
	err := s.updateLimits(ctx)
	var idleTimeoutErr *quic.IdleTimeoutError
	if err != nil && !errors.As(err, &idleTimeoutErr) && !errors.Is(err, context.DeadlineExceeded) {
		// eat error
		log.Error("update limits", zap.Error(err))
	}
}

func (s *service) updateLimits(ctx context.Context) error {
	s.multiplayerLimitsUpdater.UpdateCoordinatorStatus()
	return s.fileLimitsUpdater.UpdateNodeUsage(ctx)
}

func isNeedToDisableCache(status *proto.GetSubscriptionResponse) bool {
	return status.Status == proto.SubscriptionStatus_StatusPending
}

func (s *service) disableCache(status *proto.GetSubscriptionResponse) {
	log.Info("disabling cache to wait for Active state")

	err := s.cache.CacheDisableForNextMinutes(cacheDisableMinutes)
	if err != nil {
		log.Warn("can not disable cache", zap.Error(err))
		// return nil, errors.Wrap(ErrCacheProblem, err.Error())
	}
}

func isNeedToEnableCache(status *proto.GetSubscriptionResponse) bool {
	isEnableCacheStatus := (status.Status != proto.SubscriptionStatus_StatusUnknown) && (status.Status != proto.SubscriptionStatus_StatusPending)
	isEnableCacheTier := status.Tier > uint32(proto.SubscriptionTier_TierExplorer)

	return isEnableCacheStatus && isEnableCacheTier
}

func (s *service) enableCache(status *proto.GetSubscriptionResponse) {
	log.Info("enabling cache again")

	// or it will be automatically enabled after N minutes of DisableForNextMinutes() call
	err := s.cache.CacheEnable()
	if err != nil {
		log.Warn("can not enable cache", zap.Error(err))
		// return nil, errors.Wrap(ErrCacheProblem, err.Error())
	}
}

// refreshSubscriptionStatusBackground performs background network refresh of subscription status
// This method CAN block on network calls and should only be used by periodic sync
func (s *service) refreshSubscriptionStatusBackground(ctx context.Context) error {
	// wrap context to stop in-flight request in case of component close
	ctx, cancel := contexthelper.ContextWithCloseChan(ctx, s.closing)
	defer cancel()

	// Acquire limiter to prevent concurrent requests
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.getSubscriptionLimiter <- struct{}{}:
		defer func() {
			<-s.getSubscriptionLimiter
		}()
	}

	// Get OLD cached status for comparison
	cachedStatus, _, cacheErr := s.cache.CacheGet()
	isCacheExpired := s.cache.IsCacheExpired()

	// Make network request to PP node
	ppReq, err := s.generateRequest()
	if err != nil {
		log.Warn("background refresh: failed to generate request", zap.Error(err))
		return err
	}

	log.Debug("background refresh: fetching subscription status from PP node")
	status, err := s.ppclient.GetSubscriptionStatus(ctx, ppReq)

	// On network error, try using cached data or create empty response
	if err != nil {
		log.Warn("background refresh: PP node error", zap.Error(err))

		// Try returning from cache
		if canReturnCachedStatus(cachedStatus) {
			log.Debug("background refresh: using cached status after network error")
			// Don't return error - we'll use cached data
			return nil
		}

		// Create empty response
		log.Info("background refresh: creating empty subscription status")
		status = &proto.GetSubscriptionResponse{
			Tier:   uint32(proto.SubscriptionTier_TierUnknown),
			Status: proto.SubscriptionStatus_StatusUnknown,
		}
	}

	out := convertMembershipStatus(status)

	// Save to cache
	err = s.cache.CacheSet(&out, nil)
	if err != nil {
		log.Warn("background refresh: can not save subscription status to cache", zap.Error(err))
	}

	log.Debug("background refresh: subscription status updated", zap.Any("status", status))

	// Check if update is required (status changed)
	if !isUpdateRequired(cacheErr, isCacheExpired, cachedStatus, status) {
		log.Debug("background refresh: subscription status has NOT changed")
		return nil
	}

	// Send events and update limits
	s.updateStatus(ctx, status)

	// Enable or disable cache based on status
	if isNeedToEnableCache(status) {
		s.enableCache(status)
	} else if isNeedToDisableCache(status) {
		s.disableCache(status)
	}

	return nil
}

// refreshTiersBackground performs background network refresh of tiers data
// This method CAN block on network calls and should only be used by periodic sync
func (s *service) refreshTiersBackground(ctx context.Context) error {
	// Get OLD cached tiers for comparison
	_, cachedTiers, cacheErr := s.cache.CacheGet()

	// Make network request
	bsr := proto.GetTiersRequest{
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),
		Locale:     "", // Use default locale for background refresh
	}

	payload, err := bsr.MarshalVT()
	if err != nil {
		log.Warn("background refresh tiers: can not marshal GetTiersRequest", zap.Error(err))
		return err
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Warn("background refresh tiers: can not sign GetTiersRequest", zap.Error(err))
		return err
	}

	reqSigned := proto.GetTiersRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	log.Debug("background refresh tiers: fetching tiers from PP node")
	tiers, err := s.ppclient.GetAllTiers(ctx, &reqSigned)
	if err != nil {
		log.Warn("background refresh tiers: PP node error", zap.Error(err))

		// If we have cached tiers, keep using them
		if cacheErr == nil && canReturnCachedTiers(cachedTiers) {
			log.Debug("background refresh tiers: using cached tiers after network error")
			return nil
		}

		// Create empty tiers response
		log.Info("background refresh tiers: creating empty tiers")
		tiers = &proto.GetTiersResponse{
			Tiers: make([]*proto.TierData, 0),
		}
	}

	// Convert to RPC response format
	var out pb.RpcMembershipGetTiersResponse
	out.Error = &pb.RpcMembershipGetTiersResponseError{
		Code: pb.RpcMembershipGetTiersResponseError_NULL,
	}

	out.Tiers = make([]*model.MembershipTierData, len(tiers.Tiers))
	for i, tier := range tiers.Tiers {
		out.Tiers[i] = convertTierData(tier)
	}

	// Save to cache
	err = s.cache.CacheSet(nil, &out)
	if err != nil {
		log.Warn("background refresh tiers: can not save tiers to cache", zap.Error(err))
	}

	log.Debug("background refresh tiers: tiers updated", zap.Int("count", len(out.Tiers)))

	// Check if tiers changed and send event
	if tiersChanged(cachedTiers, &out) {
		log.Info("background refresh tiers: tiers have changed, sending event")
		s.sendTiersUpdateEvent(&out)
	} else {
		log.Debug("background refresh tiers: tiers have NOT changed")
	}

	return nil
}

func (s *service) IsNameValid(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) (*pb.RpcMembershipIsNameValidResponse, error) {
	var code proto.IsNameValidResponse_Code
	var desc string

	out := pb.RpcMembershipIsNameValidResponse{
		Error: &pb.RpcMembershipIsNameValidResponseError{
			Code: pb.RpcMembershipIsNameValidResponseError_NULL,
		},
	}

	// 1 - send request to PP node and ask her please
	invr := proto.IsNameValidRequest{
		// payment node will check if signature matches with this OwnerAnyID
		RequestedTier:    req.RequestedTier,
		RequestedAnyName: nameservice.NsNameToFullName(req.NsName, req.NsNameType),
	}

	resp, err := s.ppclient.IsNameValid(ctx, &invr)
	if err != nil {
		return nil, err
	}

	if resp.Code == proto.IsNameValidResponse_Valid {
		// no error, now check if vacant in NS
		return s.checkIfNameAvailInNS(ctx, req)
	}

	out.Error = &pb.RpcMembershipIsNameValidResponseError{}
	code = resp.Code
	desc = resp.Description

	if code == proto.IsNameValidResponse_Valid {
		// no error, now check if vacant in NS
		return s.checkIfNameAvailInNS(ctx, req)
	}

	// 2 - convert code to error
	switch code {
	case proto.IsNameValidResponse_NoDotAny:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_BAD_INPUT
		out.Error.Description = "No .any at the end of the name"
	case proto.IsNameValidResponse_TooShort:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_TOO_SHORT
		out.Error.Description = "Name is too short"
	case proto.IsNameValidResponse_TooLong:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_TOO_LONG
		out.Error.Description = "Name is too long"
	case proto.IsNameValidResponse_HasInvalidChars:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_HAS_INVALID_CHARS
		out.Error.Description = "Name has invalid characters"
	case proto.IsNameValidResponse_TierFeatureNoName:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_TIER_FEATURES_NO_NAME
		out.Error.Description = "Tier does not support any names"
	case proto.IsNameValidResponse_CanNotReserve:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_CAN_NOT_RESERVE
		out.Error.Description = "Cannot reserve this name"
	default:
		out.Error.Code = pb.RpcMembershipIsNameValidResponseError_UNKNOWN_ERROR
		out.Error.Description = "Unknown error"
	}

	out.Error.Description = desc
	return &out, nil
}

func (s *service) checkIfNameAvailInNS(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) (*pb.RpcMembershipIsNameValidResponse, error) {
	// special backward compatibility logic for some clients
	// if name is empty -> return "it's OK"
	// because if you don't pass a name to MembershipIsNameValid() - means you don't want to reserve or change it
	// so ps.IsNameValid() returned no error for empty string
	//
	// and we should preserve that behavior also here
	if req.NsName == "" {
		return &pb.RpcMembershipIsNameValidResponse{
			Error: &pb.RpcMembershipIsNameValidResponseError{
				Code:        pb.RpcMembershipIsNameValidResponseError_NULL,
				Description: "",
			},
		}, nil
	}

	// check in the NameService if name is vacant (remote call #2)
	nsreq := pb.RpcNameServiceResolveNameRequest{
		NsName:     req.NsName,
		NsNameType: req.NsNameType,
	}
	nsout, err := s.ns.NameServiceResolveName(ctx, &nsreq)
	if err != nil {
		return nil, err
	}
	if !nsout.Available {
		return nil, ErrNameIsAlreadyReserved
	}

	return &pb.RpcMembershipIsNameValidResponse{
		Error: &pb.RpcMembershipIsNameValidResponseError{
			Code:        pb.RpcMembershipIsNameValidResponseError_NULL,
			Description: "",
		},
	}, nil
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

func (s *service) RegisterPaymentRequest(ctx context.Context, req *pb.RpcMembershipRegisterPaymentRequestRequest) (*pb.RpcMembershipRegisterPaymentRequestResponse, error) {
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

		UserEmail: req.UserEmail,
	}

	payload, err := bsr.MarshalVT()
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

	out := pb.RpcMembershipRegisterPaymentRequestResponse{
		PaymentUrl: bsRet.PaymentUrl,
		BillingId:  bsRet.BillingID,
		Error: &pb.RpcMembershipRegisterPaymentRequestResponseError{
			Code: pb.RpcMembershipRegisterPaymentRequestResponseError_NULL,
		},
	}

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after payment request is created on payment node")

	err = s.cache.CacheDisableForNextMinutes(cacheDisableMinutes)
	if err != nil {
		log.Warn("can not disable cache", zap.Error(err))
		// return nil, errors.Wrap(ErrCacheProblem, err.Error())
	}

	return &out, nil
}

func (s *service) GetPortalLink(ctx context.Context, req *pb.RpcMembershipGetPortalLinkUrlRequest) (*pb.RpcMembershipGetPortalLinkUrlResponse, error) {
	// 1 - send request
	bsr := proto.GetSubscriptionPortalLinkRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),
	}

	payload, err := bsr.MarshalVT()
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
	out.Error = &pb.RpcMembershipGetPortalLinkUrlResponseError{
		Code: pb.RpcMembershipGetPortalLinkUrlResponseError_NULL,
	}

	// 2 - disable cache for 30 minutes
	log.Debug("disabling cache for 30 minutes after portal link was received")
	err = s.cache.CacheDisableForNextMinutes(cacheDisableMinutes)
	if err != nil {
		log.Warn("can not disable cache", zap.Error(err))
		// return nil, errors.Wrap(ErrCacheProblem, err.Error())
	}

	return &out, nil
}

func (s *service) GetVerificationEmail(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) (*pb.RpcMembershipGetVerificationEmailResponse, error) {
	if req.IsOnboardingList {
		// special logics just for onboarding list:
		// use email collector to save email to the DB/PP node (should work offline too)
		err := s.emailCollector.SetRequest(req)
		if err != nil {
			log.Error("can not set email", zap.Error(err))
			return nil, err
		}

		// default OK response
		return &pb.RpcMembershipGetVerificationEmailResponse{
			Error: &pb.RpcMembershipGetVerificationEmailResponseError{
				Code: pb.RpcMembershipGetVerificationEmailResponseError_NULL,
			},
		}, nil
	}

	// send request to PP node directly
	out, err := s.emailCollector.SendRequest(ctx, req)
	if err != nil {
		log.Error("can not get verification email", zap.Error(err))
		return nil, err
	}

	return out, nil
}

func (s *service) VerifyEmailCode(ctx context.Context, req *pb.RpcMembershipVerifyEmailCodeRequest) (*pb.RpcMembershipVerifyEmailCodeResponse, error) {
	// 1 - send request
	bsr := proto.VerifyEmailRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId:      s.wallet.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress: s.wallet.GetAccountEthAddress().Hex(),
		Code:            req.Code,
	}

	payload, err := bsr.MarshalVT()
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
	log.Debug("disabling cache after email verification code was confirmed")
	err = s.cache.CacheDisableForNextMinutes(cacheDisableMinutes)
	if err != nil {
		log.Warn("can not disable cache", zap.Error(err))
		// return nil, errors.Wrap(ErrCacheProblem, err.Error())
	}

	// return out
	var out pb.RpcMembershipVerifyEmailCodeResponse
	out.Error = &pb.RpcMembershipVerifyEmailCodeResponseError{
		Code: pb.RpcMembershipVerifyEmailCodeResponseError_NULL,
	}

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

	payload, err := bsr.MarshalVT()
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
	log.Debug("disable cache after subscription was finalized")
	err = s.cache.CacheDisableForNextMinutes(cacheDisableMinutes)
	if err != nil {
		log.Warn("can not disable cache", zap.Error(err))
		// return nil, errors.Wrap(ErrCacheProblem, err.Error())
	}

	// return out
	var out pb.RpcMembershipFinalizeResponse
	out.Error = &pb.RpcMembershipFinalizeResponseError{
		Code: pb.RpcMembershipFinalizeResponseError_NULL,
	}

	return &out, nil
}

func (s *service) GetTiers(ctx context.Context, req *pb.RpcMembershipGetTiersRequest) (*pb.RpcMembershipGetTiersResponse, error) {
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

	filtered := &pb.RpcMembershipGetTiersResponse{
		Tiers: make([]*model.MembershipTierData, 0),
	}

	// if your are on 0-tier OR on Explorer -> return full list
	if (status != nil) && (status.Data != nil) && status.Data.Tier <= uint32(proto.SubscriptionTier_TierExplorer) {
		// if Explorer tier is current -> move it to the end of the list
		if len(out.Tiers) > 1 && (status.Data.Tier == uint32(proto.SubscriptionTier_TierExplorer)) && (out.Tiers[0].Id == uint32(proto.SubscriptionTier_TierExplorer)) {
			// Move Explorer tier to end of list
			firstTier := out.Tiers[0]
			restTiers := out.Tiers[1:]

			filtered.Tiers = make([]*model.MembershipTierData, 0, len(out.Tiers))
			filtered.Tiers = append(filtered.Tiers, restTiers...)
			filtered.Tiers = append(filtered.Tiers, firstTier)
			return filtered, nil
		}

		return out, nil
	}

	// If the current tier is higher than Explorer, show the list without Explorer (downgrading is not allowed)
	for _, tier := range out.Tiers {
		if tier.Id != uint32(proto.SubscriptionTier_TierExplorer) {
			filtered.Tiers = append(filtered.Tiers, tier)
		}
	}
	return filtered, nil
}

func canReturnCachedTiers(t *pb.RpcMembershipGetTiersResponse) bool {
	return t != nil && t.Tiers != nil && (t.Error == nil || t.Error.Code == pb.RpcMembershipGetTiersResponseError_NULL)
}

// getAllTiers returns tiers from cache ONLY
// This method NEVER makes network calls and returns immediately
// Background refresh happens via refreshTiersBackground()
func (s *service) getAllTiers(ctx context.Context, req *pb.RpcMembershipGetTiersRequest) (*pb.RpcMembershipGetTiersResponse, error) {
	// Check if NoCache is requested but not enabled
	if req.NoCache && !isNoCacheEnabled() {
		log.Warn("NoCache flag is deprecated and ignored for tiers. Set ANYTYPE_ENABLE_NOCACHE=1 to enable")
	}

	// ALWAYS try to return from cache (ignore NoCache unless env var is set)
	_, cachedTiers, cacheErr := s.cache.CacheGet()

	// If we have cached tiers (even if expired/disabled), return them
	if cacheErr == nil && canReturnCachedTiers(cachedTiers) {
		log.Debug("returning tiers from cache (RPC)", zap.Any("cachedTiers", cachedTiers))
		return cachedTiers, nil
	}

	// Cache is empty - return empty tiers list (background sync will populate it)
	log.Debug("cache is empty, returning empty tiers list")
	emptyTiers := &pb.RpcMembershipGetTiersResponse{
		Tiers: make([]*model.MembershipTierData, 0),
		Error: &pb.RpcMembershipGetTiersResponseError{
			Code: pb.RpcMembershipGetTiersResponseError_NULL,
		},
	}

	return emptyTiers, nil
}

func (s *service) VerifyAppStoreReceipt(ctx context.Context, req *pb.RpcMembershipVerifyAppStoreReceiptRequest) (*pb.RpcMembershipVerifyAppStoreReceiptResponse, error) {
	verifyReq := proto.VerifyAppStoreReceiptRequest{
		// payment node will check if signature matches with this OwnerAnyID
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),
		Receipt:    req.Receipt,
	}

	payload, err := verifyReq.MarshalVT()
	if err != nil {
		log.Error("can not marshal VerifyAppStoreReceiptRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign VerifyAppStoreReceiptRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.VerifyAppStoreReceiptRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	_, err = s.ppclient.VerifyAppStoreReceipt(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	return &pb.RpcMembershipVerifyAppStoreReceiptResponse{
		Error: &pb.RpcMembershipVerifyAppStoreReceiptResponseError{
			Code: pb.RpcMembershipVerifyAppStoreReceiptResponseError_NULL,
		},
	}, nil
}

func (s *service) CodeGetInfo(ctx context.Context, req *pb.RpcMembershipCodeGetInfoRequest) (*pb.RpcMembershipCodeGetInfoResponse, error) {
	code := req.Code

	codeInfo := proto.CodeGetInfoRequest{
		OwnerAnyId:      s.wallet.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress: s.wallet.GetAccountEthAddress().Hex(),
		Code:            code,
	}

	payload, err := codeInfo.MarshalVT()
	if err != nil {
		log.Error("can not marshal CodeGetInfoRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign CodeGetInfoRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.CodeGetInfoRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	res, err := s.ppclient.CodeGetInfo(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	// send membership update to the payment node
	// to get new tiers, because Code can redeem a hidden tier that is not on the list yet
	_, err = s.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{
		NoCache: true,
	})
	if err != nil {
		log.Error("can not get subscription status again", zap.Error(err))
		// eat the error...
	}

	return &pb.RpcMembershipCodeGetInfoResponse{
		RequestedTier: res.Tier,
		Error: &pb.RpcMembershipCodeGetInfoResponseError{
			Code: pb.RpcMembershipCodeGetInfoResponseError_NULL,
		},
	}, nil
}

func (s *service) CodeRedeem(ctx context.Context, req *pb.RpcMembershipCodeRedeemRequest) (*pb.RpcMembershipCodeRedeemResponse, error) {
	code := req.Code
	nsName := req.NsName
	nsNameType := req.NsNameType

	codeRedeem := proto.CodeRedeemRequest{
		OwnerAnyId:       s.wallet.Account().SignKey.GetPublic().Account(),
		OwnerEthAddress:  s.wallet.GetAccountEthAddress().Hex(),
		Code:             code,
		RequestedAnyName: nameservice.NsNameToFullName(nsName, nsNameType),
	}

	payload, err := codeRedeem.MarshalVT()

	if err != nil {
		log.Error("can not marshal CodeRedeemRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Error("can not sign CodeRedeemRequest", zap.Error(err))
		return nil, ErrCanNotSign
	}

	reqSigned := proto.CodeRedeemRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	res, err := s.ppclient.CodeRedeem(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	if !res.Success {
		log.Error("code redemption failed", zap.String("code", code))
		// return this error as if code was not found
		return nil, proto.ErrCodeNotFound
	}

	return &pb.RpcMembershipCodeRedeemResponse{
		Error: &pb.RpcMembershipCodeRedeemResponseError{
			Code: pb.RpcMembershipCodeRedeemResponseError_NULL,
		},
	}, nil
}
