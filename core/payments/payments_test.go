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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient/mock"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/payments/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"

	"github.com/stretchr/testify/assert"
)

var ctx = context.Background()

var timeNow time.Time = time.Now().UTC()
var subsExpire time.Time = timeNow.Add(365 * 24 * time.Hour)

// truncate nseconds
var cacheExpireTime time.Time = time.Unix(int64(subsExpire.Unix()), 0)

type fixture struct {
	a           *app.App
	ctrl        *gomock.Controller
	cache       *mock_cache.MockCacheService
	ppclient    *mock_ppclient.MockAnyPpClientService
	wallet      *mock_wallet.MockWallet
	eventSender *mock_event.MockSender

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
	fx.periodicGetStatus = mock_periodicsync.NewMockPeriodicSync(fx.ctrl)

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

	fx.wallet.EXPECT().Account().Return(&ak)
	fx.wallet.EXPECT().GetAccountPrivkey().Return(decodedSignKey)

	fx.eventSender.EXPECT().Broadcast(mock.AnythingOfType("*pb.Event")).Maybe()

	fx.a.Register(fx.service).
		Register(testutil.PrepareMock(ctx, fx.a, fx.cache)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.ppclient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.wallet)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.eventSender))

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

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(true)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
		assert.Equal(t, model.MembershipStatus(psp.SubscriptionStatus_StatusUnknown), resp.Data.Status)
	})

	t.Run("success if NoCache flag is passed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(true)

		// Call the function being tested
		req := pb.RpcMembershipGetStatusRequest{
			/// >>> here:
			NoCache: true,
		}
		resp, err := fx.GetSubscriptionStatus(ctx, &req)
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
		assert.Equal(t, model.MembershipStatus(psp.SubscriptionStatus_StatusUnknown), resp.Data.Status)
	})

	t.Run("success if cache is expired and GetSubscriptionStatus returns no error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:             int32(sr.Tier),
				Status:           model.MembershipStatus(sr.Status),
				DateStarted:      sr.DateStarted,
				DateEnds:         sr.DateEnds,
				IsAutoRenew:      sr.IsAutoRenew,
				PaymentMethod:    model.MembershipPaymentMethod(sr.PaymentMethod),
				RequestedAnyName: sr.RequestedAnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(true)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.MembershipStatus(2), resp.Data.Status)
		assert.Equal(t, sr.DateStarted, resp.Data.DateStarted)
		assert.Equal(t, sr.DateEnds, resp.Data.DateEnds)
		assert.Equal(t, true, resp.Data.IsAutoRenew)
		assert.Equal(t, model.MembershipPaymentMethod(1), resp.Data.PaymentMethod)
		assert.Equal(t, "something.any", resp.Data.RequestedAnyName)
	})

	t.Run("success if cache is disabled and GetSubscriptionStatus returns no error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:             int32(sr.Tier),
				Status:           model.MembershipStatus(sr.Status),
				DateStarted:      sr.DateStarted,
				DateEnds:         sr.DateEnds,
				IsAutoRenew:      sr.IsAutoRenew,
				PaymentMethod:    model.MembershipPaymentMethod(sr.PaymentMethod),
				RequestedAnyName: sr.RequestedAnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		// here: cache is disabled
		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheDisabled)
		fx.cache.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.MembershipStatus(2), resp.Data.Status)
		assert.Equal(t, sr.DateStarted, resp.Data.DateStarted)
		assert.Equal(t, sr.DateEnds, resp.Data.DateEnds)
		assert.Equal(t, true, resp.Data.IsAutoRenew)
		assert.Equal(t, model.MembershipPaymentMethod(1), resp.Data.PaymentMethod)
		assert.Equal(t, "something.any", resp.Data.RequestedAnyName)
	})

	t.Run("fail if no cache, GetSubscriptionStatus returns no error, but can not save to cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:             int32(sr.Tier),
				Status:           model.MembershipStatus(sr.Status),
				DateStarted:      sr.DateStarted,
				DateEnds:         sr.DateEnds,
				IsAutoRenew:      sr.IsAutoRenew,
				PaymentMethod:    model.MembershipPaymentMethod(sr.PaymentMethod),
				RequestedAnyName: sr.RequestedAnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return errors.New("can not write to cache!")
		})

		// Call the function being tested
		_, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.Error(t, err)

		// resp object is nil in case of error
		//assert.Equal(t, pb.RpcPaymentsSubscriptionGetStatusResponseErrorCode(pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR), resp.Error.Code)
		//assert.Equal(t, "can not write to cache!", resp.Error.Description)
	})

	t.Run("success if in cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:             int32(psp.SubscriptionTier_TierExplorer),
				Status:           model.MembershipStatus(2),
				DateStarted:      uint64(timeNow.Unix()),
				DateEnds:         uint64(subsExpire.Unix()),
				IsAutoRenew:      true,
				PaymentMethod:    model.MembershipPaymentMethod(1),
				RequestedAnyName: "something.any",
			},
		}

		// HERE>>>
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.MembershipStatus(2), resp.Data.Status)
	})

	t.Run("if GetSubscriptionStatus returns 0 tier -> cache it for 10 days", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierUnknown),
			Status:           psp.SubscriptionStatus_StatusUnknown,
			DateStarted:      0,
			DateEnds:         0,
			IsAutoRenew:      false,
			PaymentMethod:    psp.PaymentMethod_MethodCard,
			RequestedAnyName: "",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:             int32(sr.Tier),
				Status:           model.MembershipStatus(sr.Status),
				DateStarted:      sr.DateStarted,
				DateEnds:         sr.DateEnds,
				IsAutoRenew:      sr.IsAutoRenew,
				PaymentMethod:    model.MembershipPaymentMethod(sr.PaymentMethod),
				RequestedAnyName: sr.RequestedAnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired)
		// here time.Now() will be passed which can be a bit different from the the cacheExpireTime
		fx.cache.EXPECT().CacheSet(&psgsr, mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(true)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
		assert.Equal(t, model.MembershipStatus(0), resp.Data.Status)
		assert.Equal(t, uint64(0), resp.Data.DateStarted)
		assert.Equal(t, uint64(0), resp.Data.DateEnds)
		assert.Equal(t, false, resp.Data.IsAutoRenew)
		assert.Equal(t, model.MembershipPaymentMethod(0), resp.Data.PaymentMethod)
		assert.Equal(t, "", resp.Data.RequestedAnyName)
	})

	t.Run("if GetSubscriptionStatus returns active tier and it expires in 5 days -> cache it for 5 days", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var subsExpire5 time.Time = timeNow.Add(365 * 24 * time.Hour)

		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire5.Unix()),
			IsAutoRenew:      false,
			PaymentMethod:    psp.PaymentMethod_MethodCard,
			RequestedAnyName: "",
		}

		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:             int32(sr.Tier),
				Status:           model.MembershipStatus(sr.Status),
				DateStarted:      sr.DateStarted,
				DateEnds:         sr.DateEnds,
				IsAutoRenew:      sr.IsAutoRenew,
				PaymentMethod:    model.MembershipPaymentMethod(sr.PaymentMethod),
				RequestedAnyName: sr.RequestedAnyName,
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired)
		fx.cache.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(true)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.MembershipStatus(2), resp.Data.Status)
	})

	t.Run("if cache was disabled and tier has changed -> save, but enable cache back", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var subsExpire5 time.Time = timeNow.Add(365 * 24 * time.Hour)

		// this is from PP node
		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierBuilder1Year),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire5.Unix()),
			IsAutoRenew:      false,
			PaymentMethod:    psp.PaymentMethod_MethodCard,
			RequestedAnyName: "",
		}

		// this is from DB
		psgsr := pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:             int32(model.Membership_TierExplorer),
				Status:           model.MembershipStatus(sr.Status),
				DateStarted:      sr.DateStarted,
				DateEnds:         sr.DateEnds,
				IsAutoRenew:      sr.IsAutoRenew,
				PaymentMethod:    model.MembershipPaymentMethod(sr.PaymentMethod),
				RequestedAnyName: sr.RequestedAnyName,
			},
		}

		// this is the new state
		var psgsr2 pb.RpcMembershipGetStatusResponse = psgsr
		psgsr2.Data.Tier = int32(model.Membership_TierBuilder)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		// return real struct and error
		fx.cache.EXPECT().CacheGet().Return(&psgsr, cache.ErrCacheDisabled)
		fx.cache.EXPECT().CacheSet(&psgsr2, cacheExpireTime).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		// this should be called
		fx.cache.EXPECT().CacheEnable().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierBuilder1Year), resp.Data.Tier)
		assert.Equal(t, model.MembershipStatus(2), resp.Data.Status)
	})
}

func TestGetPaymentURL(t *testing.T) {
	t.Run("fail if BuySubscription method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.ppclient.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired)
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		//ethPrivateKey := ecdsa.PrivateKey{}
		//w.EXPECT().GetAccountEthPrivkey().Return(&ethPrivateKey)

		// Create a test request
		req := &pb.RpcMembershipGetPaymentUrlRequest{
			RequestedTier:    int32(model.Membership_TierBuilder),
			PaymentMethod:    model.Membership_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		// Call the function being tested
		_, err := fx.GetPaymentURL(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.ppclient.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			var out psp.BuySubscriptionResponse
			out.PaymentUrl = "https://xxxx.com"

			return &out, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		// Create a test request
		req := &pb.RpcMembershipGetPaymentUrlRequest{
			RequestedTier:    int32(model.Membership_TierBuilder),
			PaymentMethod:    model.Membership_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		// Call the function being tested
		resp, err := fx.GetPaymentURL(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, "https://xxxx.com", resp.PaymentUrl)
	})
}

func TestGetPortalURL(t *testing.T) {
	t.Run("fail if GetPortal method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

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

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

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

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		//
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

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		//
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
		assert.True(t, resp.Error == nil)
	})
}

func TestVerifyEmailCode(t *testing.T) {
	t.Run("fail if VerifyEmail method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		//
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

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		//
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

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

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

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		// no errors
		fx.ppclient.EXPECT().FinalizeSubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.FinalizeSubscriptionResponse, error) {
			return &psp.FinalizeSubscriptionResponse{}, nil
		}).MinTimes(1)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90")).Once()

		fx.cache.EXPECT().CacheClear().Return(nil).Once()

		// Create a test request
		req := &pb.RpcMembershipFinalizeRequest{}

		// Call the function being tested
		_, err := fx.FinalizeSubscription(ctx, req)
		assert.NoError(t, err)
	})
}

func TestGetTiers(t *testing.T) {
	t.Run("fail if pp client returned error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		req := pb.RpcMembershipTiersGetRequest{
			NoCache:       true,
			Locale:        "EN_us",
			PaymentMethod: 0,
		}
		_, err := fx.GetTiers(ctx, &req)
		assert.Error(t, err)
	})

	t.Run("success if empty response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			return &psp.GetTiersResponse{}, nil
		}).MinTimes(1)

		req := pb.RpcMembershipTiersGetRequest{
			NoCache:       true,
			Locale:        "EN_us",
			PaymentMethod: 0,
		}
		_, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
	})

	t.Run("success if response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Maybe()
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{}, nil
		})
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, expire time.Time) (err error) {
			return nil
		})
		fx.cache.EXPECT().IsCacheEnabled().Return(false)
		fx.cache.EXPECT().CacheEnable().Return(nil)

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

		req := pb.RpcMembershipTiersGetRequest{
			NoCache:       true,
			Locale:        "EN_us",
			PaymentMethod: 0,
		}
		out, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(out.Tiers))

		assert.Equal(t, uint32(1), out.Tiers[0].Id)
		assert.Equal(t, "Explorer", out.Tiers[0].Name)
		assert.Equal(t, "Explorer tier", out.Tiers[0].Description)
		assert.Equal(t, true, out.Tiers[0].IsActive)
		assert.Equal(t, false, out.Tiers[0].IsHiddenTier)
	})
}
