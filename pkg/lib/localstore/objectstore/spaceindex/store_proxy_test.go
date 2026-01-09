package spaceindex

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
)

func TestStoreProxy_ModeTransition(t *testing.T) {
	t.Run("uninitialized store returns empty results", func(t *testing.T) {
		// given
		store := newTestStoreProxy(t)

		// when - query without initialization
		records, err := store.Query(database.Query{})

		// then - should return empty results, no error
		require.NoError(t, err)
		assert.Empty(t, records)
		assert.False(t, store.IsInitialized())
	})

	t.Run("uninitialized store returns error on write", func(t *testing.T) {
		// given
		store := newTestStoreProxy(t)

		// when - try to write without initialization
		err := store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyName: domain.String("Test Object"),
		}))

		// then - should return ErrSpaceNotInitialized
		require.ErrorIs(t, err, ErrSpaceNotInitialized)
	})

	t.Run("initialized store can read and write", func(t *testing.T) {
		// given
		store := newTestStoreProxy(t)
		require.NoError(t, store.Init())
		assert.True(t, store.IsInitialized())

		// when - write data
		err := store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyName: domain.String("Test Object"),
		}))
		require.NoError(t, err)

		// then - can read data back
		details, err := store.GetDetails("obj1")
		require.NoError(t, err)
		assert.Equal(t, "Test Object", details.GetString(bundle.RelationKeyName))
	})

	t.Run("close transitions back to empty store", func(t *testing.T) {
		// given - initialized store with data
		store := newTestStoreProxy(t)
		require.NoError(t, store.Init())

		err := store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyName: domain.String("Test Object"),
		}))
		require.NoError(t, err)

		// verify data exists
		details, err := store.GetDetails("obj1")
		require.NoError(t, err)
		assert.Equal(t, "Test Object", details.GetString(bundle.RelationKeyName))

		// when - close the store
		err = store.Close()
		require.NoError(t, err)

		// then - should be uninitialized and return empty results
		assert.False(t, store.IsInitialized())

		// read returns empty details (not error)
		details, err = store.GetDetails("obj1")
		require.NoError(t, err)
		assert.True(t, details.Len() == 0)

		// write returns error
		err = store.UpdateObjectDetails(context.Background(), "obj2", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("obj2"),
		}))
		require.ErrorIs(t, err, ErrSpaceNotInitialized)
	})

	t.Run("reinitialize after close", func(t *testing.T) {
		// given - store that was initialized, used, and closed
		store := newTestStoreProxy(t)
		require.NoError(t, store.Init())

		err := store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyName: domain.String("Test Object"),
		}))
		require.NoError(t, err)

		err = store.Close()
		require.NoError(t, err)
		assert.False(t, store.IsInitialized())

		// when - reinitialize
		err = store.Init()
		require.NoError(t, err)

		// then - store is usable again, old data persisted
		assert.True(t, store.IsInitialized())
		details, err := store.GetDetails("obj1")
		require.NoError(t, err)
		assert.Equal(t, "Test Object", details.GetString(bundle.RelationKeyName))
	})

	t.Run("multiple init calls are idempotent", func(t *testing.T) {
		// given
		store := newTestStoreProxy(t)

		// when - call Init multiple times
		require.NoError(t, store.Init())
		require.NoError(t, store.Init())
		require.NoError(t, store.Init())

		// then - store is initialized
		assert.True(t, store.IsInitialized())
	})

	t.Run("close on uninitialized store is no-op", func(t *testing.T) {
		// given
		store := newTestStoreProxy(t)

		// when - close without init
		err := store.Close()

		// then - no error
		require.NoError(t, err)
		assert.False(t, store.IsInitialized())
	})
}

func newTestStoreProxy(t testing.TB) *storeProxy {
	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return("wallet").Maybe()
	walletService.EXPECT().RepoPath().Return(t.TempDir())
	walletService.EXPECT().FtsPrimaryLang().Return("")

	provider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	fullText := ftsearch.TantivyNew()
	testApp := &app.App{}
	testApp.Register(walletService)
	err = fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = fullText.Close(context.Background())
	})

	store := New(context.Background(), "test-space", Deps{
		DbProvider:    provider,
		Fts:           fullText,
		SourceService: &detailsFromId{},
		SubManager:    &SubscriptionManager{},
		FulltextQueue: &dummyFulltextQueue{},
	})

	return store.(*storeProxy)
}
