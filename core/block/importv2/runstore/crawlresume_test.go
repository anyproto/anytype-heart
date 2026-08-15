package runstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
)

// The DM-3 manifest surface: the serialized import request lives in the
// manifest exactly while the run is mid-crawl (OQ2, decided: stored as-is,
// no application-level encryption — the run dir sits in the account repo
// beside the wallet, and the coming encrypted any-store will cover it).
// Its useful life is the crawl, so it is SCRUBBED mechanically on every
// transition out of the crawl-resumable states — that is the time-boxed
// mitigation until encrypted any-store lands.

func newCrawlStore(t *testing.T, request []byte) *Store {
	t.Helper()
	store, err := Create(context.Background(), filepath.Join(t.TempDir(), "run-1"), Manifest{
		RunId:   "run-1",
		SpaceId: "space-1",
		Request: request,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestManifestRequestLifetime(t *testing.T) {
	token := []byte("serialized-request-with-secret-token")

	t.Run("the request round-trips through create and reopen", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := newCrawlStore(t, token)
		dir := store.Dir()
		require.NoError(t, store.Close())

		// when
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()
		m, err := reopened.Manifest(ctx)
		require.NoError(t, err)

		// then
		assert.Equal(t, token, m.Request, "a mid-crawl run keeps its request across restarts")
	})

	t.Run("the request survives suspend and the crawl-resume transition", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := newCrawlStore(t, token)

		// when: suspend (mid-crawl Close), then a new incarnation begins
		require.NoError(t, store.SetState(ctx, StateSuspended))
		m, err := store.Manifest(ctx)
		require.NoError(t, err)
		assert.Equal(t, token, m.Request, "a suspended crawl still needs its request to resume")

		resumed, err := store.BeginCrawlResume(ctx)
		require.NoError(t, err)

		// then: state back to running, the CRAWL budget spent, request intact
		assert.Equal(t, StateRunning, resumed.State)
		assert.Equal(t, 2, resumed.Incarnation)
		assert.Equal(t, 1, resumed.CrawlResumeAttempts)
		assert.Zero(t, resumed.ResumeAttempts,
			"a cheap crawl attempt must not spend the pass-3 budget, whose exhaustion is the destructive one (review P1)")
		assert.False(t, resumed.MaterializeStarted,
			"a crawl resume must NOT flip the compensation-scope switch")
		assert.Equal(t, token, resumed.Request)

		// and the counter is durable
		dir := store.Dir()
		require.NoError(t, store.Close())
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()
		persisted, err := reopened.Manifest(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, persisted.CrawlResumeAttempts)
	})

	t.Run("MarkFetched scrubs the request: its useful life ends with the crawl", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := newCrawlStore(t, token)

		// when
		require.NoError(t, store.MarkFetched(ctx, importv2.RootSpec{CollectionName: "Import"}))

		// then
		m, err := store.Manifest(ctx)
		require.NoError(t, err)
		assert.Empty(t, m.Request, "the token must not outlive the crawl")
	})

	t.Run("every terminal or cleanup transition scrubs, whatever writer performs it", func(t *testing.T) {
		// The rule lives at the single manifest write site so no present or
		// future writer can keep the token past the crawl by forgetting it.
		for _, state := range []State{StateCompleted, StateFailed, StateCompensating, StateCancelling, StateMaterializing} {
			t.Run(string(state), func(t *testing.T) {
				ctx := context.Background()
				store := newCrawlStore(t, token)
				require.NoError(t, store.SetState(ctx, state))
				m, err := store.Manifest(ctx)
				require.NoError(t, err)
				assert.Empty(t, m.Request, "state %s must scrub the request", state)
			})
		}
	})

	t.Run("BeginResume (pass-3) scrubs too: materialization never needs the request", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := newCrawlStore(t, token)

		// when: the pass-3 restart path (headless by design — §8.1)
		m, err := store.BeginResume(ctx)
		require.NoError(t, err)

		// then
		assert.Empty(t, m.Request)
	})

	t.Run("a refund on a mid-crawl run keeps the request", func(t *testing.T) {
		// given: one spent attempt on a suspended crawl
		ctx := context.Background()
		store := newCrawlStore(t, token)
		_, err := store.BeginCrawlResume(ctx)
		require.NoError(t, err)

		// when: the orderly-suspend refund (review Class F machinery), on the
		// counter the crawl attempt actually spent
		require.NoError(t, store.RefundCrawlResumeAttempt(ctx))

		// then
		m, err := store.Manifest(ctx)
		require.NoError(t, err)
		assert.Zero(t, m.CrawlResumeAttempts)
		assert.Zero(t, m.ResumeAttempts)
		assert.Equal(t, token, m.Request)
	})
}

func TestPlanJSONRoundTrip(t *testing.T) {
	t.Run("the recorded plan survives reopen; absence reads as nil", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := newCrawlStore(t, nil)

		none, err := store.ReadPlanJSON(ctx)
		require.NoError(t, err)
		assert.Nil(t, none, "no recorded plan reads as nil, not an error")

		// when
		plan := []byte(`{"Containers":{"db1":{"TypeKey":"task"}}}`)
		require.NoError(t, store.SetPlanJSON(ctx, plan))
		dir := store.Dir()
		require.NoError(t, store.Close())

		// then
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()
		got, err := reopened.ReadPlanJSON(ctx)
		require.NoError(t, err)
		assert.Equal(t, plan, got)
	})
}
