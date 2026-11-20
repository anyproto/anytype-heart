package payments

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "payments"

// Exported sentinel errors used by middleware.
var (
	ErrCanNotSign            = errors.New("can not sign")
	ErrCacheProblem          = errors.New("cache problem")
	ErrNoConnection          = errors.New("can not connect to payment node")
	ErrV2NotEnabled          = errors.New("membership v2 call not enabled in AccountSelect or AccountCreate")
	ErrNoTiers               = errors.New("can not get tiers")
	ErrNoTierFound           = errors.New("can not find requested tier")
	ErrNameIsAlreadyReserved = errors.New("name is already reserved")
	ErrInvalidForceRefresh   = errors.New("forceRefreshSec must not be negative")
)

// NoneFetchState returns an empty fetch-state for offline builds.
func NoneFetchState() *model.MembershipV2FetchState { return &model.MembershipV2FetchState{} }

// IsTransientError always returns false in offline builds.
func IsTransientError(err error) bool { return false }

// Service is the offline no-op implementation.
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
	V2SubscribeToUpdates(ctx context.Context, req *pb.RpcMembershipV2SubscribeToUpdatesRequest) (*pb.RpcMembershipV2SubscribeToUpdatesResponse, error)

	app.ComponentRunnable
}

type service struct{}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) error { return nil }
func (s *service) Run(ctx context.Context) error { return nil }
func (s *service) Close(ctx context.Context) error { return nil }
func (s *service) Name() string { return CName }

func (s *service) GetSubscriptionStatus(ctx context.Context, req *pb.RpcMembershipGetStatusRequest) (*pb.RpcMembershipGetStatusResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) IsNameValid(ctx context.Context, req *pb.RpcMembershipIsNameValidRequest) (*pb.RpcMembershipIsNameValidResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) RegisterPaymentRequest(ctx context.Context, req *pb.RpcMembershipRegisterPaymentRequestRequest) (*pb.RpcMembershipRegisterPaymentRequestResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) GetPortalLink(ctx context.Context, req *pb.RpcMembershipGetPortalLinkUrlRequest) (*pb.RpcMembershipGetPortalLinkUrlResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) GetVerificationEmail(ctx context.Context, req *pb.RpcMembershipGetVerificationEmailRequest) (*pb.RpcMembershipGetVerificationEmailResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) VerifyEmailCode(ctx context.Context, req *pb.RpcMembershipVerifyEmailCodeRequest) (*pb.RpcMembershipVerifyEmailCodeResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) FinalizeSubscription(ctx context.Context, req *pb.RpcMembershipFinalizeRequest) (*pb.RpcMembershipFinalizeResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) GetTiers(ctx context.Context, req *pb.RpcMembershipGetTiersRequest) (*pb.RpcMembershipGetTiersResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) VerifyAppStoreReceipt(ctx context.Context, req *pb.RpcMembershipVerifyAppStoreReceiptRequest) (*pb.RpcMembershipVerifyAppStoreReceiptResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) CodeGetInfo(ctx context.Context, req *pb.RpcMembershipCodeGetInfoRequest) (*pb.RpcMembershipCodeGetInfoResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) CodeRedeem(ctx context.Context, req *pb.RpcMembershipCodeRedeemRequest) (*pb.RpcMembershipCodeRedeemResponse, error) {
	return nil, ErrNoConnection
}

func (s *service) V2GetPortalLink(ctx context.Context, req *pb.RpcMembershipV2GetPortalLinkRequest) (*pb.RpcMembershipV2GetPortalLinkResponse, error) {
	return nil, ErrV2NotEnabled
}

func (s *service) V2GetProducts(ctx context.Context, req *pb.RpcMembershipV2GetProductsRequest) (*pb.RpcMembershipV2GetProductsResponse, error) {
	return nil, ErrV2NotEnabled
}

func (s *service) V2GetStatus(ctx context.Context, req *pb.RpcMembershipV2GetStatusRequest) (*pb.RpcMembershipV2GetStatusResponse, error) {
	return nil, ErrV2NotEnabled
}

func (s *service) V2AnyNameIsValid(ctx context.Context, req *pb.RpcMembershipV2AnyNameIsValidRequest) (*pb.RpcMembershipV2AnyNameIsValidResponse, error) {
	return nil, ErrV2NotEnabled
}

func (s *service) V2AnyNameAllocate(ctx context.Context, req *pb.RpcMembershipV2AnyNameAllocateRequest) (*pb.RpcMembershipV2AnyNameAllocateResponse, error) {
	return nil, ErrV2NotEnabled
}

func (s *service) V2CartGet(ctx context.Context, req *pb.RpcMembershipV2CartGetRequest) (*pb.RpcMembershipV2CartGetResponse, error) {
	return nil, ErrV2NotEnabled
}

func (s *service) V2CartUpdate(ctx context.Context, req *pb.RpcMembershipV2CartUpdateRequest) (*pb.RpcMembershipV2CartUpdateResponse, error) {
	return nil, ErrV2NotEnabled
}

func (s *service) V2SubscribeToUpdates(ctx context.Context, req *pb.RpcMembershipV2SubscribeToUpdatesRequest) (*pb.RpcMembershipV2SubscribeToUpdatesResponse, error) {
	return nil, ErrV2NotEnabled
}
