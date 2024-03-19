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

	mock_ppclient "github.com/anyproto/any-sync/paymentservice/paymentserviceclient/mock"
	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/payments/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"

	"github.com/stretchr/testify/assert"
)

var ctx = context.Background()

var timeNow time.Time = time.Now().UTC()
var subsExpire time.Time = timeNow.Add(365 * 24 * time.Hour)

// truncate nseconds
var cacheExpireTime time.Time = time.Unix(int64(subsExpire.Unix()), 0)

type fixture struct {
	a            *app.App
	ctrl         *gomock.Controller
	cache        *mock_cache.MockCacheService
	ppclient     *mock_ppclient.MockAnyPpClientService
	wallet       *mock_wallet.MockWallet
	spaceService *mock_space.MockService
	account      *mock_account.MockService

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
	fx.spaceService = mock_space.NewMockService(t)
	fx.account = mock_account.NewMockService(t)

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

	fx.a.Register(fx.service).
		Register(testutil.PrepareMock(ctx, fx.a, fx.cache)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.ppclient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.wallet)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.spaceService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.account))

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

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcPaymentsSubscriptionGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.cache.EXPECT().IsCacheEnabled().Return(true).Once()

		spc := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(context.Background(), mock.AnythingOfType("string")).Once().Return(spc, nil)

		fx.account.EXPECT().PersonalSpaceID().Twice().Return("")
		fx.account.EXPECT().MyParticipantId(mock.AnythingOfType("string")).Once().Return("")

		spc.EXPECT().Do(mock.AnythingOfType("string"), mock.AnythingOfType("func(smartblock.SmartBlock) error")).Once().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcPaymentsSubscriptionGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierUnknown), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(psp.SubscriptionStatus_StatusUnknown), resp.Status)
	})

	t.Run("success if NoCache flag is passed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcPaymentsSubscriptionGetStatusResponse"), mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.cache.EXPECT().IsCacheEnabled().Return(true).Once()

		spc := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(context.Background(), mock.AnythingOfType("string")).Once().Return(spc, nil)

		fx.account.EXPECT().PersonalSpaceID().Twice().Return("")
		fx.account.EXPECT().MyParticipantId(mock.AnythingOfType("string")).Once().Return("")

		spc.EXPECT().Do(mock.AnythingOfType("string"), mock.AnythingOfType("func(smartblock.SmartBlock) error")).Once().Return(nil)

		// Call the function being tested
		req := pb.RpcPaymentsSubscriptionGetStatusRequest{
			// / >>> here:
			NoCache: true,
		}
		resp, err := fx.GetSubscriptionStatus(ctx, &req)
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierUnknown), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(psp.SubscriptionStatus_StatusUnknown), resp.Status)
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

		psgsr := pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:             int32(sr.Tier),
			Status:           pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status),
			DateStarted:      sr.DateStarted,
			DateEnds:         sr.DateEnds,
			IsAutoRenew:      sr.IsAutoRenew,
			PaymentMethod:    pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod),
			RequestedAnyName: sr.RequestedAnyName,
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.cache.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.cache.EXPECT().IsCacheEnabled().Return(true).Once()

		spc := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(context.Background(), mock.AnythingOfType("string")).Once().Return(spc, nil)

		fx.account.EXPECT().PersonalSpaceID().Twice().Return("")
		fx.account.EXPECT().MyParticipantId(mock.AnythingOfType("string")).Once().Return("")

		spc.EXPECT().Do(mock.AnythingOfType("string"), mock.AnythingOfType("func(smartblock.SmartBlock) error")).Once().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcPaymentsSubscriptionGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierExplorer), resp.Tier)
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

		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:             int32(sr.Tier),
			Status:           pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status),
			DateStarted:      sr.DateStarted,
			DateEnds:         sr.DateEnds,
			IsAutoRenew:      sr.IsAutoRenew,
			PaymentMethod:    pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod),
			RequestedAnyName: sr.RequestedAnyName,
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		// here: cache is disabled
		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheDisabled).Once()
		fx.cache.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.cache.EXPECT().IsCacheEnabled().Return(false).Once()
		fx.cache.EXPECT().CacheEnable().Return(nil).Once()

		spc := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(context.Background(), mock.AnythingOfType("string")).Once().Return(spc, nil)

		fx.account.EXPECT().PersonalSpaceID().Twice().Return("")
		fx.account.EXPECT().MyParticipantId(mock.AnythingOfType("string")).Once().Return("")

		spc.EXPECT().Do(mock.AnythingOfType("string"), mock.AnythingOfType("func(smartblock.SmartBlock) error")).Once().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcPaymentsSubscriptionGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierExplorer), resp.Tier)
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

		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "something.any",
		}

		psgsr := pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:             int32(sr.Tier),
			Status:           pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status),
			DateStarted:      sr.DateStarted,
			DateEnds:         sr.DateEnds,
			IsAutoRenew:      sr.IsAutoRenew,
			PaymentMethod:    pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod),
			RequestedAnyName: sr.RequestedAnyName,
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.cache.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return errors.New("can not write to cache!")
		}).Once()

		// Call the function being tested
		_, err := fx.GetSubscriptionStatus(ctx, &pb.RpcPaymentsSubscriptionGetStatusRequest{})
		assert.Error(t, err)

		// resp object is nil in case of error
		// assert.Equal(t, pb.RpcPaymentsSubscriptionGetStatusResponseErrorCode(pb.RpcPaymentsSubscriptionGetStatusResponseError_UNKNOWN_ERROR), resp.Error.Code)
		// assert.Equal(t, "can not write to cache!", resp.Error.Description)
	})

	t.Run("success if in cache", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		psgsr := pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:             int32(psp.SubscriptionTier_TierExplorer),
			Status:           pb.RpcPaymentsSubscriptionSubscriptionStatus(2),
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    pb.RpcPaymentsSubscriptionPaymentMethod(1),
			RequestedAnyName: "something.any",
		}

		// HERE>>>
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil).Once()

		spc := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(context.Background(), mock.AnythingOfType("string")).Once().Return(spc, nil)

		fx.account.EXPECT().PersonalSpaceID().Twice().Return("")
		fx.account.EXPECT().MyParticipantId(mock.AnythingOfType("string")).Once().Return("")

		spc.EXPECT().Do(mock.AnythingOfType("string"), mock.AnythingOfType("func(smartblock.SmartBlock) error")).Once().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcPaymentsSubscriptionGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierExplorer), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
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

		psgsr := pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:             int32(sr.Tier),
			Status:           pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status),
			DateStarted:      sr.DateStarted,
			DateEnds:         sr.DateEnds,
			IsAutoRenew:      sr.IsAutoRenew,
			PaymentMethod:    pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod),
			RequestedAnyName: sr.RequestedAnyName,
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		// here time.Now() will be passed which can be a bit different from the the cacheExpireTime
		fx.cache.EXPECT().CacheSet(&psgsr, mock.AnythingOfType("time.Time")).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.cache.EXPECT().IsCacheEnabled().Return(true).Once()

		spc := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(context.Background(), mock.AnythingOfType("string")).Once().Return(spc, nil)

		fx.account.EXPECT().PersonalSpaceID().Twice().Return("")
		fx.account.EXPECT().MyParticipantId(mock.AnythingOfType("string")).Once().Return("")

		spc.EXPECT().Do(mock.AnythingOfType("string"), mock.AnythingOfType("func(smartblock.SmartBlock) error")).Once().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcPaymentsSubscriptionGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierUnknown), resp.Tier)
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

		sr := psp.GetSubscriptionResponse{
			Tier:             int32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire5.Unix()),
			IsAutoRenew:      false,
			PaymentMethod:    psp.PaymentMethod_MethodCard,
			RequestedAnyName: "",
		}

		psgsr := pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:             int32(sr.Tier),
			Status:           pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status),
			DateStarted:      sr.DateStarted,
			DateEnds:         sr.DateEnds,
			IsAutoRenew:      sr.IsAutoRenew,
			PaymentMethod:    pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod),
			RequestedAnyName: sr.RequestedAnyName,
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheGet().Return(nil, cache.ErrCacheExpired).Once()
		fx.cache.EXPECT().CacheSet(&psgsr, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.cache.EXPECT().IsCacheEnabled().Return(true).Once()

		spc := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(context.Background(), mock.AnythingOfType("string")).Once().Return(spc, nil)

		fx.account.EXPECT().PersonalSpaceID().Twice().Return("")
		fx.account.EXPECT().MyParticipantId(mock.AnythingOfType("string")).Once().Return("")

		spc.EXPECT().Do(mock.AnythingOfType("string"), mock.AnythingOfType("func(smartblock.SmartBlock) error")).Once().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcPaymentsSubscriptionGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierExplorer), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
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
		psgsr := pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:             int32(pb.RpcPaymentsSubscription_TierExplorer),
			Status:           pb.RpcPaymentsSubscriptionSubscriptionStatus(sr.Status),
			DateStarted:      sr.DateStarted,
			DateEnds:         sr.DateEnds,
			IsAutoRenew:      sr.IsAutoRenew,
			PaymentMethod:    pb.RpcPaymentsSubscriptionPaymentMethod(sr.PaymentMethod),
			RequestedAnyName: sr.RequestedAnyName,
		}

		// this is the new state
		var psgsr2 pb.RpcPaymentsSubscriptionGetStatusResponse = psgsr
		psgsr2.Tier = int32(pb.RpcPaymentsSubscription_TierBuilder)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &sr, nil
		}).MinTimes(1)

		// return real struct and error
		fx.cache.EXPECT().CacheGet().Return(&psgsr, cache.ErrCacheDisabled).Once()
		fx.cache.EXPECT().CacheSet(&psgsr2, cacheExpireTime).RunAndReturn(func(in *pb.RpcPaymentsSubscriptionGetStatusResponse, expire time.Time) (err error) {
			return nil
		}).Once()
		fx.cache.EXPECT().IsCacheEnabled().Return(false).Once()
		// this should be called
		fx.cache.EXPECT().CacheEnable().Return(nil).Once()

		spc := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(context.Background(), mock.AnythingOfType("string")).Once().Return(spc, nil)

		fx.account.EXPECT().PersonalSpaceID().Twice().Return("")
		fx.account.EXPECT().MyParticipantId(mock.AnythingOfType("string")).Once().Return("")

		spc.EXPECT().Do(mock.AnythingOfType("string"), mock.AnythingOfType("func(smartblock.SmartBlock) error")).Once().Return(nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcPaymentsSubscriptionGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, int32(psp.SubscriptionTier_TierBuilder1Year), resp.Tier)
		assert.Equal(t, pb.RpcPaymentsSubscriptionSubscriptionStatus(2), resp.Status)
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

		// ethPrivateKey := ecdsa.PrivateKey{}
		// w.EXPECT().GetAccountEthPrivkey().Return(&ethPrivateKey)

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPaymentUrlRequest{
			RequestedTier:    int32(pb.RpcPaymentsSubscription_TierBuilder),
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

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.ppclient.EXPECT().BuySubscription(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.BuySubscriptionResponse, error) {
			var out psp.BuySubscriptionResponse
			out.PaymentUrl = "https://xxxx.com"

			return &out, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionGetPaymentUrlRequest{
			RequestedTier:    int32(pb.RpcPaymentsSubscription_TierBuilder),
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

		fx.ppclient.EXPECT().GetSubscriptionPortalLink(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetSubscriptionPortalLinkResponse, error) {
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

		fx.ppclient.EXPECT().GetSubscriptionPortalLink(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetSubscriptionPortalLinkResponse, error) {
			return &psp.GetSubscriptionPortalLinkResponse{
				PortalUrl: "https://xxxx.com",
			}, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

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

		fx.ppclient.EXPECT().GetVerificationEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetVerificationEmailResponse, error) {
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

		fx.ppclient.EXPECT().GetVerificationEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetVerificationEmailResponse, error) {
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
		fx.ppclient.EXPECT().VerifyEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyEmailResponse, error) {
			return nil, errors.New("bad error")
		}).MinTimes(1)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

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
		fx.ppclient.EXPECT().VerifyEmail(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.VerifyEmailResponse, error) {
			return &psp.VerifyEmailResponse{}, nil
		}).MinTimes(1)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90"))

		fx.cache.EXPECT().CacheClear().Return(nil).Once()

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionVerifyEmailCodeRequest{}
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
		req := &pb.RpcPaymentsSubscriptionFinalizeRequest{}

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

		fx.cache.EXPECT().CacheClear().Return(nil).Once()

		// Create a test request
		req := &pb.RpcPaymentsSubscriptionFinalizeRequest{}

		// Call the function being tested
		_, err := fx.FinalizeSubscription(ctx, req)
		assert.NoError(t, err)
	})
}

func TestGetTiers(t *testing.T) {
	t.Run("fail if pp client returned error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		req := pb.RpcPaymentsTiersGetRequest{
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

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			return &psp.GetTiersResponse{}, nil
		}).MinTimes(1)

		req := pb.RpcPaymentsTiersGetRequest{
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

		req := pb.RpcPaymentsTiersGetRequest{
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
