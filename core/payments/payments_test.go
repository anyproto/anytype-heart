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

// TestGetSubscriptionStatus tests the cache-only RPC method
func TestGetStatus(t *testing.T) {
	t.Run("return default if cache is empty (cache-only RPC)", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// RPC method only calls CacheGet() and returns empty status if cache is empty
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)

		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)
		assert.Equal(t, uint32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusUnknown, resp.Data.Status)
	})

	t.Run("NoCache flag is ignored (cache-only RPC)", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)

		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{NoCache: true})
		assert.NoError(t, err)
		assert.Equal(t, uint32(psp.SubscriptionTier_TierUnknown), resp.Data.Tier)
	})

	t.Run("returns cached values (cache-only RPC)", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		cachedStatus := pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{Code: pb.RpcMembershipGetStatusResponseError_NULL},
			Data: &model.Membership{
				Tier:          uint32(psp.SubscriptionTier_TierExplorer),
				Status:        model.Membership_StatusActive,
				PaymentMethod: model.Membership_MethodCrypto,
			},
		}

		fx.cache.EXPECT().CacheGet().Return(&cachedStatus, nil, nil)

		resp, err := fx.GetSubscriptionStatus(ctx, &pb.RpcMembershipGetStatusRequest{})
		assert.NoError(t, err)
		assert.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), resp.Data.Tier)
		assert.Equal(t, model.Membership_StatusActive, resp.Data.Status)
	})
}

// TestRefreshSubscriptionStatusBackground tests the background network fetching
func TestRefreshSubscriptionStatusBackground(t *testing.T) {
	t.Run("network success - updates cache and triggers limits update", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// Old cache is empty
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)
		fx.cache.EXPECT().IsCacheExpired().Return(false)

		// Network call succeeds
		networkStatus := &psp.GetSubscriptionResponse{
			Tier:             uint32(psp.SubscriptionTier_TierExplorer),
			Status:           psp.SubscriptionStatus_StatusActive,
			DateStarted:      uint64(timeNow.Unix()),
			DateEnds:         uint64(subsExpire.Unix()),
			IsAutoRenew:      true,
			PaymentMethod:    psp.PaymentMethod_MethodCrypto,
			RequestedAnyName: "alice.any",
		}
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).Return(networkStatus, nil)

		// Cache should be updated
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.Anything).Return(nil)

		// Limits should be updated because status changed
		fx.expectLimitsUpdated()

		// Call background refresh
		err := fx.service.refreshSubscriptionStatusBackground(ctx)
		assert.NoError(t, err)
	})

	t.Run("network error with cache - uses cached data", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// Cache has data
		cachedStatus := &pb.RpcMembershipGetStatusResponse{
			Error: &pb.RpcMembershipGetStatusResponseError{Code: pb.RpcMembershipGetStatusResponseError_NULL},
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		}
		fx.cache.EXPECT().CacheGet().Return(cachedStatus, nil, nil)
		fx.cache.EXPECT().IsCacheExpired().Return(false)

		// Network call fails
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).Return(nil, errors.New("network error"))

		// Should NOT update cache or limits (using cached data)

		// Call background refresh
		err := fx.service.refreshSubscriptionStatusBackground(ctx)
		assert.NoError(t, err) // No error - gracefully uses cache
	})

	t.Run("network error with no cache - creates empty status", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// Cache is empty
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)
		fx.cache.EXPECT().IsCacheExpired().Return(false)

		// Network call fails
		fx.ppclient.EXPECT().GetSubscriptionStatus(gomock.Any(), gomock.Any()).Return(nil, errors.New("network error"))

		// Should save empty status to cache
		fx.cache.EXPECT().CacheSet(mock.AnythingOfType("*pb.RpcMembershipGetStatusResponse"), mock.Anything).Return(nil)

		// Limits updated because we went from no cache to empty status
		fx.expectLimitsUpdated()

		// Call background refresh
		err := fx.service.refreshSubscriptionStatusBackground(ctx)
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
	t.Run("return empty if cache is empty (cache-only RPC)", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		// RPC method only calls CacheGet() and returns empty tiers if cache is empty
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)

		resp, err := fx.GetTiers(ctx, &pb.RpcMembershipGetTiersRequest{})
		assert.NoError(t, err)
		assert.Empty(t, resp.Tiers)
	})

	t.Run("returns cached tiers (cache-only RPC)", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		cachedTiers := &pb.RpcMembershipGetTiersResponse{
			Tiers: []*model.MembershipTierData{
				{Id: 1, Name: "Explorer"},
				{Id: 2, Name: "Builder"},
			},
		}
		fx.cache.EXPECT().CacheGet().Return(nil, cachedTiers, nil)

		resp, err := fx.GetTiers(ctx, &pb.RpcMembershipGetTiersRequest{})
		assert.NoError(t, err)
		assert.Len(t, resp.Tiers, 2)
		assert.Equal(t, "Explorer", resp.Tiers[0].Name)
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

		// Mock PP client response for CodeGetInfo
		fx.ppclient.EXPECT().
			CodeGetInfo(gomock.Any(), gomock.Any()).
			Return(&psp.CodeGetInfoResponse{
				Tier: expectedTier,
			}, nil)

		fx.wallet.EXPECT().GetAccountEthAddress().Return(common.HexToAddress("0x55DCad916750C19C4Ec69D65Ff0317767B36cE90")).Once()

		// CodeGetInfo internally calls GetSubscriptionStatus which is now cache-only
		// It will just return from cache or empty status (no network call)
		fx.cache.EXPECT().CacheGet().Return(nil, nil, cache.ErrCacheDbError)

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
