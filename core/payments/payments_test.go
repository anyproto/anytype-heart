package payments

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/any-sync/paymentservice/paymentserviceclient"
	mock_ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient/mock"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/payments/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"

	"github.com/tj/assert"
)

var ctx = context.Background()

var timeNow time.Time = time.Now().UTC()
var subsExpire time.Time = timeNow.Add(365 * 24 * time.Hour)

// truncate nseconds
var cacheExpireTime time.Time = time.Unix(int64(subsExpire.Unix()), 0)

type fixture struct {
	a    *app.App
	ctrl *gomock.Controller
	c    *mock_cache.MockCacheService
	pp   *mock_ppclient.MockAnyPpClientService
	w    *mock_wallet.MockWallet

	*service
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		a:       new(app.App),
		ctrl:    gomock.NewController(t),
		service: New().(*service),
	}

	fx.c = mock_cache.NewMockCacheService(t)
	fx.pp = mock_ppclient.NewMockAnyPpClientService(fx.ctrl)
	fx.w = mock_wallet.NewMockWallet(t)

	fx.pp.EXPECT().Name().Return(paymentserviceclient.CName).AnyTimes()
	fx.pp.EXPECT().Init(gomock.Any()).Return(nil).AnyTimes()

	fx.c.EXPECT().Name().Return(cache.CName)
	fx.c.EXPECT().Init(fx.a).Return(nil)

	fx.w.EXPECT().Name().Return(wallet.CName)
	fx.w.EXPECT().Init(fx.a).Return(nil)

	// init w mock
	SignKey := "psqF8Rj52Ci6gsUl5ttwBVhINTP8Yowc2hea73MeFm4Ek9AxedYSB4+r7DYCclDL4WmLggj2caNapFUmsMtn5Q=="
	decodedSignKey, err := crypto.DecodeKeyFromString(
		SignKey,
		crypto.UnmarshalEd25519PrivateKey,
		nil)

	assert.NoError(t, err)

	var ak accountdata.AccountKeys
	ak.PeerId = "123"
	ak.SignKey = decodedSignKey

	fx.w.EXPECT().Account().Return(&ak)
	fx.w.EXPECT().GetAccountPrivkey().Return(decodedSignKey)

	fx.a.Register(fx.service).
		Register(fx.c).
		Register(fx.pp).
		Register(fx.w)

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

		fx.pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.c.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.c.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcPaymentsSubscriptionGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.c.EXPECT().IsCacheEnabled().Return(true).Once()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx)
		assert.NoError(t, err)

		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierUnknown), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(psp.SubscriptionStatus_StatusUnknown), resp.Status)
	})

	t.Run("success if cache is expired and GetSubscriptionStatus returns no error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var sr psp.GetSubscriptionResponse
		sr.Tier = psp.SubscriptionTier_TierExplorer
		sr.Status = psp.SubscriptionStatus_StatusActive
		sr.DateStarted = uint64(timeNow.Unix())
		sr.DateEnds = uint64(subsExpire.Unix())
		sr.IsAutoRenew = true
		sr.PaymentMethod = psp.PaymentMethod_MethodCrypto
		sr.RequestedAnyName = "something.any"

		var psgsr pb.RpcPaymentsSubscriptionGetStatusResponse
		psgsr.Tier = pb.RpcPaymentsSubscriptionSubscriptionTier(sr.Tier)
		psgsr.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status)
		psgsr.DateStarted = sr.DateStarted
		psgsr.DateEnds = sr.DateEnds
		psgsr.IsAutoRenew = sr.IsAutoRenew
		psgsr.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod)
		psgsr.RequestedAnyName = sr.RequestedAnyName

		fx.pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.c.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.c.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.c.EXPECT().IsCacheEnabled().Return(true).Once()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx)
		assert.NoError(t, err)

		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierExplorer), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
		assert.Equal(t, sr.DateStarted, resp.DateStarted)
		assert.Equal(t, sr.DateEnds, resp.DateEnds)
		assert.Equal(t, true, resp.IsAutoRenew)
		assert.Equal(t, pb.RpcPaymentsSubscriptionPaymentMethod(1), resp.PaymentMethod)
		assert.Equal(t, "something.any", resp.RequestedAnyName)
	})

	t.Run("success if cache is disabled and GetSubscriptionStatus returns no error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var sr psp.GetSubscriptionResponse
		sr.Tier = psp.SubscriptionTier_TierExplorer
		sr.Status = psp.SubscriptionStatus_StatusActive
		sr.DateStarted = uint64(timeNow.Unix())
		sr.DateEnds = uint64(subsExpire.Unix())
		sr.IsAutoRenew = true
		sr.PaymentMethod = psp.PaymentMethod_MethodCrypto
		sr.RequestedAnyName = "something.any"

		var psgsr pb.RpcPaymentsSubscriptionGetStatusResponse
		psgsr.Tier = pb.RpcPaymentsSubscriptionSubscriptionTier(sr.Tier)
		psgsr.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status)
		psgsr.DateStarted = sr.DateStarted
		psgsr.DateEnds = sr.DateEnds
		psgsr.IsAutoRenew = sr.IsAutoRenew
		psgsr.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod)
		psgsr.RequestedAnyName = sr.RequestedAnyName

		fx.pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		// here: cache is disabled
		fx.c.EXPECT().CacheGet().Return(nil, cache.ErrCacheDisabled).Once()
		fx.c.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.c.EXPECT().IsCacheEnabled().Return(false).Once()
		fx.c.EXPECT().CacheEnable().Return(nil).Once()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx)
		assert.NoError(t, err)

		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierExplorer), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
		assert.Equal(t, sr.DateStarted, resp.DateStarted)
		assert.Equal(t, sr.DateEnds, resp.DateEnds)
		assert.Equal(t, true, resp.IsAutoRenew)
		assert.Equal(t, pb.RpcPaymentsSubscriptionPaymentMethod(1), resp.PaymentMethod)
		assert.Equal(t, "something.any", resp.RequestedAnyName)
	})

	t.Run("fail if no cache, GetSubscriptionStatus returns no error, but can not save to cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var sr psp.GetSubscriptionResponse
		sr.Tier = psp.SubscriptionTier_TierExplorer
		sr.Status = psp.SubscriptionStatus_StatusActive
		sr.DateStarted = uint64(timeNow.Unix())
		sr.DateEnds = uint64(subsExpire.Unix())
		sr.IsAutoRenew = true
		sr.PaymentMethod = psp.PaymentMethod_MethodCrypto
		sr.RequestedAnyName = "something.any"

		var psgsr pb.RpcPaymentsSubscriptionGetStatusResponse
		psgsr.Tier = pb.RpcPaymentsSubscriptionSubscriptionTier(sr.Tier)
		psgsr.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status)
		psgsr.DateStarted = sr.DateStarted
		psgsr.DateEnds = sr.DateEnds
		psgsr.IsAutoRenew = sr.IsAutoRenew
		psgsr.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod)
		psgsr.RequestedAnyName = sr.RequestedAnyName

		fx.pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.c.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.c.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return errors.New("can not write to cache!")
		}).Once()

		// Call the function being tested
		_, err := fx.GetSubscriptionStatus(ctx)
		assert.Error(t, err)

		// resp object is nil in case of error
		//assert.Equal(t, pb.RpcPaymentsSubscriptionGetStatusResponseErrorCode(pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR), resp.Error.Code)
		//assert.Equal(t, "can not write to cache!", resp.Error.Description)
	})

	t.Run("success if in cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var psgsr pb.RpcPaymentsSubscriptionGetStatusResponse
		psgsr.Tier = pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierExplorer)
		psgsr.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(2)
		psgsr.DateStarted = uint64(timeNow.Unix())
		psgsr.DateEnds = uint64(subsExpire.Unix())
		psgsr.IsAutoRenew = true
		psgsr.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(1)
		psgsr.RequestedAnyName = "something.any"

		// HERE>>>
		fx.c.EXPECT().CacheGet().Return(&psgsr, nil).Once()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx)
		assert.NoError(t, err)

		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierExplorer), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
	})

	t.Run("if GetSubscriptionStatus returns 0 tier -> cache it for 10 days", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var sr psp.GetSubscriptionResponse
		sr.Tier = psp.SubscriptionTier_TierUnknown
		sr.Status = psp.SubscriptionStatus_StatusUnknown
		sr.DateStarted = 0
		sr.DateEnds = 0
		sr.IsAutoRenew = false
		sr.PaymentMethod = psp.PaymentMethod_MethodCard
		sr.RequestedAnyName = ""

		var psgsr pb.RpcPaymentsSubscriptionGetStatusResponse
		psgsr.Tier = pb.RpcPaymentsSubscriptionSubscriptionTier(sr.Tier)
		psgsr.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status)
		psgsr.DateStarted = sr.DateStarted
		psgsr.DateEnds = sr.DateEnds
		psgsr.IsAutoRenew = sr.IsAutoRenew
		psgsr.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod)
		psgsr.RequestedAnyName = sr.RequestedAnyName

		fx.pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.c.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		// here time.Now() will be passed which can be a bit different from the the cacheExpireTime
		fx.c.EXPECT().CacheSet(&psgsr, mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.c.EXPECT().IsCacheEnabled().Return(true).Once()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx)
		assert.NoError(t, err)

		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierUnknown), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(0), resp.Status)
		assert.Equal(t, uint64(0), resp.DateStarted)
		assert.Equal(t, uint64(0), resp.DateEnds)
		assert.Equal(t, false, resp.IsAutoRenew)
		assert.Equal(t, pb.RpcPaymentsSubscriptionPaymentMethod(0), resp.PaymentMethod)
		assert.Equal(t, "", resp.RequestedAnyName)
	})

	t.Run("if GetSubscriptionStatus returns active tier and it expires in 5 days -> cache it for 5 days", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var subsExpire5 time.Time = timeNow.Add(365 * 24 * time.Hour)

		var sr psp.GetSubscriptionResponse
		sr.Tier = psp.SubscriptionTier_TierExplorer
		sr.Status = psp.SubscriptionStatus_StatusActive
		sr.DateStarted = uint64(timeNow.Unix())
		sr.DateEnds = uint64(subsExpire5.Unix())
		sr.IsAutoRenew = false
		sr.PaymentMethod = psp.PaymentMethod_MethodCard
		sr.RequestedAnyName = ""

		var psgsr pb.RpcPaymentsSubscriptionGetStatusResponse
		psgsr.Tier = pb.RpcPaymentsSubscriptionSubscriptionTier(sr.Tier)
		psgsr.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status)
		psgsr.DateStarted = sr.DateStarted
		psgsr.DateEnds = sr.DateEnds
		psgsr.IsAutoRenew = sr.IsAutoRenew
		psgsr.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod)
		psgsr.RequestedAnyName = sr.RequestedAnyName

		fx.pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.c.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.c.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.c.EXPECT().IsCacheEnabled().Return(true).Once()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx)
		assert.NoError(t, err)

		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierExplorer), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
	})

	t.Run("if cache was disabled and tier has changed -> save, but enable cache back", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var subsExpire5 time.Time = timeNow.Add(365 * 24 * time.Hour)

		// this is from PP node
		var sr psp.GetSubscriptionResponse
		// tier is different!!!
		sr.Tier = psp.SubscriptionTier_TierBuilder1Year
		sr.Status = psp.SubscriptionStatus_StatusActive
		sr.DateStarted = uint64(timeNow.Unix())
		sr.DateEnds = uint64(subsExpire5.Unix())
		sr.IsAutoRenew = false
		sr.PaymentMethod = psp.PaymentMethod_MethodCard
		sr.RequestedAnyName = ""

		// this is from DB
		var psgsr pb.RpcPaymentsSubscriptionGetStatusResponse
		psgsr.Tier = pb.RpcPaymentsSubscription_TierExplorer
		psgsr.Status = pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status)
		psgsr.DateStarted = sr.DateStarted
		psgsr.DateEnds = sr.DateEnds
		psgsr.IsAutoRenew = sr.IsAutoRenew
		psgsr.PaymentMethod = pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod)
		psgsr.RequestedAnyName = sr.RequestedAnyName

		// this is the new state
		var psgsr2 pb.RpcPaymentsSubscriptionGetStatusResponse = psgsr
		psgsr2.Tier = pb.RpcPaymentsSubscription_TierBuilder1Year

		fx.pp.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		// return real struct and error
		fx.c.EXPECT().CacheGet().Return(&psgsr, cache.ErrCacheDisabled).Once()
		fx.c.EXPECT().CacheSet(&psgsr2, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.c.EXPECT().IsCacheEnabled().Return(false).Once()
		// this should be called
		fx.c.EXPECT().CacheEnable().Return(nil).Once()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx)
		assert.NoError(t, err)

		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionTier(psp.SubscriptionTier_TierBuilder1Year), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
	})
}

func TestGetPaymentURL(t *testing.T) {
	t.Run("fail if BuySubscription method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.w.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.pp.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		//ethPrivateKey := ecdsa.PrivateKey{}
		//w.EXPECT().GetAccountEthPrivkey().Return(&ethPrivateKey)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPaymentUrlRequest{
			RequestedTier:    pb.RpcPaymentsSubscription_TierBuilder1Year,
			PaymentMethod:    pb.RpcPaymentsSubscription_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		// Call the function being tested
		_, err := fx.GetPaymentURL(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.w.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.pp.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			var out psp.BuySubscriptionResponse
			out.PaymentUrl = "https://xxxx.com"

			return &out, nil
		}).MinTimes(1)

		fx.c.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPaymentUrlRequest{
			RequestedTier:    pb.RpcPaymentsSubscription_TierBuilder1Year,
			PaymentMethod:    pb.RpcPaymentsSubscription_MethodCrypto,
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

		fx.pp.EXPECT().GetSubscriptionPortalLink(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetSubscriptionPortalLinkResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest{}

		// Call the function being tested
		_, err := fx.GetPortalLink(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.pp.EXPECT().GetSubscriptionPortalLink(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetSubscriptionPortalLinkResponse, error) {
			return &psp.GetSubscriptionPortalLinkResponse{
				PortalUrl: "https://xxxx.com",
			}, nil
		}).MinTimes(1)

		fx.c.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPortalLinkUrlRequest{}

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

		fx.pp.EXPECT().GetVerificationEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetVerificationEmailResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetVerificationEmailRequest{}
		req.Email = "some@mail.com"
		req.SubscribeToNewsletter = true

		// Call the function being tested
		_, err := fx.GetVerificationEmail(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.pp.EXPECT().GetVerificationEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetVerificationEmailResponse, error) {
			return &psp.GetVerificationEmailResponse{}, nil
		}).MinTimes(1)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetVerificationEmailRequest{}
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

		// no errors
		fx.pp.EXPECT().VerifyEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyEmailResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		fx.w.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest{}
		req.Code = "1234"

		// Call the function being tested
		_, err := fx.VerifyEmailCode(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// no errors
		fx.pp.EXPECT().VerifyEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyEmailResponse, error) {
			return &psp.VerifyEmailResponse{}, nil
		}).MinTimes(1)

		fx.w.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.c.EXPECT().CacheClear().Return(nil).Once()

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest{}
		req.Code = "1234"

		// Call the function being tested
		_, err := fx.VerifyEmailCode(ctx, req)
		assert.NoError(t, err)
	})
}
