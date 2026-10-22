package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"

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

func TestPayments_CacheV2(t *testing.T) {
	t.Run("missing entry loads as nothing fetched", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		snap, err := fx.CacheV2Load(ctx)
		require.NoError(t, err)
		assert.False(t, snap.StatusFetched)
		assert.False(t, snap.ProductsFetched)
	})

	t.Run("commit writes only the set resource and marks it fetched", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		at := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		status := &model.MembershipV2Data{TeamOwnerID: "owner"}
		require.NoError(t, fx.CacheV2Commit(ctx, V2Commit{SetStatus: true, Status: status, SuccessAt: at}))
		// a successfully fetched empty products list
		require.NoError(t, fx.CacheV2Commit(ctx, V2Commit{SetProducts: true, Products: nil, SuccessAt: at.Add(time.Second)}))

		snap, err := fx.CacheV2Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, "owner", snap.Status.TeamOwnerID, "products commit kept status")
		assert.True(t, snap.StatusFetched)
		assert.True(t, snap.StatusLastSuccessAt.Equal(at))
		assert.True(t, snap.ProductsFetched)
		assert.NotNil(t, snap.Products)
		assert.Empty(t, snap.Products)
		assert.True(t, snap.ExpireTime.IsZero(), "no renewal unless asked")

		require.NoError(t, fx.CacheV2Commit(ctx, V2Commit{SetProducts: true, Products: []*model.MembershipV2Product{{Id: "p"}}, SuccessAt: at}))
		require.NoError(t, fx.CacheV2Commit(ctx, V2Commit{SetStatus: true, Status: status, SuccessAt: at, RenewExpiry: true}))
		snap, err = fx.CacheV2Load(ctx)
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().Add(cacheLifetimeDurOther), snap.ExpireTime, delta)
		require.Len(t, snap.Products, 1, "status commit kept products")
		assert.Equal(t, "p", snap.Products[0].Id)
	})

	t.Run("legacy entry migrates conservatively without a wipe", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)

		expire := time.Now().Add(time.Hour)
		require.NoError(t, fx.dbV2.Set(ctx, anystoreprovider.SystemKeys.PaymentCacheV2Key(cacheV2LastVersion), &StorageV2Struct{
			CurrentVersion: cacheV2LastVersion,
			ExpireTime:     expire,
			V2Data: &model.MembershipV2Data{Products: []*model.MembershipV2PurchasedProduct{
				{Product: &model.MembershipV2Product{Id: "p1"}},
			}},
			// written as a placeholder along with the status
			ProductsData: []*model.MembershipV2Product{},
		}))

		snap, err := fx.CacheV2Load(ctx)
		require.NoError(t, err)
		assert.True(t, snap.StatusFetched, "status with content counts as fetched")
		assert.True(t, snap.StatusLastSuccessAt.IsZero(), "unknown age, not derived from the expiry")
		assert.False(t, snap.ProductsFetched, "an empty placeholder doesn't count")
		require.Len(t, snap.Status.Products, 1)

		// a later field-scoped commit keeps the migrated status
		require.NoError(t, fx.CacheV2Commit(ctx, V2Commit{SetProducts: true, Products: []*model.MembershipV2Product{{Id: "x"}}, SuccessAt: time.Now()}))
		snap, err = fx.CacheV2Load(ctx)
		require.NoError(t, err)
		assert.True(t, snap.StatusFetched)
		require.Len(t, snap.Status.Products, 1)
		assert.True(t, snap.ProductsFetched)
	})

	t.Run("placeholder status in a legacy entry is not fetched", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		require.NoError(t, fx.dbV2.Set(ctx, anystoreprovider.SystemKeys.PaymentCacheV2Key(cacheV2LastVersion), &StorageV2Struct{
			CurrentVersion: cacheV2LastVersion,
			V2Data:         &model.MembershipV2Data{},
			ProductsData:   []*model.MembershipV2Product{{Id: "p"}},
		}))
		snap, err := fx.CacheV2Load(ctx)
		require.NoError(t, err)
		assert.False(t, snap.StatusFetched)
		assert.True(t, snap.ProductsFetched)
	})
}

// failingGetStore fails reads and records writes
type failingGetStore struct {
	keyvaluestore.Store[*StorageV2Struct]
	sets int
}

func (s *failingGetStore) Get(context.Context, string) (*StorageV2Struct, error) {
	return nil, errors.New("disk I/O error")
}

func (s *failingGetStore) Set(context.Context, string, *StorageV2Struct) error {
	s.sets++
	return nil
}

func TestPayments_CacheV2CommitReadError(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)
	store := &failingGetStore{Store: fx.dbV2}
	fx.dbV2 = store

	err := fx.CacheV2Commit(ctx, V2Commit{SetProducts: true, Products: []*model.MembershipV2Product{{Id: "p"}}, SuccessAt: time.Now()})
	require.Error(t, err)
	assert.Zero(t, store.sets, "a read failure never replaces the other resource with a placeholder")
}

func TestPayments_CacheV2LockBounded(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)
	fx.lock() // e.g. a slow V1 write holds the cache lock
	defer fx.unlock()

	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := fx.CacheV2Commit(short, V2Commit{SetProducts: true, SuccessAt: time.Now()})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = fx.CacheV2Load(short)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
}

// hangingSetStore has no entry and blocks writes until ctx ends
type hangingSetStore struct {
	keyvaluestore.Store[*StorageV2Struct]
}

func (s *hangingSetStore) Get(context.Context, string) (*StorageV2Struct, error) {
	return nil, anystore.ErrDocNotFound
}

func (s *hangingSetStore) Set(ctx context.Context, _ string, _ *StorageV2Struct) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestPayments_CacheV2CommitWriteBounded(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)
	fx.dbV2 = &hangingSetStore{Store: fx.dbV2}
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- fx.CacheV2Commit(short, V2Commit{SetProducts: true, SuccessAt: time.Now()}) }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(2 * time.Second):
		t.Fatal("the write ignored ctx")
	}
}

// hangingGetStore blocks reads until ctx ends
type hangingGetStore struct {
	keyvaluestore.Store[*StorageV2Struct]
}

func (s *hangingGetStore) Get(ctx context.Context, _ string) (*StorageV2Struct, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestPayments_CacheV2LoadReadBounded(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)
	fx.dbV2 = &hangingGetStore{Store: fx.dbV2}
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := fx.CacheV2Load(short)
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("the read ignored ctx")
	}
}

func TestPayments_CacheCloseClearsV2(t *testing.T) {
	fx := newFixture(t)
	require.NoError(t, fx.a.Close(ctx))
	_, err := fx.CacheV2Load(ctx)
	require.Error(t, err)
}
