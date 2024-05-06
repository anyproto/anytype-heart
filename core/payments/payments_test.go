package payments

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/periodicsync/mock_periodicsync"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient/mock"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/payments/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/deletioncontroller/mock_deletioncontroller"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

var timeNow time.Time = time.Now().UTC()
var subsExpire time.Time = timeNow.Add(365 * 24 * time.Hour)

// truncate nseconds
var cacheExpireTime time.Time = time.Unix(int64(subsExpire.Unix()), 0)

type mockGlobalNamesUpdater struct{}

func (u *mockGlobalNamesUpdater) UpdateOwnGlobalName(string) {}

func (u *mockGlobalNamesUpdater) Init(*app.App) (err error) {
	return nil
}

func (u *mockGlobalNamesUpdater) Name() string {
	return ""
}

type fixture struct {
	a                        *app.App
	ctrl                     *gomock.Controller
	cache                    *mock_cache.MockCacheService
	ppclient                 *mock_ppclient.MockAnyPpClientService
	wallet                   *mock_wallet.MockWallet
	eventSender              *mock_event.MockSender
	periodicGetStatus        *mock_periodicsync.MockPeriodicSync
	identitiesUpdater        *mockGlobalNamesUpdater
	multiplayerLimitsUpdater *mock_deletioncontroller.MockDeletionController
	fileLimitsUpdater        *mock_filesync.MockFileSync
	*service
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		a:       new(app.App),
		ctrl:    gomock.NewController(t),
		service: New().(*service),
	}

	fx.cache = mock_cache.NewMockCacheService(t)
	fx.ppclient = mock_ppclient.NewMockAnyPpClientService(fx.ctrl)
	fx.wallet = mock_wallet.NewMockWallet(t)
	fx.eventSender = mock_event.NewMockSender(t)
	fx.multiplayerLimitsUpdater = mock_deletioncontroller.NewMockDeletionController(t)
	fx.fileLimitsUpdater = mock_filesync.NewMockFileSync(t)

	// init w mock
	SignKey := "psqF8Rj52Ci6gsUl5ttwBVhINTP8Yowc2hea73MeFm4Ek9AxedYSB4+r7DYCclDL4WmLggj2caNapFUmsMtn5Q=="
	decodedSignKey, err := crypto.DecodeKeyFromString(
		SignKey,
		crypto.UnmarshalEd25519PrivateKey,
		nil)

	assert.NoError(t, err)

	ak := accountdata.AccountKeys{
		PeerId:  "123",
		SignKey: decodedSignKey,
	}

	fx.wallet.EXPECT().Account().Return(&ak).Maybe()
	fx.wallet.EXPECT().GetAccountPrivkey().Return(decodedSignKey).Maybe()

	fx.eventSender.EXPECT().Broadcast(mock.AnythingOfType("*pb.Event")).Maybe()

	ctx = context.WithValue(ctx, "dontRunPeriodicGetStatus", true)

	fx.a.Register(fx.service).
		Register(testutil.PrepareMock(ctx, fx.a, fx.cache)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.ppclient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.wallet)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.eventSender)).
		Register(fx.identitiesUpdater).
		Register(testutil.PrepareMock(ctx, fx.a, fx.multiplayerLimitsUpdater)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.fileLimitsUpdater))

	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func (fx *fixture) finish(t *testing.T) {
	assert.NoError(t, fx.a.Close(ctx))
}

func TestGetStatus(t *testing.T) {
	t.Run("success if no cache and GetSubscriptionStatus returns error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})
		//fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusUnknown, resp.Data.Status)
	})

	t.Run("success if NoCache flag is passed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})
		//fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		// Call the function being tested
		req := pb.RpcMembershipGetStatusRequest{
			// / >>> here:
			NoCache: true,
		}
		resp, err := fx.GetSubscriptionStatus(ctx, &req)
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusUnknown, resp.Data.Status)
	})

	t.Run("success if cache is expired and GetSubscriptionStatus returns no error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:          uint32(sr.Tier),
				Status:        model.MembershipStatus(sr.Status),
				DateStarted:   sr.DateStarted,
				DateEnds:      sr.DateEnds,
				IsAutoRenew:   sr.IsAutoRenew,
				PaymentMethod: PaymentMethodToModel(sr.PaymentMethod),
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})
		// fx.cache.EXPECT().CacheEnable().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
		assert.Equal(t, sr.DateStarted, resp.Data.DateStarted)
		assert.Equal(t, sr.DateEnds, resp.Data.DateEnds)
		assert.Equal(t, true, resp.Data.IsAutoRenew)
		assert.Equal(t, model.Membership_MethodCrypto, resp.Data.PaymentMethod)
		assert.Equal(t, "something", resp.Data.NsName)
	})

	t.Run("success if cache is disabled and GetSubscriptionStatus returns no error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:          uint32(sr.Tier),
				Status:        model.MembershipStatus(sr.Status),
				DateStarted:   sr.DateStarted,
				DateEnds:      sr.DateEnds,
				IsAutoRenew:   sr.IsAutoRenew,
				PaymentMethod: PaymentMethodToModel(sr.PaymentMethod),
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		// here: cache is disabled
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, cache.ErrCacheDisabled)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})
		// fx.cache.EXPECT().CacheEnable().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
		assert.Equal(t, sr.DateStarted, resp.Data.DateStarted)
		assert.Equal(t, sr.DateEnds, resp.Data.DateEnds)
		assert.Equal(t, true, resp.Data.IsAutoRenew)
		assert.Equal(t, model.Membership_MethodCrypto, resp.Data.PaymentMethod)
		assert.Equal(t, "something", resp.Data.NsName)
	})

	t.Run("fail if no cache, GetSubscriptionStatus returns no error, but can not save to cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{
				Code: pb.RpcMembershipGetStatusResponseError_NULL,
			},
			Data: &model.Membership{
				Tier:          uint32(sr.Tier),
				Status:        model.MembershipStatus(sr.Status),
				DateStarted:   sr.DateStarted,
				DateEnds:      sr.DateEnds,
				IsAutoRenew:   sr.IsAutoRenew,
				PaymentMethod: PaymentMethodToModel(sr.PaymentMethod),
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return errors.New("can not write to cache!")
		})

		// Call the function being tested
		_, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.Error(t, err)

		// resp object is nil in case of error
		// assert.Equal(t, pb.RpcPaymentsSubscriptionGetStatusResponseErrorCode(pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR), resp.Error.Code)
		// assert.Equal(t, "can not write to cache!", resp.Error.Description)
	})

	t.Run("success if in cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:          uint32(psp.SubscriptionTier_TierExplorer),
				Status:        model.Membership_StatusActive,
				DateStarted:   uint64(timeNow.Unix()),
				DateEnds:      uint64(subsExpire.Unix()),
				IsAutoRenew:   true,
				PaymentMethod: model.Membership_MethodCrypto,
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}

		// HERE>>>
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})

	t.Run("if GetSubscriptionStatus returns active tier and it expires in 5 days -> cache it for 5 days", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var subsExpire5 time.Time = timeNow.Add(365 * 24 * time.Hour)

		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire5.Unix()),
			IsAutoRenew:      false,
			PaymentMethod:    psp.PaymentMethod_MethodCard,
			RequestedAnyName: "",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{
				Code: pb.RpcMembershipGetStatusResponseError_NULL,
			},
			Data: &model.Membership{
				Tier:          uint32(sr.Tier),
				Status:        model.MembershipStatus(sr.Status),
				DateStarted:   sr.DateStarted,
				DateEnds:      sr.DateEnds,
				IsAutoRenew:   sr.IsAutoRenew,
				PaymentMethod: PaymentMethodToModel(sr.PaymentMethod),
				NsName:        "",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(&psgsr, mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})

	t.Run("if cache was disabled and tier has changed -> save, but enable cache back", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var subsExpire5 time.Time = timeNow.Add(365 * 24 * time.Hour)

		// this is from PP node
		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierBuilder1Year),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire5.Unix()),
			IsAutoRenew:      false,
			PaymentMethod:    psp.PaymentMethod_MethodCard,
			RequestedAnyName: "",
		}

		// this is from DB
		psgsr := pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{
				Code: pb.RpcMembershipGetStatusResponseError_NULL,
			},
			Data: &model.Membership{
				Tier:          uint32(psp.SubscriptionTier_TierExplorer),
				Status:        model.MembershipStatus(sr.Status),
				DateStarted:   sr.DateStarted,
				DateEnds:      sr.DateEnds,
				IsAutoRenew:   sr.IsAutoRenew,
				PaymentMethod: PaymentMethodToModel(sr.PaymentMethod),
				NsName:        "",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}

		// this is the new state
		var psgsr2 pb.RpcMembershipGetStatusResponse = psgsr
		psgsr2.Data.Tier = uint32(psp.SubscriptionTier_TierBuilder1Year)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		// return real struct and error
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDisabled)
		fx.cache.EXPECT().CacheSet(&psgsr2, mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})

		// this should be called
		fx.cache.EXPECT().CacheEnable().Return(nil).Maybe()

		fx.expectLimitsUpdated()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierBuilder1Year), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})
}

func (fx *fixture) expectLimitsUpdated() {
	fx.multiplayerLimitsUpdater.EXPECT().UpdateCoordinatorStatus().Return()
	fx.fileLimitsUpdater.EXPECT().UpdateNodeUsage(mock.Anything).Return(nil)
}

func TestRegisterPaymentRequest(t *testing.T) {
	t.Run("fail if BuySubscription method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.ppclient.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		// ethPrivateKey := ecdsa.PrivateKey{}
		// w.EXPECT().GetAccountEthPrivkey().Return(&ethPrivateKey)

		// Create a test request
		req := &pb.RpcMembershipGetPaymentUrlRequest{
			RequestedTier: uint32(psp.SubscriptionTier_TierBuilder1Year),
			PaymentMethod: model.Membership_MethodCrypto,
			NsName:        "something",
			NsNameType:    model.NameserviceNameType_AnyName,
		}

		// Call the function being tested
		_, err := fx.RegisterPaymentRequest(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.ppclient.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			var out psp.BuySubscriptionResponse
			out.PaymentUrl = "https://xxxx.com"
			out.BillingID = "killbillingid"

			return &out, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

		// Create a test request
		req := &pb.RpcMembershipGetPaymentUrlRequest{
			RequestedTier: uint32(psp.SubscriptionTier_TierBuilder1Year),
			PaymentMethod: model.Membership_MethodCrypto,
			NsName:        "something",
			NsNameType:    model.NameserviceNameType_AnyName,
		}

		// Call the function being tested
		resp, err := fx.RegisterPaymentRequest(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, "https://xxxx.com", resp.PaymentUrl)
		assert.Equal(t, "killbillingid", resp.BillingId)
	})
}

func TestGetPortalURL(t *testing.T) {
	t.Run("fail if GetPortal method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionPortalLink(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetSubscriptionPortalLinkResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		// Create a test request
		req := &pb.RpcMembershipGetPortalLinkUrlRequest{}

		// Call the function being tested
		_, err := fx.GetPortalLink(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionPortalLink(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetSubscriptionPortalLinkResponse, error) {
			return &psp.GetSubscriptionPortalLinkResponse{
				PortalUrl: "https://xxxx.com",
			}, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

		// Create a test request
		req := &pb.RpcMembershipGetPortalLinkUrlRequest{}

		// Call the function being tested
		resp, err := fx.GetPortalLink(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, "https://xxxx.com", resp.PortalUrl)
	})
}

func TestGetVerificationEmail(t *testing.T) {
	t.Run("fail if GetVerificationEmail method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetVerificationEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetVerificationEmailResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		// Create a test request
		req := &pb.RpcMembershipGetVerificationEmailRequest{}
		req.Email = "some@mail.com"
		req.SubscribeToNewsletter = true

		// Call the function being tested
		_, err := fx.GetVerificationEmail(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetVerificationEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetVerificationEmailResponse, error) {
			return &psp.GetVerificationEmailResponse{}, nil
		}).MinTimes(1)

		// Create a test request
		req := &pb.RpcMembershipGetVerificationEmailRequest{}
		req.Email = "some@mail.com"
		req.SubscribeToNewsletter = true

		// Call the function being tested
		resp, err := fx.GetVerificationEmail(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, pb.RpcMembershipGetVerificationEmailResponseErrorCode(0), resp.Error.Code)
	})
}

func TestVerifyEmailCode(t *testing.T) {
	t.Run("fail if VerifyEmail method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// no errors
		fx.ppclient.EXPECT().VerifyEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyEmailResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		// Create a test request
		req := &pb.RpcMembershipVerifyEmailCodeRequest{}
		req.Code = "1234"

		// Call the function being tested
		_, err := fx.VerifyEmailCode(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// no errors
		fx.ppclient.EXPECT().VerifyEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyEmailResponse, error) {
			return &psp.VerifyEmailResponse{}, nil
		}).MinTimes(1)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.cache.EXPECT().CacheClear().Return(nil).Once()

		// Create a test request
		req := &pb.RpcMembershipVerifyEmailCodeRequest{}
		req.Code = "1234"

		// Call the function being tested
		_, err := fx.VerifyEmailCode(ctx, req)
		assert.NoError(t, err)
	})
}

func TestFinalizeSubscription(t *testing.T) {
	t.Run("fail if FinalizeSubscription method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// no errors
		fx.ppclient.EXPECT().FinalizeSubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.FinalizeSubscriptionResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90")).Once()

		// Create a test request
		req := &pb.RpcMembershipFinalizeRequest{}

		// Call the function being tested
		_, err := fx.FinalizeSubscription(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// no errors
		fx.ppclient.EXPECT().FinalizeSubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.FinalizeSubscriptionResponse, error) {
			return &psp.FinalizeSubscriptionResponse{}, nil
		}).MinTimes(1)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90")).Once()

		fx.cache.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

		// Create a test request
		req := &pb.RpcMembershipFinalizeRequest{}

		// Call the function being tested
		_, err := fx.FinalizeSubscription(ctx, req)
		assert.NoError(t, err)
	})
}

func TestGetTiers(t *testing.T) {
	t.Run("fail if no cache, pp client returned error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheExpired)

		req := pb.RpcMembershipGetTiersRequest{
			NoCache: false,
			Locale:  "en_US",
		}
		_, err := fx.GetTiers(ctx, &req)
		assert.Error(t, err)
	})

	t.Run("success if no cache, empty response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			return &psp.GetTiersResponse{}, nil
		}).MinTimes(1)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{
				Tier:             uint32(psp.SubscriptionTier_TierExplorer),
				Status:           psp.SubscriptionStatus_StatusActive,
				DateStarted:      uint64(timeNow.Unix()),
				DateEnds:         uint64(subsExpire.Unix()),
				IsAutoRenew:      true,
				PaymentMethod:    psp.PaymentMethod_MethodCrypto,
				RequestedAnyName: "something.any",
			}, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		req := pb.RpcMembershipGetTiersRequest{
			NoCache: true,
			Locale:  "en_US",
		}
		_, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
	})

	t.Run("success if no cache, response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			out := &psp.GetTiersResponse{

				Tiers: []*psp.TierData{
					{
						Id:           1,
						Name:         "Explorer",
						Description:  "Explorer tier",
						IsActive:     true,
						IsHiddenTier: false,
						// []*Feature
						Features: []*psp.Feature{
							{
								Description: "special support",
							},
							{
								Description: "storage GBs",
							},
						},
						AndroidProductId: "id_android_sub_explorer",
						AndroidManageUrl: "android_explorer_tier.url",
						IosProductId:     "Membership.Tiers.Explorer",
						IosManageUrl:     "ios_explorer_tier.url",
						StripeProductId:  "explorer_tier",
						StripeManageUrl:  "explorer_tier.com",
					},
				},
			}

			return out, nil
		}).MinTimes(1)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{
				Tier:             uint32(psp.SubscriptionTier_TierExplorer),
				Status:           psp.SubscriptionStatus_StatusActive,
				DateStarted:      uint64(timeNow.Unix()),
				DateEnds:         uint64(subsExpire.Unix()),
				IsAutoRenew:      true,
				PaymentMethod:    psp.PaymentMethod_MethodCrypto,
				RequestedAnyName: "something.any",
			}, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		req := pb.RpcMembershipGetTiersRequest{
			NoCache: false,
			Locale:  "en_US",
		}
		out, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(out.Tiers))

		assert.Equal(t, uint32(1), out.Tiers[0].Id)
		assert.Equal(t, "Explorer", out.Tiers[0].Name)
		assert.Equal(t, "Explorer tier", out.Tiers[0].Description)
		// should be converted to array
		assert.Equal(t, 2, len(out.Tiers[0].Features))
		assert.Equal(t, "special support", out.Tiers[0].Features[0])
		assert.Equal(t, "id_android_sub_explorer", out.Tiers[0].AndroidProductId)
		assert.Equal(t, "android_explorer_tier.url", out.Tiers[0].AndroidManageUrl)
		assert.Equal(t, "Membership.Tiers.Explorer", out.Tiers[0].IosProductId)
		assert.Equal(t, "ios_explorer_tier.url", out.Tiers[0].IosManageUrl)
		assert.Equal(t, "explorer_tier", out.Tiers[0].StripeProductId)
		assert.Equal(t, "explorer_tier.com", out.Tiers[0].StripeManageUrl)
	})

	t.Run("success if status is in cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:          uint32(sr.Tier),
				Status:        model.MembershipStatus(sr.Status),
				DateStarted:   sr.DateStarted,
				DateEnds:      sr.DateEnds,
				IsAutoRenew:   sr.IsAutoRenew,
				PaymentMethod: PaymentMethodToModel(sr.PaymentMethod),
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			return &psp.GetTiersResponse{

				Tiers: []*psp.TierData{
					{
						Id:           1,
						Name:         "Explorer",
						Description:  "Explorer tier",
						IsActive:     true,
						IsHiddenTier: false,
					},
				},
			}, nil
		}).MinTimes(1)

		req := pb.RpcMembershipGetTiersRequest{
			NoCache: false,
			Locale:  "en_US",
		}
		out, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(out.Tiers))

		assert.Equal(t, uint32(1), out.Tiers[0].Id)
		assert.Equal(t, "Explorer", out.Tiers[0].Name)
		assert.Equal(t, "Explorer tier", out.Tiers[0].Description)
	})

	t.Run("success if full status is in cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:          uint32(sr.Tier),
				Status:        model.MembershipStatus(sr.Status),
				DateStarted:   sr.DateStarted,
				DateEnds:      sr.DateEnds,
				IsAutoRenew:   sr.IsAutoRenew,
				PaymentMethod: PaymentMethodToModel(sr.PaymentMethod),
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}

		tgr := pb.RpcMembershipGetTiersResponse{
			Tiers: []*model.MembershipTierData{
				{
					Id:          1,
					Name:        "Explorer",
					Description: "Explorer tier",
				},
			},
		}
		fx.cache.EXPECT().CacheGet().Return(&psgsr, &tgr, nil)

		req := pb.RpcMembershipGetTiersRequest{
			NoCache: false,
			Locale:  "en_US",
		}
		out, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(out.Tiers))

		assert.Equal(t, uint32(1), out.Tiers[0].Id)
		assert.Equal(t, "Explorer", out.Tiers[0].Name)
		assert.Equal(t, "Explorer tier", out.Tiers[0].Description)
	})

	t.Run("success if full status is in cache and higher then Explorer", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierBuilder1Year),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}
		/*
			fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
				return &sr, nil
			}).MinTimes(1)
		*/

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:          uint32(sr.Tier),
				Status:        model.MembershipStatus(sr.Status),
				DateStarted:   sr.DateStarted,
				DateEnds:      sr.DateEnds,
				IsAutoRenew:   sr.IsAutoRenew,
				PaymentMethod: PaymentMethodToModel(sr.PaymentMethod),
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			},
		}

		tgr := pb.RpcMembershipGetTiersResponse{
			Tiers: []*model.MembershipTierData{
				{
					Id:          1,
					Name:        "Explorer",
					Description: "Explorer tier",
				},
				{
					Id:          2,
					Name:        "Builder",
					Description: "Builder tier",
				},
				{
					Id:          3,
					Name:        "Special",
					Description: "Special tier",
				},
			},
		}
		fx.cache.EXPECT().CacheGet().Return(&psgsr, &tgr, nil)

		req := pb.RpcMembershipGetTiersRequest{
			NoCache: false,
			Locale:  "en_US",
		}
		out, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(out.Tiers))

		assert.Equal(t, uint32(2), out.Tiers[0].Id)
		assert.Equal(t, "Builder", out.Tiers[0].Name)
		assert.Equal(t, "Builder tier", out.Tiers[0].Description)
	})

	t.Run("success if full status is in cache and higher then Explorer, no status cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierBuilder1Year),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		tgr := pb.RpcMembershipGetTiersResponse{
			Tiers: []*model.MembershipTierData{
				{
					Id:          1,
					Name:        "Explorer",
					Description: "Explorer tier",
				},
				{
					Id:          2,
					Name:        "Builder",
					Description: "Builder tier",
				},
				{
					Id:          3,
					Name:        "Special",
					Description: "Special tier",
				},
			},
		}
		fx.cache.EXPECT().CacheGet().Return(nil, &tgr, nil)
		// should call it to save status
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		req := pb.RpcMembershipGetTiersRequest{
			NoCache: false,
			Locale:  "en_US",
		}
		out, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(out.Tiers))

		assert.Equal(t, uint32(2), out.Tiers[0].Id)
		assert.Equal(t, "Builder", out.Tiers[0].Name)
		assert.Equal(t, "Builder tier", out.Tiers[0].Description)
	})
}

func TestIsNameValid(t *testing.T) {

	/*
		t.Run("success if reading from cache", func(t *testing.T) {
			fx := newFixture(t)
			defer fx.finish(t)

			tgr := pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{
					{
						Id:                    1,
						Name:                  "Explorer",
						Description:           "Explorer tier",
						AnyNamesCountIncluded: 1,
						AnyNameMinLength:      5,
					},
					{
						Id:                    2,
						Name:                  "Suppa",
						Description:           "Suppa tieren",
						AnyNamesCountIncluded: 2,
						AnyNameMinLength:      7,
					},
					{
						Id:                    3,
						Name:                  "NoNamme",
						Description:           "Nicht Suppa tieren",
						AnyNamesCountIncluded: 0,
						AnyNameMinLength:      0,
					},
				},
			}
			fx.cache.EXPECT().CacheGet().Return(nil, &tgr, nil)

			req := pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 0,
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err := fx.IsNameValid(ctx, &req)
			assert.Error(t, err)
			assert.Equal(t, (*pb.RpcMembershipIsNameValidResponse)(nil), resp)

			// 2
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 1,
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.NoError(t, err)
			assert.Equal(t, (*pb.RpcMembershipIsNameValidResponseError)(nil), resp.Error)

			// 3
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 2,
				NsName:        "somet",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.NoError(t, err)
			assert.Equal(t, pb.RpcMembershipIsNameValidResponseError_TOO_SHORT, resp.Error.Code)

			// 4
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 3,
				NsName:        "somet",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.NoError(t, err)
			assert.Equal(t, pb.RpcMembershipIsNameValidResponseError_TIER_FEATURES_NO_NAME, resp.Error.Code)

			// 5 - TIER NOT FOUND will return error immediately
			// not response
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 4,
				NsName:        "somet",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			_, err = fx.IsNameValid(ctx, &req)
			assert.Error(t, err)

			// 6 - space between
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 1,
				NsName:        "some thing",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.NoError(t, err)
			assert.Equal(t, pb.RpcMembershipIsNameValidResponseError_HAS_INVALID_CHARS, resp.Error.Code)

			// 7 - dot
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 1,
				NsName:        "some.thing",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.NoError(t, err)
			assert.Equal(t, pb.RpcMembershipIsNameValidResponseError_HAS_INVALID_CHARS, resp.Error.Code)
		})

		t.Run("success if asking directly from node", func(t *testing.T) {
			fx := newFixture(t)
			defer fx.finish(t)

			fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
				return &psp.GetTiersResponse{

					Tiers: []*psp.TierData{
						{
							Id:                    1,
							Name:                  "Explorer",
							Description:           "Explorer tier",
							IsActive:              true,
							IsHiddenTier:          false,
							AnyNamesCountIncluded: 1,
							AnyNameMinLength:      5,
						},
						{
							Id:                    2,
							Name:                  "Suppa",
							Description:           "Suppa tieren",
							IsActive:              true,
							IsHiddenTier:          false,
							AnyNamesCountIncluded: 2,
							AnyNameMinLength:      7,
						},
						{
							Id:                    3,
							Name:                  "NoNamme",
							Description:           "Nicht Suppa tieren",
							IsActive:              true,
							IsHiddenTier:          false,
							AnyNamesCountIncluded: 0,
							AnyNameMinLength:      0,
						},
					},
				}, nil
			}).MinTimes(1)

			fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheExpired)
			fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse, expire time.Time) (err error) {
				return nil
			})

			req := pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 0,
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err := fx.IsNameValid(ctx, &req)
			assert.Error(t, err)
			assert.Equal(t, (*pb.RpcMembershipIsNameValidResponse)(nil), resp)

			// 2
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 1,
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.NoError(t, err)
			assert.Equal(t, (*pb.RpcMembershipIsNameValidResponseError)(nil), resp.Error)

			// 3
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 2,
				NsName:        "some",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.NoError(t, err)
			assert.Equal(t, pb.RpcMembershipIsNameValidResponseError_TOO_SHORT, resp.Error.Code)

			// 4
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 3,
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.NoError(t, err)
			assert.Equal(t, pb.RpcMembershipIsNameValidResponseError_TIER_FEATURES_NO_NAME, resp.Error.Code)

			// 5
			req = pb.RpcMembershipIsNameValidRequest{
				RequestedTier: 4,
				NsName:        "something",
				NsNameType:    model.NameserviceNameType_AnyName,
			}
			resp, err = fx.IsNameValid(ctx, &req)
			assert.Error(t, err)
		})
	*/

	t.Run("validation error", func(t *testing.T) {
		fx := newFixture(t)

		fx.ppclient.EXPECT().IsNameValid(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.IsNameValidResponse, error) {
			return &psp.IsNameValidResponse{
				Code: psp.IsNameValidResponse_HasInvalidChars,
			}, nil
		}).MinTimes(1)

		req := pb.RpcMembershipIsNameValidRequest{
			RequestedTier: 4,
			NsName:        "something",
			NsNameType:    model.NameserviceNameType_AnyName,
		}
		resp, err := fx.IsNameValid(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, pb.RpcMembershipIsNameValidResponseError_HAS_INVALID_CHARS, resp.Error.Code)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)

		fx.ppclient.EXPECT().IsNameValid(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.IsNameValidResponse, error) {
			return &psp.IsNameValidResponse{
				Code: psp.IsNameValidResponse_Valid,
			}, nil
		}).MinTimes(1)

		req := pb.RpcMembershipIsNameValidRequest{
			RequestedTier: 4,
			NsName:        "something",
			NsNameType:    model.NameserviceNameType_AnyName,
		}
		resp, err := fx.IsNameValid(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, pb.RpcMembershipIsNameValidResponseErrorCode(0), resp.Error.Code)
	})
}

func TestVerifyAppStoreReceipt(t *testing.T) {
	t.Run("fail if VerifyAppStoreReceipt fails", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.ppclient.EXPECT().VerifyAppStoreReceipt(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyAppStoreReceiptResponse, error) {
			return nil, psp.ErrUnknown
		}).MinTimes(1)

		req := &pb.RpcMembershipVerifyAppStoreReceiptRequest{
			BillingId: "billingID",
			Receipt:   "sjakflkajsfh.kajsflksadjflas.oicpvoxvpovi",
		}

		// when
		resp, err := fx.VerifyAppStoreReceipt(ctx, req)

		// then
		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("success if VerifyAppStoreReceipt successes", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.ppclient.EXPECT().VerifyAppStoreReceipt(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyAppStoreReceiptResponse, error) {
			return &psp.VerifyAppStoreReceiptResponse{}, nil
		}).MinTimes(1)

		req := &pb.RpcMembershipVerifyAppStoreReceiptRequest{
			BillingId: "billingID",
			Receipt:   "sjakflkajsfh.kajsflksadjflas.oicpvoxvpovi",
		}

		// when
		resp, err := fx.VerifyAppStoreReceipt(ctx, req)

		// then
		assert.NoError(t, err)
		assert.Equal(t, pb.RpcMembershipVerifyAppStoreReceiptResponseErrorCode(0), resp.Error.Code)
	})
}
