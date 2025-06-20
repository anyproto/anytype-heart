package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"

	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
)

const delta = 1 * time.Second

var ctx = context.Background()

type fixture struct {
	a *app.App

	*cacheservice
}

func newTestAnystore(t *testing.T) anystore.DB {
	db, err := anystore.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func newFixture(t *testing.T) *fixture {
	testApp := new(app.App)
	fx := &fixture{
		a:            testApp,
		cacheservice: New().(*cacheservice),
	}

	dbProvider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	testApp.Register(dbProvider)

	err = fx.Init(testApp)
	require.NoError(t, err)

	// fx.a.Register(fx.ts)

	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func (fx *fixture) finish(t *testing.T) {
	assert.NoError(t, fx.a.Close(ctx))

	// assert.NoError(t, fx.db.Close())
}

func TestPayments_EnableCache(t *testing.T) {
	t.Run("should succeed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheEnable()
		require.NoError(t, err)
	})

	t.Run("should succeed even when called twice", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheEnable()
		require.NoError(t, err)

		err = fx.CacheEnable()
		require.NoError(t, err)
	})
}

func TestPayments_DisableCache(t *testing.T) {
	t.Run("should succeed with 0", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheDisableForNextMinutes(0)
		require.NoError(t, err)
	})

	t.Run("should succeed even when called twice", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheDisableForNextMinutes(60)
		require.NoError(t, err)

		err = fx.CacheDisableForNextMinutes(40)
		require.NoError(t, err)
	})

	t.Run("clear cache should remove disabling", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheDisableForNextMinutes(60)
		require.NoError(t, err)

		_, _, err = fx.CacheGet()
		require.NoError(t, err)
		require.True(t, fx.IsCacheDisabled())

		err = fx.CacheClear()
		require.NoError(t, err)

		_, _, err = fx.CacheGet()
		require.NoError(t, err)
		require.False(t, fx.IsCacheDisabled())
	})
}

func TestPayments_ClearCache(t *testing.T) {
	t.Run("should succeed even if no cache in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheClear()
		require.NoError(t, err)
	})

	t.Run("should succeed even when called twice", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheClear()
		require.NoError(t, err)

		err = fx.CacheClear()
		require.NoError(t, err)
	})

	t.Run("should succeed when cache is disabled", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheDisableForNextMinutes(60)
		require.NoError(t, err)

		err = fx.CacheClear()
		require.NoError(t, err)
	})

	t.Run("should succeed when cache is in DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)

		require.NoError(t, err)

		err = fx.CacheClear()
		require.NoError(t, err)

		_, _, err = fx.CacheGet()
		require.NoError(t, err)
		require.True(t, fx.IsCacheExpired())
	})
}

func TestPayments_CacheGetSubscriptionStatus(t *testing.T) {
	t.Run("should fail if no record in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		_, _, err := fx.CacheGet()
		require.Equal(t, ErrCacheDbError, err)
	})

	t.Run("should succeed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)
		require.NoError(t, err)

		out, _, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), out.Data.Tier)
		require.Equal(t, model.Membership_StatusActive, out.Data.Status)

		err = fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier: uint32(psp.SubscriptionTier_TierExplorer),
				// here
				Status: model.Membership_StatusUnknown,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)
		require.NoError(t, err)

		out, _, err = fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), out.Data.Tier)
		require.Equal(t, model.Membership_StatusUnknown, out.Data.Status)
	})

	t.Run("should return object and error if cache is disabled", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		dis := fx.IsCacheDisabled()
		require.False(t, dis)

		err := fx.CacheDisableForNextMinutes(10)
		require.NoError(t, err)

		dis = fx.IsCacheDisabled()
		require.True(t, dis)

		err = fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)
		require.NoError(t, err)
		dis = fx.IsCacheDisabled()
		require.True(t, dis)

		out, _, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), out.Data.Tier)

		err = fx.CacheEnable()
		require.NoError(t, err)

		dis = fx.IsCacheDisabled()
		require.False(t, dis)

		out, _, err = fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), out.Data.Tier)
	})

	t.Run("should return error if cache is cleared", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)
		require.NoError(t, err)

		err = fx.CacheClear()
		require.NoError(t, err)

		// check if cache is expired
		exp := fx.IsCacheExpired()
		require.True(t, exp)

		_, _, err = fx.CacheGet()
		require.NoError(t, err)
	})
}

func TestPayments_CacheSetSubscriptionStatus(t *testing.T) {
	t.Run("should succeed if no record was in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)
		require.Equal(t, nil, err)

		out, _, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), out.Data.Tier)
		require.Equal(t, model.Membership_StatusActive, out.Data.Status)
	})

	t.Run("should succeed if cache is disabled", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheDisableForNextMinutes(10)
		require.NoError(t, err)

		err = fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)
		require.Equal(t, nil, err)
	})

	t.Run("should succeed if cache is cleared", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheClear()
		require.NoError(t, err)

		err = fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)
		require.Equal(t, nil, err)
	})

	t.Run("should succeed if expire is set to 0", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheClear()
		require.NoError(t, err)

		err = fx.CacheSet(&pb.RpcMembershipGetStatusResponse{
			Data: &model.Membership{
				Tier:   uint32(psp.SubscriptionTier_TierExplorer),
				Status: model.Membership_StatusActive,
			},
		},
			&pb.RpcMembershipGetTiersResponse{
				Tiers: []*model.MembershipTierData{},
			},
		)
		require.Equal(t, nil, err)

		_, _, err = fx.CacheGet()
		require.Equal(t, nil, err)
	})
}

func assertTimeNear(actual, expected time.Time, delta time.Duration) bool {
	// actual ∊ [expected - delta; expected + delta]
	return actual.After(expected.Add(-1*delta)) && expected.Add(delta).After(actual)
}

func TestGetExpireTime(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   *model.Membership
		duration time.Time
	}{
		{"should return 10 minutes in case of nil", nil, time.Now().UTC().Add(cacheLifetimeDurOther)},
		{"should return 24 hours in case of Explorer", &model.Membership{Tier: 1}, time.Now().UTC().Add(cacheLifetimeDurExplorer)},
		{"should return 10 minutes in case of other", &model.Membership{Tier: 3}, time.Now().UTC().Add(cacheLifetimeDurOther)},
		{"should return dateEnds in case it is earlier than 10 minutes",
			&model.Membership{Tier: 4, DateEnds: uint64(time.Now().UTC().Add(3 * time.Minute).Unix())}, time.Now().UTC().Add(3 * time.Minute)},
		{"should return 10 minutes in case dateEnds is expired",
			&model.Membership{Tier: 3, DateEnds: uint64(time.Now().UTC().Add(-10 * time.Hour).Unix())}, time.Now().UTC().Add(cacheLifetimeDurOther)},
		{"should return 10 minutes in case dateEnds is 0",
			&model.Membership{Tier: 3, DateEnds: uint64(time.Unix(0, 0).Unix())}, time.Now().UTC().Add(cacheLifetimeDurOther)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			fx := newFixture(t)
			defer fx.finish(t)

			// when
			expire := getExpireTime(tc.status)

			// then
			assert.True(t, assertTimeNear(expire, tc.duration, delta))
		})
	}
}
