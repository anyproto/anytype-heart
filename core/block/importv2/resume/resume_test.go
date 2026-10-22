package resume

import (
	"context"
	"path/filepath"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/report"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

// interruptedRun builds a run dir imitating a crash mid-materialize:
// pass 1+2 complete (claims + spool + fetched marker), pass 3 partially
// journaled, finalize partially done.
func interruptedRun(t *testing.T) *runstore.Store {
	t.Helper()
	ctx := context.Background()
	store, err := runstore.Create(ctx, filepath.Join(t.TempDir(), "run-1"), runstore.Manifest{
		RunId: "run-1", SpaceId: "space-1", Converter: "Markdown",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// pass 1: three claims — two minted, one matched
	require.NoError(t, store.RecordClaims(ctx, []runstore.ClaimRecord{
		{SourceKey: "page-1", ObjectId: "obj-1", PayloadRoot: []byte("root-1"), PayloadHeads: []string{"obj-1"}},
		{SourceKey: "page-2", ObjectId: "obj-2", PayloadRoot: []byte("root-2"), PayloadHeads: []string{"obj-2"}},
		{SourceKey: "page-3", ObjectId: "obj-3", Matched: true},
	}))
	// pass 2: the spool holds the three pages plus a derived definition
	spool, err := store.Spool(ctx)
	require.NoError(t, err)
	for key, sbType := range map[string]coresb.SmartBlockType{
		"page-1": coresb.SmartBlockTypePage,
		"page-2": coresb.SmartBlockTypePage,
		"page-3": coresb.SmartBlockTypePage,
		"rel-1":  coresb.SmartBlockTypeRelation,
	} {
		require.NoError(t, spool.Append(ctx, &importv2.Object{
			SourceKey: key, SbType: sbType, Payload: &importv2.Snapshot{},
		}))
	}
	require.NoError(t, store.MarkFetched(ctx, importv2.RootSpec{CollectionName: "Import"}))

	// pass 3, interrupted: page-1 created, page-3 updated, the derived
	// definition created (a fresh late row, but in the spool); page-2 never
	// finished. Finalize: the collection and report persisted, and one more
	// finalize claim was interrupted before its object existed.
	require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
	require.NoError(t, store.RecordUpdated(ctx, "page-3", "obj-3"))
	require.NoError(t, store.RecordCreated(ctx, "rel-1", "obj-rel"))
	require.NoError(t, store.RecordFile(ctx, "img.png", "file-1", false))
	require.NoError(t, store.RecordCreated(ctx, "collection:Import 2026", "obj-root"))
	require.NoError(t, store.RecordCreated(ctx, report.SourceKey, "obj-report"))
	require.NoError(t, store.RecordClaims(ctx, []runstore.ClaimRecord{
		{SourceKey: "collection:Interrupted", ObjectId: "obj-abandoned", PayloadRoot: []byte("r")},
	}))

	// issues: one real warning, plus the suspend's own abort record
	require.NoError(t, store.AppendIssue(ctx, runstore.IssueRecord{
		Severity: int(importv2.SeverityWarning), Code: string(importv2.IssueDataLoss),
		SourceKey: "page-1", Message: "recorded in incarnation 1",
	}))
	require.NoError(t, store.AppendIssue(ctx, runstore.IssueRecord{
		Severity: int(importv2.SeverityFatal), Code: string(importv2.IssueCancelled),
		Message: "import cancelled",
	}))
	return store
}

func TestLoad(t *testing.T) {
	ctx := context.Background()

	t.Run("a cross-version dir refuses to load", func(t *testing.T) {
		// given — live since the v2 bump: only the frozen compensation
		// core is promised across versions, and Load rehydrates far more
		// than the core. The belt behind the sweep's resumable() gate: any
		// caller gets the refusal, and the dir routes to compensate-only.
		store := interruptedRun(t)
		db, err := anystore.Open(ctx, filepath.Join(store.Dir(), "run.db"), nil)
		require.NoError(t, err)
		coll, err := db.Collection(ctx, "manifest")
		require.NoError(t, err)
		_, err = coll.UpsertId(ctx, "manifest", query.ModifyFunc(
			func(a *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
				v.Set("schemaVersion", a.NewNumberInt(runstore.SchemaVersion-1))
				return v, true, nil
			}))
		require.NoError(t, err)
		require.NoError(t, db.Close())

		// when
		_, err = Load(ctx, store)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be resumed")
	})

	t.Run("the restart seed is a pure function of the ledger", func(t *testing.T) {
		// given
		store := interruptedRun(t)

		// when
		state, err := Load(ctx, store)

		// then — skip set: terminal rows and done files; the unfinished
		// page heals; the derived definition is skip-listed but the sink
		// exempts it by type
		require.NoError(t, err)
		assert.Equal(t, map[string]struct{}{
			"page-1": {}, "page-3": {}, "rel-1": {}, "img.png": {},
		}, state.Engine.SkipKeys)
		assert.True(t, state.Heal()("page-2", false), "the unfinished minted create heals on ErrTreeExists")
		assert.False(t, state.Heal()("page-2", true), "minted proof never heals a derived-class collision (class guard)")
		assert.False(t, state.Heal()("page-3", false), "a matched row never heals")
		assert.False(t, state.Heal()("page-1", false), "a terminal row is skipped, not healed")

		// and — finalize inference: the collection and report are reused,
		// the interrupted finalize claim is dropped
		assert.Equal(t, "obj-root", state.Engine.RootCollectionId)
		assert.Equal(t, "obj-report", state.Engine.ReportObjectId)

		// and — the numbers resume: created counts the page, the
		// derived definition and the file; finalize outputs are excluded
		// exactly as the live counters exclude them
		assert.Equal(t, int64(3), state.Engine.Created)
		assert.Equal(t, int64(1), state.Engine.Updated)
		assert.Equal(t, 4, state.SpoolCount)
		assert.Equal(t, importv2.RootSpec{CollectionName: "Import"}, state.Engine.RootSpec)
		assert.Equal(t, "Markdown", state.Engine.ConverterName)

		// and — issues carry over minus the abort record
		require.Len(t, state.Engine.Issues, 1)
		assert.Equal(t, importv2.IssueDataLoss, state.Engine.Issues[0].Code)
	})

	t.Run("the identity option rehydrates exactly the stream rows", func(t *testing.T) {
		// given
		store := interruptedRun(t)
		state, err := Load(ctx, store)
		require.NoError(t, err)

		// when
		objectStore := objectstore.NewStoreFixture(t)
		space := mock_clientspace.NewMockSpace(t) // no expectations: nothing mints
		service := identity.NewService(space, objectStore.SpaceIndex("space-1"), false,
			time.Unix(1700000000, 0), state.IdentityOption())

		// then: stream rows resolve; the unfinished one is pending; the
		// dropped finalize claims are absent
		id, ok := service.Resolve("page-1")
		assert.True(t, ok)
		assert.Equal(t, "obj-1", id)
		fileId, err := service.ResolveFile(ctx, "img.png")
		require.NoError(t, err)
		assert.Equal(t, "file-1", fileId)
		assert.Equal(t, []string{"page-2"}, service.UnassignedClaims())
		_, ok = service.Resolve("collection:Interrupted")
		assert.False(t, ok, "an interrupted finalize claim must not rehydrate")
	})
}
