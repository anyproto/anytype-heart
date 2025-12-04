package payments

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	ppclient2 "github.com/anyproto/any-sync/paymentservice/paymentserviceclient2"
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
)

const CName = "payments"

var log = logging.Logger(CName)

var (
	refreshIntervalSecs  = 60
	forceRefreshInterval = 10 * time.Second
	networkTimeout       = 60 * time.Second
	networkTimeout2      = 90 * time.Second
)

var (
	ErrCanNotSign            = errors.New("can not sign")
	ErrCacheProblem          = errors.New("cache problem")
	ErrNoConnection          = errors.New("can not connect to payment node")
	ErrV2NotEnabled          = errors.New("membership v2 call not enabled in AccountSelect or AccountCreate")
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
	proto.PaymentMethod_MethodNone:        model.Membership_MethodNone,
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

	V2GetPortalLink(ctx context.Context, req *pb.RpcMembershipV2GetPortalLinkRequest) (*pb.RpcMembershipV2GetPortalLinkResponse, error)
	V2GetProducts(ctx context.Context, req *pb.RpcMembershipV2GetProductsRequest) (*pb.RpcMembershipV2GetProductsResponse, error)
	V2GetStatus(ctx context.Context, req *pb.RpcMembershipV2GetStatusRequest) (*pb.RpcMembershipV2GetStatusResponse, error)
	V2AnyNameIsValid(ctx context.Context, req *pb.RpcMembershipV2AnyNameIsValidRequest) (*pb.RpcMembershipV2AnyNameIsValidResponse, error)
	V2AnyNameAllocate(ctx context.Context, req *pb.RpcMembershipV2AnyNameAllocateRequest) (*pb.RpcMembershipV2AnyNameAllocateResponse, error)
	V2CartGet(ctx context.Context, req *pb.RpcMembershipV2CartGetRequest) (*pb.RpcMembershipV2CartGetResponse, error)
	V2CartUpdate(ctx context.Context, req *pb.RpcMembershipV2CartUpdateRequest) (*pb.RpcMembershipV2CartUpdateResponse, error)
	app.ComponentRunnable
}

func New() Service {
	return &service{}
}

type service struct {
	cfg       *config.Config
	cache     cache.CacheService
	ppclient  ppclient.AnyPpClientService
	ppclient2 ppclient2.AnyPpClientServiceV2

	wallet                 wallet.Wallet
	getSubscriptionLimiter chan struct{}
	getStatusV2Limiter     chan struct{}
	eventSender            event.Sender
	profileUpdater         globalNamesUpdater
	ns                     nameservice.Service
	componentCtx           context.Context
	componentCtxCancel     context.CancelFunc

	refreshCtrl              *refreshController
	refreshCtrlV2            *refreshController
	multiplayerLimitsUpdater deletioncontroller.DeletionController
	fileLimitsUpdater        filesync.FileSync
	emailCollector           emailcollector.EmailCollector
}

type refreshController struct {
	ctx           context.Context
	cancel        context.CancelFunc
	fetch         func(ctx context.Context, forceFetch bool) (bool, error)
	interval      time.Duration
	forceInterval time.Duration
	forceCh       chan time.Duration
	closeCh       chan struct{}
	now           func() time.Time
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.cfg = app.MustComponent[*config.Config](a)
	s.cache = app.MustComponent[cache.CacheService](a)
	s.emailCollector = app.MustComponent[emailcollector.EmailCollector](a)
	s.ppclient = app.MustComponent[ppclient.AnyPpClientService](a)
	s.ppclient2 = app.MustComponent[ppclient2.AnyPpClientServiceV2](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.ns = app.MustComponent[nameservice.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.profileUpdater = app.MustComponent[globalNamesUpdater](a)
	s.multiplayerLimitsUpdater = app.MustComponent[deletioncontroller.DeletionController](a)
	s.fileLimitsUpdater = app.MustComponent[filesync.FileSync](a)
	s.getSubscriptionLimiter = make(chan struct{}, 1)
	s.getStatusV2Limiter = make(chan struct{}, 1)
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())

	return nil
}

func (s *service) Run(ctx context.Context) (err error) {
	// this parameter is set in the AccountSelect and AccountCreate commands
	if !s.cfg.EnableMembershipV2 {
		log.Info("starting v1 refresh controller")

		fetchFn := func(baseCtx context.Context, forceFetch bool) (bool, error) {
			fetchCtx, cancel := context.WithTimeout(baseCtx, networkTimeout)
			defer cancel()
			changed, _, _, err := s.fetchAndUpdate(fetchCtx, forceFetch, true, true)
			return changed, err
		}

		s.refreshCtrl = newRefreshController(s.componentCtx, fetchFn, time.Second*time.Duration(refreshIntervalSecs), forceRefreshInterval)
		s.refreshCtrl.Start()
	} else {
		// Start V2 refresh controller
		log.Info("starting V2 refresh controller")

		fetchFnV2 := func(baseCtx context.Context, forceFetch bool) (bool, error) {
			fetchCtx, cancel := context.WithTimeout(baseCtx, networkTimeout2)
			defer cancel()
			changed, _, _, err := s.fetchAndUpdateV2(fetchCtx, forceFetch, true, true)
			return changed, err
		}

		s.refreshCtrlV2 = newRefreshController(s.componentCtx, fetchFnV2, time.Second*time.Duration(refreshIntervalSecs), forceRefreshInterval)
		s.refreshCtrlV2.Start()
	}

	return nil
}

func (s *service) Close(_ context.Context) (err error) {
	if s.refreshCtrl != nil {
		s.refreshCtrl.Stop()
		s.refreshCtrl = nil
	}
	if s.refreshCtrlV2 != nil {
		s.refreshCtrlV2.Stop()
		s.refreshCtrlV2 = nil
	}
	s.componentCtxCancel()
	return nil
}

// forceRefresh performs more aggressive fetching of subscription status and tiers.
func (s *service) forceRefresh(duration time.Duration) {
	if s.refreshCtrl == nil {
		return
	}
	s.refreshCtrl.Force(duration)
}

// forceRefreshV2 performs more aggressive fetching of V2 subscription status.
func (s *service) forceRefreshV2(duration time.Duration) {
	if s.refreshCtrlV2 == nil {
		return
	}
	s.refreshCtrlV2.Force(duration)
}

func (s *service) fetchAndUpdate(ctx context.Context, forceIfNotExpired, fetchTiers, fetchMembership bool) (changed bool, tiers []*model.MembershipTierData, membership *model.Membership, err error) {
	// skip running loop if we are in local-only mode
	if s.cfg.GetNetworkMode() == pb.RpcAccount_LocalOnly {
		// do not trace to log to prevent spamming
		return false, nil, nil, nil
	}

	cachedStatus, cachedTiers, cacheExpirationTime, cacheErr := s.cache.CacheGet()
	if cacheErr != nil {
		log.Debug("periodic refresh: can not get from cache", zap.Error(cacheErr))
	}
	if !forceIfNotExpired && cacheExpirationTime.After(time.Now()) {
		return false, cachedTiers, cachedStatus, nil
	}
	var errs []error
	tiers = cachedTiers
	membership = cachedStatus

	if fetchTiers {
		fetchedTiers, fetchErr := s.fetchTiers(ctx)
		if fetchErr != nil {
			log.Warn("periodic refresh: tiers update failed", zap.Error(fetchErr))
			errs = append(errs, fetchErr)
		} else {
			if !tiersAreEqual(cachedTiers, fetchedTiers) {
				fmt.Printf("%+v\n", fetchedTiers)
				fmt.Printf("%+v\n", cachedTiers)
				log.Warn("background refresh tiers: tiers have changed, sending event")
				s.sendTiersUpdateEvent(fetchedTiers)
				changed = true
			}
			tiers = fetchedTiers
		}
	}

	if fetchMembership {
		fetchedMembership, fetchErr := s.fetchMembership(ctx)
		if fetchErr != nil {
			log.Warn("periodic refresh: subscription status update failed", zap.Error(fetchErr))
			errs = append(errs, fetchErr)
		} else {
			if !fetchedMembership.Equal(cachedStatus) {
				log.Warn("background refresh membership: membership has changed, sending event")
				s.sendMembershipUpdateEvent(fetchedMembership)
				changed = true
			}
			membership = fetchedMembership
		}
	}

	if changed {
		if cacheSetErr := s.cache.CacheSet(membership, tiers); cacheSetErr != nil {
			log.Warn("periodic refresh: can not set to cache", zap.Error(cacheSetErr))
		}
		if limitsErr := s.updateLimits(ctx); limitsErr != nil {
			log.Warn("periodic refresh: limits update failed", zap.Error(limitsErr))
		}
	}

	if membership == nil {
		membership = &model.Membership{
			Tier:   uint32(proto.SubscriptionTier_TierUnknown),
			Status: model.Membership_StatusUnknown,
		}
	}
	if tiers == nil {
		tiers = []*model.MembershipTierData{}
	}

	if len(errs) > 0 {
		err = errors.Join(errs...)
	}

	return
}

func (s *service) fetchAndUpdateV2(ctx context.Context, forceIfNotExpired, fetchMembership, fetchProducts bool) (changed bool, membership *model.MembershipV2Data, products []*model.MembershipV2Product, err error) {
	// skip running loop if we are in local-only mode
	if s.cfg.GetNetworkMode() == pb.RpcAccount_LocalOnly {
		// do not trace to log to prevent spamming
		return false, nil, nil, nil
	}

	cachedData, cacheExpirationTime, cacheErr := s.cache.CacheV2Get()
	cachedProducts, _, productsCacheErr := s.cache.CacheV2ProductsGet()
	if cacheErr != nil {
		log.Debug("periodic refresh: can not get V2 membership status from cache", zap.Error(cacheErr))
	}
	if productsCacheErr != nil {
		log.Debug("periodic refresh: can not get V2 products from cache", zap.Error(productsCacheErr))
	}
	if !forceIfNotExpired && cacheExpirationTime.After(time.Now()) {
		return false, cachedData, cachedProducts, nil
	}
	var errs []error
	membership = cachedData
	products = cachedProducts

	if fetchProducts {
		fetchedProducts, fetchErr := s.fetchV2Products(ctx)
		if fetchErr != nil {
			log.Warn("periodic refresh: V2 products update failed", zap.Error(fetchErr))
			errs = append(errs, fetchErr)
		} else {
			if !productsV2Equal(cachedProducts, fetchedProducts) {
				log.Warn("background refresh V2 products: products have changed, sending event", zap.Any("cachedProducts", cachedProducts), zap.Any("fetchedProducts", fetchedProducts))
				s.sendMembershipV2ProductsUpdateEvent(fetchedProducts)
				changed = true
			}
			products = fetchedProducts
		}
	}

	if fetchMembership {
		fetchedMembership, fetchErr := s.fetchV2Membership(ctx)
		if fetchErr != nil {
			log.Warn("periodic refresh: V2 subscription status update failed", zap.Error(fetchErr))
			errs = append(errs, fetchErr)
		} else {
			// Compare V2 data - check if Products or NextInvoice changed
			if !membershipV2DataEqual(cachedData, fetchedMembership) {
				log.Warn("background refresh V2 membership: membership has changed, sending event", zap.Any("cachedData", cachedData), zap.Any("fetchedMembership", fetchedMembership))

				s.sendMembershipV2UpdateEvent(fetchedMembership)
				changed = true
			}
			membership = fetchedMembership
		}
	}

	if changed {
		s.updateCacheAndLimitsV2(ctx, membership, products)
	}

	if membership == nil {
		membership = &model.MembershipV2Data{
			Products:    []*model.MembershipV2PurchasedProduct{},
			NextInvoice: nil,
		}
	}
	if products == nil {
		products = []*model.MembershipV2Product{}
	}

	if len(errs) > 0 {
		err = errors.Join(errs...)
	}

	return
}

func (s *service) updateCacheAndLimitsV2(ctx context.Context, membership *model.MembershipV2Data, products []*model.MembershipV2Product) {
	if membership != nil {
		if cacheSetErr := s.cache.CacheV2Set(membership); cacheSetErr != nil {
			log.Warn("periodic refresh: can not set V2 membership status to cache", zap.Error(cacheSetErr))
		}
	}
	if products != nil {
		if productsCacheSetErr := s.cache.CacheV2ProductsSet(products); productsCacheSetErr != nil {
			log.Warn("periodic refresh: can not set V2 products to cache", zap.Error(productsCacheSetErr))
		}
	}
	if limitsErr := s.updateLimits(ctx); limitsErr != nil {
		log.Warn("periodic refresh: limits update failed", zap.Error(limitsErr))
	}
}

func (s *service) sendMembershipUpdateEvent(membership *model.Membership) {
	// send ANY name update
	if membership != nil {
		nsName := membership.GetNsName()
		nsNameType := membership.GetNsNameType()

		s.profileUpdater.UpdateOwnGlobalName(nameservice.NsNameToFullName(nsName, nsNameType))
	}

	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfMembershipUpdate{
		MembershipUpdate: &pb.EventMembershipUpdate{
			Data: membership,
		},
	}))
}

func (s *service) sendTiersUpdateEvent(tiers []*model.MembershipTierData) {
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfMembershipTiersUpdate{
		MembershipTiersUpdate: &pb.EventMembershipTiersUpdate{
			Tiers: tiers,
		},
	}))
}

// GetSubscriptionStatus returns subscription status from cache ONLY
// This method NEVER makes network calls and returns immediately
// Background refresh happens via refreshSubscriptionStatusBackground()
func (s *service) GetSubscriptionStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) (*pb.RpcMembershipGetStatusResponse, error) {
	var (
		membership *model.Membership
		err        error
	)

	_, _, membership, err = s.fetchAndUpdate(ctx, req.NoCache, false, req.NoCache)
	if err != nil && req.NoCache && !errors.Is(err, cache.ErrCacheDbError) {
		return nil, err
	}
	if membership == nil {
		membership = &model.Membership{
			Tier:   uint32(proto.SubscriptionTier_TierUnknown),
			Status: model.Membership_StatusUnknown,
		}
	}

	status := &pb.RpcMembershipGetStatusResponse{
		Data: membership,
		Error: &pb.RpcMembershipGetStatusResponseError{
			Code: pb.RpcMembershipGetStatusResponseError_NULL,
		},
	}

	return status, nil
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

func (s *service) updateLimits(ctx context.Context) error {
	s.multiplayerLimitsUpdater.UpdateCoordinatorStatus()
	return s.fileLimitsUpdater.UpdateNodeUsage(ctx)
}

// fetchSubscriptionStatus performs network refresh of subscription status
func (s *service) fetchMembership(ctx context.Context) (*model.Membership, error) {
	// Acquire limiter to prevent concurrent requests
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.getSubscriptionLimiter <- struct{}{}:
		defer func() {
			<-s.getSubscriptionLimiter
		}()
	}

	// Make network request to PP node
	ppReq, err := s.generateRequest()
	if err != nil {
		log.Warn("background refresh: failed to generate request", zap.Error(err))
		return nil, err
	}

	log.Debug("background refresh: fetching subscription status from PP node")
	status, err := s.ppclient.GetSubscriptionStatus(ctx, ppReq)

	// On network error, try using cached data or create empty response
	if err != nil {
		return nil, err
	}

	return convertMembershipData(status), nil
}

// fetchV2Membership performs network refresh of V2 membership status
func (s *service) fetchV2Membership(ctx context.Context) (*model.MembershipV2Data, error) {
	// Acquire limiter to prevent concurrent requests
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.getStatusV2Limiter <- struct{}{}:
		defer func() {
			<-s.getStatusV2Limiter
		}()
	}

	// Make network request to PP node
	in := proto.MembershipV2_GetStatusRequest{}

	log.Debug("background refresh: fetching V2 subscription status from PP node")
	out, err := s.ppclient2.GetStatus(ctx, &in)

	// On network error, return error
	if err != nil {
		return nil, err
	}

	// convert Products
	var productsModel []*model.MembershipV2PurchasedProduct
	if len(out.Products) > 0 {
		productsModel = make([]*model.MembershipV2PurchasedProduct, len(out.Products))
		for i, product := range out.Products {
			productsModel[i] = convertPurchasedProductData(product)
		}
	}

	// convert NextInvoice
	nextInvoiceModel := convertInvoiceData(out.NextInvoice)

	return &model.MembershipV2Data{
		Products:        productsModel,
		NextInvoice:     nextInvoiceModel,
		TeamOwnerID:     out.TeamOwnerID,
		PaymentProvider: model.MembershipV2PaymentProvider(out.PaymentProvider),
	}, nil
}

// fetchV2Products performs network fetch of V2 products data
func (s *service) fetchV2Products(ctx context.Context) ([]*model.MembershipV2Product, error) {
	// Make network request
	productsReq := proto.MembershipV2_GetProductsRequest{}

	log.Debug("background refresh: fetching V2 products from PP node")
	products, err := s.ppclient2.GetProducts(ctx, &productsReq)
	if err != nil {
		return nil, err
	}

	var modelProducts []*model.MembershipV2Product
	if len(products.Products) > 0 {
		modelProducts = make([]*model.MembershipV2Product, len(products.Products))
		for i, product := range products.Products {
			modelProducts[i] = convertProductData(product)
		}
	}

	return modelProducts, nil
}

// fetchTiersBackground performs network fetch of tiers data
func (s *service) fetchTiers(ctx context.Context) ([]*model.MembershipTierData, error) {
	// Make network request
	bsr := proto.GetTiersRequest{
		OwnerAnyId: s.wallet.Account().SignKey.GetPublic().Account(),
		Locale:     "",    // Use default locale for background refresh
		Version:    "2.0", // Use default (new) version
	}

	payload, err := bsr.MarshalVT()
	if err != nil {
		log.Warn("background refresh tiers: can not marshal GetTiersRequest", zap.Error(err))
		return nil, err
	}

	privKey := s.wallet.GetAccountPrivkey()
	signature, err := privKey.Sign(payload)
	if err != nil {
		log.Warn("background refresh tiers: can not sign GetTiersRequest", zap.Error(err))
		return nil, err
	}

	reqSigned := proto.GetTiersRequestSigned{
		Payload:   payload,
		Signature: signature,
	}

	log.Debug("background refresh tiers: fetching tiers from PP node")
	tiers, err := s.ppclient.GetAllTiers(ctx, &reqSigned)
	if err != nil {
		return nil, err
	}

	var modelTiers []*model.MembershipTierData
	if len(tiers.Tiers) > 0 {
		modelTiers = make([]*model.MembershipTierData, len(tiers.Tiers))
		for i, tier := range tiers.Tiers {
			modelTiers[i] = convertTierData(tier)
		}
	}

	return modelTiers, nil
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

	go s.forceRefresh(30 * time.Minute)

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

	go s.forceRefresh(30 * time.Minute)

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

	// update any name immediately (optimistic)
	s.profileUpdater.UpdateOwnGlobalName(nameservice.NsNameToFullName(req.NsName, req.NsNameType))

	// 2 - clear cache
	go s.forceRefresh(30 * time.Minute)

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

// getAllTiers returns tiers from cache ONLY
// This method NEVER makes network calls and returns immediately
// Background refresh happens via refreshTiersBackground()
func (s *service) getAllTiers(ctx context.Context, req *pb.RpcMembershipGetTiersRequest) (*pb.RpcMembershipGetTiersResponse, error) {
	_, tiers, _, err := s.fetchAndUpdate(ctx, req.NoCache, req.NoCache, false)
	if err != nil {
		return nil, err
	}

	return &pb.RpcMembershipGetTiersResponse{
		Tiers: tiers,
		Error: &pb.RpcMembershipGetTiersResponseError{
			Code: pb.RpcMembershipGetTiersResponseError_NULL,
		},
	}, nil
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

	// immediately update own any name, do not wait for background refresh
	s.profileUpdater.UpdateOwnGlobalName(nameservice.NsNameToFullName(nsName, nsNameType))

	go s.forceRefresh(30 * time.Minute)

	// 2 - force refresh v2 to get updated membership status
	go s.forceRefreshV2(30 * time.Minute)

	return &pb.RpcMembershipCodeRedeemResponse{
		Error: &pb.RpcMembershipCodeRedeemResponseError{
			Code: pb.RpcMembershipCodeRedeemResponseError_NULL,
		},
	}, nil
}

func (s *service) V2GetPortalLink(ctx context.Context, req *pb.RpcMembershipV2GetPortalLinkRequest) (*pb.RpcMembershipV2GetPortalLinkResponse, error) {
	if !s.cfg.EnableMembershipV2 {
		return nil, ErrV2NotEnabled
	}

	webAuth := proto.MembershipV2_WebAuthRequest{}

	res, err := s.ppclient2.WebAuth(ctx, &webAuth)
	if err != nil {
		return nil, err
	}

	go s.forceRefreshV2(30 * time.Minute)

	return &pb.RpcMembershipV2GetPortalLinkResponse{
		Url: res.Url,

		Error: &pb.RpcMembershipV2GetPortalLinkResponseError{
			Code: pb.RpcMembershipV2GetPortalLinkResponseError_NULL,
		},
	}, nil
}

func (s *service) V2GetProducts(ctx context.Context, req *pb.RpcMembershipV2GetProductsRequest) (*pb.RpcMembershipV2GetProductsResponse, error) {
	if !s.cfg.EnableMembershipV2 {
		return nil, ErrV2NotEnabled
	}

	// Get all products from cache (including background refresh if needed)
	products, err := s.getAllV2Products(ctx, req)
	if err != nil {
		return nil, err
	}

	return &pb.RpcMembershipV2GetProductsResponse{
		Products: products,
		Error: &pb.RpcMembershipV2GetProductsResponseError{
			Code: pb.RpcMembershipV2GetProductsResponseError_NULL,
		},
	}, nil
}

// getAllV2Products returns products from cache ONLY
// This method NEVER makes network calls and returns immediately
// Background refresh happens via refreshSubscriptionStatusBackground()
func (s *service) getAllV2Products(ctx context.Context, req *pb.RpcMembershipV2GetProductsRequest) ([]*model.MembershipV2Product, error) {
	_, _, products, err := s.fetchAndUpdateV2(ctx, req.NoCache, false, req.NoCache)
	if err != nil {
		return nil, err
	}

	return products, nil
}

// V2GetStatus returns V2 subscription status from cache ONLY
// This method NEVER makes network calls and returns immediately
// Background refresh happens via refreshSubscriptionStatusBackground()
func (s *service) V2GetStatus(ctx context.Context, req *pb.RpcMembershipV2GetStatusRequest) (*pb.RpcMembershipV2GetStatusResponse, error) {
	if !s.cfg.EnableMembershipV2 {
		return nil, ErrV2NotEnabled
	}

	var (
		membership *model.MembershipV2Data
		err        error
	)

	_, membership, _, err = s.fetchAndUpdateV2(ctx, req.NoCache, req.NoCache, false)
	if err != nil && req.NoCache && !errors.Is(err, cache.ErrCacheDbError) {
		return nil, err
	}
	if membership == nil {
		membership = &model.MembershipV2Data{
			Products:    []*model.MembershipV2PurchasedProduct{},
			NextInvoice: nil,
		}
	}

	status := &pb.RpcMembershipV2GetStatusResponse{
		Data: membership,
		Error: &pb.RpcMembershipV2GetStatusResponseError{
			Code: pb.RpcMembershipV2GetStatusResponseError_NULL,
		},
	}

	return status, nil
}

func (s *service) v2CheckIfNameAvailInNS(ctx context.Context, req *pb.RpcMembershipV2AnyNameIsValidRequest) (*pb.RpcMembershipV2AnyNameIsValidResponse, error) {
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

	return &pb.RpcMembershipV2AnyNameIsValidResponse{
		Error: &pb.RpcMembershipV2AnyNameIsValidResponseError{
			Code:        pb.RpcMembershipV2AnyNameIsValidResponseError_NULL,
			Description: "",
		},
	}, nil
}

func (s *service) V2AnyNameIsValid(ctx context.Context, req *pb.RpcMembershipV2AnyNameIsValidRequest) (*pb.RpcMembershipV2AnyNameIsValidResponse, error) {
	if !s.cfg.EnableMembershipV2 {
		return nil, ErrV2NotEnabled
	}

	var code proto.MembershipV2_AnyNameIsValidResponse_Code
	var desc string

	out := pb.RpcMembershipV2AnyNameIsValidResponse{
		Error: &pb.RpcMembershipV2AnyNameIsValidResponseError{
			Code: pb.RpcMembershipV2AnyNameIsValidResponseError_NULL,
		},
	}

	// 1 - send request to PP node and ask her please
	invr := proto.MembershipV2_AnyNameIsValidRequest{
		RequestedAnyName: nameservice.NsNameToFullName(req.NsName, req.NsNameType),
	}

	resp, err := s.ppclient2.AnyNameIsValid(ctx, &invr)
	if err != nil {
		return nil, err
	}

	if resp.Code == proto.MembershipV2_AnyNameIsValidResponse_Valid {
		// no error, now check if vacant in NS
		return s.v2CheckIfNameAvailInNS(ctx, req)
	}

	out.Error = &pb.RpcMembershipV2AnyNameIsValidResponseError{}
	code = resp.Code
	desc = resp.Description

	if code == proto.MembershipV2_AnyNameIsValidResponse_Valid {
		// no error, now check if vacant in NS
		return s.v2CheckIfNameAvailInNS(ctx, req)
	}

	// 2 - convert code to error
	switch code {
	case proto.MembershipV2_AnyNameIsValidResponse_NoDotAny:
		out.Error.Code = pb.RpcMembershipV2AnyNameIsValidResponseError_BAD_INPUT
		out.Error.Description = "No .any at the end of the name"
	case proto.MembershipV2_AnyNameIsValidResponse_TooShort:
		out.Error.Code = pb.RpcMembershipV2AnyNameIsValidResponseError_TOO_SHORT
		out.Error.Description = "Name is too short"
	case proto.MembershipV2_AnyNameIsValidResponse_TooLong:
		out.Error.Code = pb.RpcMembershipV2AnyNameIsValidResponseError_TOO_LONG
		out.Error.Description = "Name is too long"
	case proto.MembershipV2_AnyNameIsValidResponse_HasInvalidChars:
		out.Error.Code = pb.RpcMembershipV2AnyNameIsValidResponseError_HAS_INVALID_CHARS
		out.Error.Description = "Name has invalid characters"
	case proto.MembershipV2_AnyNameIsValidResponse_AccountHasNoName:
		out.Error.Code = pb.RpcMembershipV2AnyNameIsValidResponseError_ACCOUNT_FEATURES_NO_NAME
		out.Error.Description = "Account does not have any name enabled"
	default:
		out.Error.Code = pb.RpcMembershipV2AnyNameIsValidResponseError_UNKNOWN_ERROR
		out.Error.Description = "Unknown error"
	}

	out.Error.Description = desc
	return &out, nil
}

func (s *service) V2AnyNameAllocate(ctx context.Context, req *pb.RpcMembershipV2AnyNameAllocateRequest) (*pb.RpcMembershipV2AnyNameAllocateResponse, error) {
	if !s.cfg.EnableMembershipV2 {
		return nil, ErrV2NotEnabled
	}

	// 1 - send request
	anar := proto.MembershipV2_AnyNameAllocateRequest{
		RequestedAnyName: nameservice.NsNameToFullName(req.NsName, req.NsNameType),
		OwnerEthAddress:  s.wallet.GetAccountEthAddress().Hex(),
	}

	// empty return or error
	_, err := s.ppclient2.AnyNameAllocate(ctx, &anar)
	if err != nil {
		return nil, err
	}

	if err == nil {
		// immediately update own any name, do not wait for background refresh
		s.profileUpdater.UpdateOwnGlobalName(nameservice.NsNameToFullName(req.NsName, req.NsNameType))
	}

	// 2 - force refresh to get updated membership status
	go s.forceRefreshV2(30 * time.Minute)

	// return out
	var out pb.RpcMembershipV2AnyNameAllocateResponse
	out.Error = &pb.RpcMembershipV2AnyNameAllocateResponseError{
		Code: pb.RpcMembershipV2AnyNameAllocateResponseError_NULL,
	}

	return &out, nil
}

func (s *service) V2CartGet(ctx context.Context, req *pb.RpcMembershipV2CartGetRequest) (*pb.RpcMembershipV2CartGetResponse, error) {
	if !s.cfg.EnableMembershipV2 {
		return nil, ErrV2NotEnabled
	}

	cartReq := proto.MembershipV2_StoreCartGetRequest{}

	cart, err := s.ppclient2.StoreCartGet(ctx, &cartReq)
	if err != nil {
		return nil, err
	}

	cartModel := convertCartData(cart)
	return &pb.RpcMembershipV2CartGetResponse{
		Cart: cartModel,
		Error: &pb.RpcMembershipV2CartGetResponseError{
			Code: pb.RpcMembershipV2CartGetResponseError_NULL,
		},
	}, nil
}

func (s *service) V2CartUpdate(ctx context.Context, req *pb.RpcMembershipV2CartUpdateRequest) (*pb.RpcMembershipV2CartUpdateResponse, error) {
	if !s.cfg.EnableMembershipV2 {
		return nil, ErrV2NotEnabled
	}

	products := make([]*proto.MembershipV2_CartProduct, len(req.ProductIds))
	for i, productId := range req.ProductIds {
		products[i] = &proto.MembershipV2_CartProduct{
			Product: &proto.MembershipV2_Product{
				// specify only the ID of the product
				Id: productId,
			},
			IsYearly: req.IsYearly,

			// add to cart
			Remove: false,
		}
	}

	cartReq := proto.MembershipV2_StoreCartUpdateRequest{
		Products: products,

		OwnerEthAddress: s.wallet.GetAccountEthAddress().Hex(),
	}

	_, err := s.ppclient2.StoreCartUpdate(ctx, &cartReq)
	if err != nil {
		return nil, err
	}

	return &pb.RpcMembershipV2CartUpdateResponse{
		Error: &pb.RpcMembershipV2CartUpdateResponseError{
			Code: pb.RpcMembershipV2CartUpdateResponseError_NULL,
		},
	}, nil
}

func (s *service) sendMembershipV2UpdateEvent(membership *model.MembershipV2Data) {
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfMembershipV2Update{
		MembershipV2Update: &pb.EventMembershipV2Update{
			Data: membership,
		},
	}))
}

func (s *service) sendMembershipV2ProductsUpdateEvent(products []*model.MembershipV2Product) {
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfMembershipV2ProductsUpdate{
		MembershipV2ProductsUpdate: &pb.EventMembershipV2ProductsUpdate{
			Products: products,
		},
	}))
}
