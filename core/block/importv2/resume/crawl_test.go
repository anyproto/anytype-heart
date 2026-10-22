package resume

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

// interruptedCrawl builds a run dir imitating a kill mid-crawl: claims
// recorded, one page spooled, NO fetched marker — the DM-3 resume class.
func interruptedCrawl(t *testing.T) *runstore.Store {
	t.Helper()
	ctx := context.Background()
	store, err := runstore.Create(ctx, filepath.Join(t.TempDir(), "run-1"), runstore.Manifest{
		RunId: "run-1", SpaceId: "space-1", Converter: "Notion",
		Request: []byte("serialized-request"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.RecordClaims(ctx, []runstore.ClaimRecord{
		{SourceKey: "page-1", ObjectId: "obj-1", PayloadRoot: []byte("root-1"), PayloadHeads: []string{"obj-1"}},
		{SourceKey: "page-2", ObjectId: "obj-2", PayloadRoot: []byte("root-2"), PayloadHeads: []string{"obj-2"}},
		{SourceKey: "page-3", ObjectId: "obj-3", Matched: true},
	}))
	spool, err := store.Spool(ctx)
	require.NoError(t, err)
	require.NoError(t, spool.Append(ctx, &importv2.Object{
		SourceKey: "page-1", SbType: coresb.SmartBlockTypePage, Payload: &importv2.Snapshot{},
	}))
	// one content warning, plus the abort records an interrupted crawl leaves
	// behind (the suspend's cancelled fatal; a transient rate-limit fatal a
	// kept-for-retry attempt recorded)
	require.NoError(t, store.AppendIssue(ctx, runstore.IssueRecord{
		Severity: int(importv2.SeverityWarning), Code: string(importv2.IssueDataLoss),
		SourceKey: "page-1", Message: "recorded in incarnation 1",
	}))
	require.NoError(t, store.AppendIssue(ctx, runstore.IssueRecord{
		Severity: int(importv2.SeverityFatal), Code: string(importv2.IssueCancelled),
		Message: "import suspended",
	}))
	require.NoError(t, store.AppendIssue(ctx, runstore.IssueRecord{
		Severity: int(importv2.SeverityFatal), Code: string(importv2.IssueRateLimited),
		Message: "rate limit budget exhausted",
	}))
	return store
}

func TestLoadCrawl(t *testing.T) {
	ctx := context.Background()

	t.Run("the crawl seed: spool census, prior claims, content issues only", func(t *testing.T) {
		// given
		store := interruptedCrawl(t)

		// when
		state, err := LoadCrawl(ctx, store)

		// then
		require.NoError(t, err)
		assert.Equal(t, map[string]struct{}{"page-1": {}}, state.Engine.SpooledKeys,
			"the spool is the skip set — no separate status bookkeeping")
		assert.Equal(t, map[string]struct{}{
			"page-1": {}, "page-2": {}, "page-3": {},
		}, state.Engine.PriorClaims)
		require.Len(t, state.Engine.Issues, 1,
			"fatal records are the interrupted incarnation's lifecycle, not content")
		assert.Equal(t, importv2.IssueDataLoss, state.Engine.Issues[0].Code)
		assert.Equal(t, []byte("serialized-request"), state.Manifest.Request)
	})

	t.Run("the identity option rehydrates reclaimable claims: re-enumeration mints nothing", func(t *testing.T) {
		// given
		store := interruptedCrawl(t)
		state, err := LoadCrawl(ctx, store)
		require.NoError(t, err)

		// when: the resumed pass 1 re-claims a prior key
		objectStore := objectstore.NewStoreFixture(t)
		space := mock_clientspace.NewMockSpace(t) // no expectations: nothing mints
		service := identity.NewService(space, objectStore.SpaceIndex("space-1"), false,
			time.Unix(1700000000, 0), state.IdentityOption())
		require.NoError(t, service.Claim(ctx, importv2.IdentityClaim{
			SourceKey: "page-2", SbType: coresb.SmartBlockTypePage, SourceFilePath: "page-2",
		}))

		// then: the recorded identity stands, payload bytes included
		assignment, err := service.Assign("page-2")
		require.NoError(t, err)
		assert.Equal(t, "obj-2", assignment.Id)
		assert.Equal(t, []byte("root-2"), assignment.Payload.RootRawChange.GetRawChange())
	})

	t.Run("a materialize-started run refuses the crawl loader", func(t *testing.T) {
		// given: the two resume classes are disjoint by the sticky marker;
		// crossing them would rehydrate effect rows as reclaimable claims.
		store := interruptedCrawl(t)
		require.NoError(t, store.MarkFetched(ctx, importv2.RootSpec{}))

		// when
		_, err := LoadCrawl(ctx, store)

		// then
		require.Error(t, err)
	})

	t.Run("effect-shaped rows in a crawl-phase ledger refuse loudly", func(t *testing.T) {
		// given: a terminal row without the materialize marker is a ledger
		// that contradicts its own manifest — replaying it as a reclaimable
		// claim would resurrect an object the compensation scope excludes.
		store := interruptedCrawl(t)
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))

		// when
		_, err := LoadCrawl(ctx, store)

		// then: strict-loud (the sweep's attempt cap then routes the dir to
		// compensation), never a silent wrong replay
		require.Error(t, err)
	})

	t.Run("a minted claim without payload bytes refuses", func(t *testing.T) {
		// given — the Load sibling rule: the id is the hash of exactly those
		// bytes; RecordClaims writes both in one tx, so this shape is
		// corruption.
		store := interruptedCrawl(t)
		require.NoError(t, store.RecordClaims(ctx, []runstore.ClaimRecord{
			{SourceKey: "page-4", ObjectId: "obj-4"}, // minted, no payload
		}))

		// when
		_, err := LoadCrawl(ctx, store)

		// then
		require.Error(t, err)
	})

	t.Run("a spooled page without its claim row refuses loudly", func(t *testing.T) {
		// given — review P0-D: spool rows commit immediately, claims used to
		// batch until the end of pass 2, and a kill between the two left this
		// exact shape. Since the write-ahead fix (a late claim flushes before
		// its append) it can only mean corruption — and replaying it would
		// fail the whole resumed import at pass 3 ('object was not claimed in
		// pass 1'), AFTER the re-crawl spent its requests. Strict-loud here,
		// like every other load contradiction.
		store := interruptedCrawl(t)
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		require.NoError(t, spool.Append(ctx, &importv2.Object{
			SourceKey: "orphan-page", SbType: coresb.SmartBlockTypePage, Payload: &importv2.Snapshot{},
		}))

		// when
		_, err = LoadCrawl(ctx, store)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "orphan-page")
	})

	t.Run("derived and file spool rows need no claim row", func(t *testing.T) {
		// given: relations/types/options are never pass-1 claims (the replay
		// re-derives them) and files ride futures — the claim/spool
		// cross-check must exempt exactly those classes.
		store := interruptedCrawl(t)
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		require.NoError(t, spool.Append(ctx, &importv2.Object{
			SourceKey: "relation:tags", SbType: coresb.SmartBlockTypeRelation, Payload: &importv2.Snapshot{},
		}))
		require.NoError(t, spool.Append(ctx, &importv2.Object{
			SourceKey: "pic.png", SbType: coresb.SmartBlockTypeFileObject, Payload: &importv2.Snapshot{},
		}))

		// when
		state, err := LoadCrawl(ctx, store)

		// then
		require.NoError(t, err)
		assert.Contains(t, state.Engine.SpooledKeys, "relation:tags")
		assert.Contains(t, state.Engine.SpooledKeys, "pic.png")
	})

	t.Run("the recorded plan rides the seed", func(t *testing.T) {
		// given
		store := interruptedCrawl(t)
		require.NoError(t, store.SetPlanJSON(ctx, []byte(`{"Containers":{}}`)))

		// when
		state, err := LoadCrawl(ctx, store)

		// then
		require.NoError(t, err)
		assert.Equal(t, []byte(`{"Containers":{}}`), state.PlanJSON)
	})
}
