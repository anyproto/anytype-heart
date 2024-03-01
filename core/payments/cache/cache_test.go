package cache

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/assert"
)

var ctx = context.Background()

type fixture struct {
	a *app.App

	*cacheservice
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		a:            new(app.App),
		cacheservice: New().(*cacheservice),
	}

	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)
	fx.db = db

	//fx.a.Register(fx.ts)

	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func (fx *fixture) finish(t *testing.T) {
	assert.NoError(t, fx.a.Close(ctx))

	//assert.NoError(t, fx.db.Close())
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

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheDisabled, err)

		err = fx.CacheClear()
		require.NoError(t, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheExpired, err)
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

		timeNow := time.Now().UTC()
		timePlus5Hours := timeNow.Add(5 * time.Hour)

		err := fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, timePlus5Hours,
		)

		require.NoError(t, err)

		err = fx.CacheClear()
		require.NoError(t, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheExpired, err)
	})
}

func TestPayments_CacheGetSubscriptionStatus(t *testing.T) {
	t.Run("should fail if no record in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		_, err := fx.CacheGet()
		require.Equal(t, ErrCacheExpired, err)
	})

	t.Run("should succeed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		timeNow := time.Now().UTC()
		timePlus5Hours := timeNow.Add(5 * time.Hour)

		err := fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, timePlus5Hours)
		require.NoError(t, err)

		out, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)
		require.Equal(t, pb.RpcPaymentsSubscription_StatusActive, out.Status)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier: pb.RpcPaymentsSubscription_TierExplorer,
			// here
			Status: pb.RpcPaymentsSubscription_StatusUnknown,
		}, timePlus5Hours)
		require.NoError(t, err)

		out, err = fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)
		require.Equal(t, pb.RpcPaymentsSubscription_StatusUnknown, out.Status)
	})

	t.Run("should return object and error if cache is disabled", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		en := fx.IsCacheEnabled()
		require.Equal(t, true, en)

		err := fx.CacheDisableForNextMinutes(10)
		require.NoError(t, err)

		en = fx.IsCacheEnabled()
		require.Equal(t, false, en)

		timeNow := time.Now().UTC()
		timePlus5Hours := timeNow.Add(5 * time.Hour)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, timePlus5Hours)
		require.NoError(t, err)

		out, err := fx.CacheGet()
		require.Equal(t, ErrCacheDisabled, err)
		// HERE: weird semantics, error is returned too :-)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)

		err = fx.CacheEnable()
		require.NoError(t, err)

		en = fx.IsCacheEnabled()
		require.Equal(t, true, en)

		out, err = fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)
	})

	t.Run("should return error if cache is cleared", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		timeNow := time.Now().UTC()
		timePlus5Hours := timeNow.Add(5 * time.Hour)

		err := fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, timePlus5Hours)
		require.NoError(t, err)

		err = fx.CacheClear()
		require.NoError(t, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheExpired, err)
	})
}

func TestPayments_CacheSetSubscriptionStatus(t *testing.T) {
	t.Run("should succeed if no record was in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		timeNow := time.Now().UTC()
		timePlus5Hours := timeNow.Add(5 * time.Hour)

		err := fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, timePlus5Hours)
		require.Equal(t, nil, err)

		out, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)
		require.Equal(t, pb.RpcPaymentsSubscription_StatusActive, out.Status)
	})

	t.Run("should succeed if cache is disabled", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheDisableForNextMinutes(10)
		require.NoError(t, err)

		timeNow := time.Now().UTC()
		timePlus5Hours := timeNow.Add(5 * time.Hour)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, timePlus5Hours)
		require.Equal(t, nil, err)
	})

	t.Run("should succeed if cache is cleared", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheClear()
		require.NoError(t, err)

		timeNow := time.Now().UTC()
		timePlus5Hours := timeNow.Add(5 * time.Hour)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, timePlus5Hours)
		require.Equal(t, nil, err)
	})

	t.Run("should succeed if expire is set to 0", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheClear()
		require.NoError(t, err)

		timeNull := time.Time{}

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, timeNull)
		require.Equal(t, nil, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheExpired, err)
	})
}
