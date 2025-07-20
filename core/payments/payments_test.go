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
	mock_emailcollector "github.com/anyproto/anytype-heart/core/payments/emailcollector/mock_emailcollector"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/core/nameservice/mock_nameservice"
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
	ns                       *mock_nameservice.MockService
	emailCollector           *mock_emailcollector.MockEmailCollector

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
	fx.ns = mock_nameservice.NewMockService(t)
	fx.emailCollector = mock_emailcollector.NewMockEmailCollector(t)

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
	fx.wallet.EXPECT().RepoPath().Return(t.TempDir())

	fx.eventSender.EXPECT().Broadcast(mock.AnythingOfType("*pb.Event")).Maybe()

	ctx = context.WithValue(ctx, "dontRunPeriodicGetStatus", true)

	fx.a.Register(fx.service).
		Register(testutil.PrepareMock(ctx, fx.a, fx.cache)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.ppclient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.wallet)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.emailCollector)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.eventSender)).
		Register(fx.identitiesUpdater).
		Register(testutil.PrepareMock(ctx, fx.a, fx.multiplayerLimitsUpdater)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.fileLimitsUpdater)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.ns)).
		Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})

	require.NoError(t, fx.a.Start(ctx))

	return fx
}

func (fx *fixture) finish(t *testing.T) {
	assert.NoError(t, fx.a.Close(ctx))
}

func TestGetStatus(t *testing.T) {
	t.Run("return default if no cache and GetSubscriptionStatus returns error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return nil
		})

		// changing from NO CACHE -> default "Unknown" tier
		fx.expectLimitsUpdated()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusUnknown, resp.Data.Status)
	})

	t.Run("return default if no cache and GetSubscriptionStatus returns error, NoCache is passed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return nil
		})

		// changing from NO CACHE -> default "Unknown" tier
		fx.expectLimitsUpdated()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{
			// / >>> here:
			NoCache: true,
		})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusUnknown, resp.Data.Status)
	})

	t.Run("return prev values if ErrCacheExpired and GetSubscriptionStatus returns error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

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

		fx.cache.EXPECT().IsCacheExpired().Return(true)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})

	t.Run("return prev values if ErrCacheExpired, GetSubscriptionStatus returns error, and if NoCache flag is passed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

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

		// in case of cache.ErrCacheExpired this should always return objects
		fx.cache.EXPECT().IsCacheExpired().Return(true)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)

		// Call the function being tested
		req := pb.RpcMembershipGetStatusRequest{
			// / >>> here:
			NoCache: true,
		}
		resp, err := fx.GetSubscriptionStatus(ctx, &req)
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})

	t.Run("success if NoCache flag is passed, but no connectivity", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			// >>> here
			return nil, ErrNoConnection
		}).MinTimes(1)

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
		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)

		// Call the function being tested
		req := pb.RpcMembershipGetStatusRequest{
			// / >>> here:
			NoCache: true,
		}
		resp, err := fx.GetSubscriptionStatus(ctx, &req)
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})

	t.Run("return from cache, if cache expired and GetSubscriptionStatus returns error, and default tiers", func(t *testing.T) {
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

		tgr := pb.RpcMembershipGetTiersResponse{
			Tiers: []*model.MembershipTierData{
				{
					Id:          1,
					Name:        "Explorer",
					Description: "Explorer tier",
				},
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("no internet")
		}).MinTimes(1)

		fx.cache.EXPECT().IsCacheExpired().Return(true)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, &tgr, nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})

	t.Run("success if no cache, GetSubscriptionStatus returns error and data", func(t *testing.T) {
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

		tgr := pb.RpcMembershipGetTiersResponse{
			Tiers: []*model.MembershipTierData{
				{
					// see here
					Id:          2,
					Name:        "TIER2",
					Description: "TIER2 tier",
				},
			},
		}

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return nil, errors.New("no internet")
		}).MinTimes(1)

		fx.cache.EXPECT().IsCacheExpired().Return(true)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, &tgr, nil)

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
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
				// >>> here: different tier returned by cache!
				Tier:          uint32(psp.SubscriptionTier_TierBuilder1WeekTEST),
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

		fx.cache.EXPECT().IsCacheExpired().Return(true)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return nil
		})
		// this should not be called because server returned Explorer tier
		// fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		// the tier should be as returned by GetSubscriptionStatus, not from cache
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
				// same tier returned by cache here
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

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(true)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return nil
		})

		// tier was not changed
		// fx.expectLimitsUpdated()

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

	t.Run("success if cache was disabled and GetSubscriptionStatus returns error", func(t *testing.T) {
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
				// same tier returned by cache here
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
			return nil, errors.New("no internet")
		}).MinTimes(1)

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(true)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)

		// tier was not changed
		// fx.expectLimitsUpdated()

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

	t.Run("success if cache was expired and GetSubscriptionStatus returns error", func(t *testing.T) {
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
				// same tier returned by cache here
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
			return nil, errors.New("no internet")
		}).MinTimes(1)

		fx.cache.EXPECT().IsCacheExpired().Return(true)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)

		// tier was not changed
		// fx.expectLimitsUpdated()

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

	t.Run("do not fail if no cache, GetSubscriptionStatus returns no error, but can not save to cache", func(t *testing.T) {
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

		fx.cache.EXPECT().IsCacheExpired().Return(true)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return errors.New("can not write to cache!")
		})
		// this should not be called because server returned Explorer tier
		// fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		// Call the function being tested
		_, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

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

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
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

		fx.cache.EXPECT().IsCacheExpired().Return(true)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, nil)
		fx.cache.EXPECT().CacheSet(&psgsr, mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return nil
		})

		// because cache was expired before!
		fx.expectLimitsUpdated()

		// Call the function being tested
		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)

		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})

	t.Run("if cache was disabled and tier has changed -> save, and enable cache back", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		var subsExpire5 time.Time = timeNow.Add(365 * 24 * time.Hour)

		// this is from PP node (new status)
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

		psgsr2 := pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{
				Code: pb.RpcMembershipGetStatusResponseError_NULL,
			},
			Data: &model.Membership{
				Tier:          uint32(psp.SubscriptionTier_TierBuilder1Year),
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

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(true)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)
		fx.cache.EXPECT().CacheSet(&psgsr2, mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
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

	t.Run("cache has error saved, GetSubscriptionStatus returns no error", func(t *testing.T) {
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
				// >> here:
				Code: pb.RpcMembershipGetStatusResponseError_PAYMENT_NODE_ERROR,
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

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return nil
		})
		// this should not be called because server returned Explorer tier
		// fx.cache.EXPECT().CacheEnable().Return(nil)

		fx.expectLimitsUpdated()

		// Call the function being tested
		_, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)
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
		req := &pb.RpcMembershipRegisterPaymentRequestRequest{
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
		req := &pb.RpcMembershipRegisterPaymentRequestRequest{
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
	t.Run("fail if SetRequest method fails", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.emailCollector.EXPECT().SetRequest(
			&pb.RpcMembershipGetVerificationEmailRequest{
				Email:                   "some@mail.com",
				SubscribeToNewsletter:   true,
				InsiderTipsAndTutorials: false,
				IsOnboardingList:        true,
			},
		).Return(errors.New("bad error")).Once()

		// Create a test request
		req := &pb.RpcMembershipGetVerificationEmailRequest{}
		req.Email = "some@mail.com"
		req.SubscribeToNewsletter = true
		req.InsiderTipsAndTutorials = false
		req.IsOnboardingList = true

		// Call the function being tested
		_, err := fx.GetVerificationEmail(ctx, req)
		assert.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.emailCollector.EXPECT().SetRequest(
			&pb.RpcMembershipGetVerificationEmailRequest{
				Email:                   "some@mail.com",
				SubscribeToNewsletter:   true,
				InsiderTipsAndTutorials: false,
				IsOnboardingList:        true,
			},
		).Return(nil).Once()

		// Create a test request
		req := &pb.RpcMembershipGetVerificationEmailRequest{}
		req.Email = "some@mail.com"
		req.SubscribeToNewsletter = true
		req.IsOnboardingList = true

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

		fx.cache.EXPECT().CacheDisableForNextMinutes(30).Return(nil).Once()

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
	t.Run("do not fail if no cache, pp client returned error", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.ppclient.EXPECT().GetAllTiers(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.GetTiersResponse, error) {
			return nil, errors.New("test error")
		}).MinTimes(1)

		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().CacheGet().Return(nil, nil, nil)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return nil
		})
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

		fx.expectLimitsUpdated()

		req := pb.RpcMembershipGetTiersRequest{
			NoCache: false,
			Locale:  "en_US",
		}
		out, err := fx.GetTiers(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(out.Tiers))
	})

	t.Run("success if no cache, empty response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
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

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
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

		// this should not be called because server returned Explorer tier
		// fx.cache.EXPECT().CacheEnable().Return(nil)

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

		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
		fx.cache.EXPECT().CacheGet().Return(&psgsr, nil, nil)
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
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
		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
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
		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)
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
		fx.cache.EXPECT().IsCacheExpired().Return(false)
		fx.cache.EXPECT().IsCacheDisabled().Return(false)

		fx.cache.EXPECT().CacheGet().Return(nil, &tgr, nil)
		// should call it to save status
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
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

	t.Run("error if name is not available", func(t *testing.T) {
		fx := newFixture(t)

		fx.ppclient.EXPECT().IsNameValid(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.IsNameValidResponse, error) {
			return &psp.IsNameValidResponse{
				Code: psp.IsNameValidResponse_Valid,
			}, nil
		}).MinTimes(1)

		rr := &pb.RpcNameServiceResolveNameRequest{NsName: "something", NsNameType: 0}
		fx.ns.EXPECT().NameServiceResolveName(ctx, rr).Return(&pb.RpcNameServiceResolveNameResponse{
			Error: &pb.RpcNameServiceResolveNameResponseError{
				Code: pb.RpcNameServiceResolveNameResponseError_NULL,
			},
			Available: false,
		}, nil)

		req := pb.RpcMembershipIsNameValidRequest{
			RequestedTier: 4,
			NsName:        "something",
			NsNameType:    model.NameserviceNameType_AnyName,
		}
		_, err := fx.IsNameValid(ctx, &req)
		assert.Error(t, err)
	})

	t.Run("success if name is empty", func(t *testing.T) {
		fx := newFixture(t)

		fx.ppclient.EXPECT().IsNameValid(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.IsNameValidResponse, error) {
			return &psp.IsNameValidResponse{
				Code: psp.IsNameValidResponse_Valid,
			}, nil
		}).MinTimes(1)

		req := pb.RpcMembershipIsNameValidRequest{
			RequestedTier: 4,
			NsName:        "",
			NsNameType:    model.NameserviceNameType_AnyName,
		}
		resp, err := fx.IsNameValid(ctx, &req)
		assert.NoError(t, err)
		assert.Equal(t, pb.RpcMembershipIsNameValidResponseErrorCode(0), resp.Error.Code)
	})

	t.Run("success if name is available", func(t *testing.T) {
		fx := newFixture(t)

		fx.ppclient.EXPECT().IsNameValid(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in interface{}) (*psp.IsNameValidResponse, error) {
			return &psp.IsNameValidResponse{
				Code: psp.IsNameValidResponse_Valid,
			}, nil
		}).MinTimes(1)

		rr := &pb.RpcNameServiceResolveNameRequest{NsName: "something", NsNameType: 0}
		fx.ns.EXPECT().NameServiceResolveName(ctx, rr).Return(&pb.RpcNameServiceResolveNameResponse{
			Error: &pb.RpcNameServiceResolveNameResponseError{
				Code: pb.RpcNameServiceResolveNameResponseError_NULL,
			},
			Available: true,
		}, nil)

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
			Receipt: "sjakflkajsfh.kajsflksadjflas.oicpvoxvpovi",
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
			Receipt: "sjakflkajsfh.kajsflksadjflas.oicpvoxvpovi",
		}

		// when
		resp, err := fx.VerifyAppStoreReceipt(ctx, req)

		// then
		assert.NoError(t, err)
		assert.Equal(t, pb.RpcMembershipVerifyAppStoreReceiptResponseErrorCode(0), resp.Error.Code)
	})
}

func TestCodeGetInfo(t *testing.T) {
	t.Run("should get code info successfully", func(t *testing.T) {
		// Given
		fx := newFixture(t)
		defer fx.finish(t)

		code := "TEST-CODE-123"
		expectedTier := uint32(psp.SubscriptionTier_TierBuilder1Year)

		// Mock PP client response
		fx.ppclient.EXPECT().
			CodeGetInfo(gomock.Any(), gomock.Any()).
			Return(&psp.CodeGetInfoResponse{
				Tier: expectedTier,
			}, nil)

		// mock GetAccountEthAddress
		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90")).Once()

		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, in *psp.GetSubscriptionRequestSigned) (*psp.GetSubscriptionResponse, error) {
			return &psp.GetSubscriptionResponse{
				Tier: expectedTier,
			}, nil
		}).MinTimes(1)

		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.AnythingOfType("*pb.RpcMembershipGetTiersResponse")).RunAndReturn(func(in *pb.RpcMembershipGetStatusResponse, tiers *pb.RpcMembershipGetTiersResponse) (err error) {
			return nil
		})

		fx.expectLimitsUpdated()

		// When
		resp, err := fx.CodeGetInfo(context.Background(), &pb.RpcMembershipCodeGetInfoRequest{
			Code: code,
		})

		// Then
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, expectedTier, resp.RequestedTier)
		assert.Equal(t, pb.RpcMembershipCodeGetInfoResponseError_NULL, resp.Error.Code)
	})

	t.Run("should return error if code is not found", func(t *testing.T) {
		// Given
		fx := newFixture(t)
		defer fx.finish(t)

		code := "TEST-CODE-123"

		// Mock PP client response
		fx.ppclient.EXPECT().
			CodeGetInfo(gomock.Any(), gomock.Any()).
			Return(&psp.CodeGetInfoResponse{
				Tier: 0,
			}, psp.ErrCodeNotFound)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90")).Once()

		// When
		resp, err := fx.CodeGetInfo(context.Background(), &pb.RpcMembershipCodeGetInfoRequest{
			Code: code,
		})

		// Then
		require.Equal(t, psp.ErrCodeNotFound, err)
		require.Nil(t, resp)
	})
}
