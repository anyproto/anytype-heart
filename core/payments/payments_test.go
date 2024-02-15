package payments

import (
	"context"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
	"github.com/tj/assert"
	"go.uber.org/mock/gomock"
)

var ctx = context.Background()

type fixture struct {
	a    *app.App
	ctrl *gomock.Controller
	//db     *badger.DB
	tmpDir string

	*service
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		a:    new(app.App),
		ctrl: gomock.NewController(t),

		service: New().(*service),
	}

	// init real (non-mocked) badger db
	tmpDir, err := os.MkdirTemp("", "payments_cache_*")
	require.NoError(t, err)
	fx.tmpDir = tmpDir

	db, err := badger.Open(badger.DefaultOptions(tmpDir).WithLoggingLevel(badger.ERROR))
	require.NoError(t, err)
	fx.db = db

	//fx.a.Register(fx.ts)

	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func (fx *fixture) finish(t *testing.T) {
	assert.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()

	//assert.NoError(t, fx.db.Close())
	_ = os.RemoveAll(fx.tmpDir)
}

func TestPayments_EnableCache(t *testing.T) {
	t.Run("should succeed", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.EnableCache()
		require.NoError(t, err)
	})

	t.Run("should succeed even when called twice", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.EnableCache()
		require.NoError(t, err)

		err = fx.EnableCache()
		require.NoError(t, err)
	})
}

func TestPayments_DisableCache(t *testing.T) {
	t.Run("should succeed with 0", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.DisableCacheForNextMinutes(0)
		require.NoError(t, err)
	})

	t.Run("should succeed even when called twice", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.DisableCacheForNextMinutes(60)
		require.NoError(t, err)

		err = fx.DisableCacheForNextMinutes(40)
		require.NoError(t, err)
	})

	t.Run("clear cache should remove disabling", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.DisableCacheForNextMinutes(60)
		require.NoError(t, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheDisabled, err)

		err = fx.ClearCache()
		require.NoError(t, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheExpired, err)
	})
}

func TestPayments_ClearCache(t *testing.T) {
	t.Run("should succeed even if no cache in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.ClearCache()
		require.NoError(t, err)
	})

	t.Run("should succeed even when called twice", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.ClearCache()
		require.NoError(t, err)

		err = fx.ClearCache()
		require.NoError(t, err)
	})

	t.Run("should succeed when cache is disabled", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.DisableCacheForNextMinutes(60)
		require.NoError(t, err)

		err = fx.ClearCache()
		require.NoError(t, err)
	})

	t.Run("should succeed when cache is in DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, 60,
		)

		require.NoError(t, err)

		err = fx.ClearCache()
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

		err := fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, 60)
		require.NoError(t, err)

		out, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)
		require.Equal(t, pb.RpcPaymentsSubscription_StatusActive, out.Status)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier: pb.RpcPaymentsSubscription_TierExplorer,
			// here
			Status: pb.RpcPaymentsSubscription_StatusUnknown,
		}, 60)
		require.NoError(t, err)

		out, err = fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)
		require.Equal(t, pb.RpcPaymentsSubscription_StatusUnknown, out.Status)
	})

	t.Run("should return error if cache is disabled", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.DisableCacheForNextMinutes(10)
		require.NoError(t, err)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, 60)
		require.NoError(t, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheDisabled, err)

		err = fx.EnableCache()
		require.NoError(t, err)

		out, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)
	})

	t.Run("should return error if cache is cleared", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, 60)
		require.NoError(t, err)

		err = fx.ClearCache()
		require.NoError(t, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheExpired, err)
	})
}

func TestPayments_CacheSetSubscriptionStatus(t *testing.T) {
	t.Run("should succeed if no record was in the DB", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, 60)
		require.Equal(t, nil, err)

		out, err := fx.CacheGet()
		require.NoError(t, err)
		require.Equal(t, pb.RpcPaymentsSubscription_TierExplorer, out.Tier)
		require.Equal(t, pb.RpcPaymentsSubscription_StatusActive, out.Status)
	})

	t.Run("should succeed if cache is disabled", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.DisableCacheForNextMinutes(10)
		require.NoError(t, err)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, 60)
		require.Equal(t, nil, err)
	})

	t.Run("should succeed if cache is cleared", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.ClearCache()
		require.NoError(t, err)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, 60)
		require.Equal(t, nil, err)
	})

	t.Run("should succeed if expire is set to 0", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		err := fx.ClearCache()
		require.NoError(t, err)

		err = fx.CacheSet(&pb.RpcPaymentsSubscriptionGetStatusResponse{
			Tier:   pb.RpcPaymentsSubscription_TierExplorer,
			Status: pb.RpcPaymentsSubscription_StatusActive,
		}, 0)
		require.Equal(t, nil, err)

		_, err = fx.CacheGet()
		require.Equal(t, ErrCacheExpired, err)
	})
}
