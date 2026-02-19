package cache

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	t.Run("should succeed when cache is in DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&model.Membership{
			Tier:   uint32(psp.SubscriptionTier_TierExplorer),
			Status: model.Membership_StatusActive,
		},
			[]*model.MembershipTierData{},
		)

		require.NoError(t, err)

		err = fx.CacheClear()
		require.NoError(t, err)

		_, _, expired, err := fx.CacheGet()
		require.NoError(t, err)
		require.True(t, time.Now().After(expired))
	})
}

func TestPayments_CacheGetSubscriptionStatus(t *testing.T) {
	t.Run("should fail if no record in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		_, _, _, err := fx.CacheGet()
		require.Equal(t, ErrCacheDbError, err)
	})

	t.Run("should succeed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&model.Membership{
			Tier:   uint32(psp.SubscriptionTier_TierExplorer),
			Status: model.Membership_StatusActive,
		},
			[]*model.MembershipTierData{},
		)
		require.NoError(t, err)

		out, _, _, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), out.Tier)
		require.Equal(t, model.Membership_StatusActive, out.Status)

		err = fx.CacheSet(&model.Membership{
			Tier: uint32(psp.SubscriptionTier_TierExplorer),
			// here
			Status: model.Membership_StatusUnknown,
		},
			[]*model.MembershipTierData{},
		)
		require.NoError(t, err)

		out, _, _, err = fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), out.Tier)
		require.Equal(t, model.Membership_StatusUnknown, out.Status)
	})

	t.Run("should return error if cache is cleared", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&model.Membership{
			Tier:   uint32(psp.SubscriptionTier_TierExplorer),
			Status: model.Membership_StatusActive,
		},
			[]*model.MembershipTierData{})
		require.NoError(t, err)

		err = fx.CacheClear()
		require.NoError(t, err)

		_, _, expired, err := fx.CacheGet()
		require.NoError(t, err)
		require.True(t, time.Now().After(expired))
	})
}

func TestPayments_CacheSetSubscriptionStatus(t *testing.T) {
	t.Run("should succeed if no record was in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&model.Membership{
			Tier:   uint32(psp.SubscriptionTier_TierExplorer),
			Status: model.Membership_StatusActive,
		},
			[]*model.MembershipTierData{},
		)
		require.Equal(t, nil, err)

		out, _, _, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, uint32(psp.SubscriptionTier_TierExplorer), out.Tier)
		require.Equal(t, model.Membership_StatusActive, out.Status)
	})

	t.Run("should succeed if cache is cleared", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheClear()
		require.NoError(t, err)

		err = fx.CacheSet(&model.Membership{
			Tier:   uint32(psp.SubscriptionTier_TierExplorer),
			Status: model.Membership_StatusActive,
		},
			[]*model.MembershipTierData{},
		)
		require.Equal(t, nil, err)
	})

	t.Run("should succeed if expire is set to 0", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheClear()
		require.NoError(t, err)

		err = fx.CacheSet(&model.Membership{
			Tier:   uint32(psp.SubscriptionTier_TierExplorer),
			Status: model.Membership_StatusActive,
		}, []*model.MembershipTierData{},
		)
		require.Equal(t, nil, err)

		_, _, _, err = fx.CacheGet()
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
