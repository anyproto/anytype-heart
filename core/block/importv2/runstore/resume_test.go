package runstore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The dormant-run readers: everything a pass-3 restart (DM spec §8.1) and
// the §15 pull RPCs consume from a run dir with no live engine.

func TestReadEntries(t *testing.T) {
	ctx := context.Background()

	t.Run("entries read back with payloads joined and terminality decided", func(t *testing.T) {
		// given: a minted claim (payload retained), a matched claim, and a
		// minted claim whose object persisted
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "pending", ObjectId: "obj-1", PayloadRoot: []byte("root-1"), PayloadHeads: []string{"obj-1"}},
			{SourceKey: "existing", ObjectId: "obj-2", Matched: true},
			{SourceKey: "done", ObjectId: "obj-3", PayloadRoot: []byte("root-3"), PayloadHeads: []string{"obj-3"}},
		}))
		require.NoError(t, store.RecordCreated(ctx, "done", "obj-3"))

		// when
		records, err := store.ReadEntries(ctx)

		// then
		require.NoError(t, err)
		byKey := map[string]EntryRecord{}
		for _, record := range records {
			byKey[record.SourceKey] = record
		}
		require.Len(t, byKey, 3)
		pending := byKey["pending"]
		assert.Equal(t, "obj-1", pending.ObjectId)
		assert.False(t, pending.Matched)
		assert.False(t, pending.Terminal, "a claimed row is not terminal")
		assert.Equal(t, []byte("root-1"), pending.PayloadRoot, "the retained payload joins the entry")
		assert.Equal(t, []string{"obj-1"}, pending.PayloadHeads)
		existing := byKey["existing"]
		assert.True(t, existing.Matched)
		assert.Nil(t, existing.PayloadRoot, "matched claims carry no payload")
		done := byKey["done"]
		assert.True(t, done.Terminal, "a persisted row is terminal")
		assert.Equal(t, "created", done.Action)
	})

	t.Run("claims recorded after materialize began read back late", func(t *testing.T) {
		// given — the late marker is what lets a restart tell a converter
		// claim (must reconcile) from a finalize-stage claim (re-claimed
		// fresh on resume): pass-1/2 claims precede the materializing state,
		// finalize claims follow it.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "crawled", ObjectId: "obj-1", PayloadRoot: []byte("r")},
		}))
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "collection:Import", ObjectId: "obj-2", PayloadRoot: []byte("r")},
		}))

		// when
		records, err := store.ReadEntries(ctx)

		// then
		require.NoError(t, err)
		byKey := map[string]EntryRecord{}
		for _, record := range records {
			byKey[record.SourceKey] = record
		}
		assert.False(t, byKey["crawled"].Late)
		assert.True(t, byKey["collection:Import"].Late)
	})

	t.Run("a fresh effect row during pass 3 is late; a claimed row's effect is not", func(t *testing.T) {
		// given — the finalize-stage objects (root collection, report page)
		// write their EFFECT row before their buffered claim flushes, so the
		// late stamp must live on recordEntry's fresh branch too, not only
		// on RecordClaims — or the restart's finalize inference reads them
		// as ordinary stream rows.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "crawled", ObjectId: "obj-1", PayloadRoot: []byte("r")},
		}))
		require.NoError(t, store.SetState(ctx, StateMaterializing))

		// when: the stream object's effect merges into its claim row; the
		// finalize object's effect writes a fresh row
		require.NoError(t, store.RecordCreated(ctx, "crawled", "obj-1"))
		require.NoError(t, store.RecordCreated(ctx, "collection:Import", "obj-2"))

		// then
		records, err := store.ReadEntries(ctx)
		require.NoError(t, err)
		byKey := map[string]EntryRecord{}
		for _, record := range records {
			byKey[record.SourceKey] = record
		}
		assert.False(t, byKey["crawled"].Late)
		assert.True(t, byKey["collection:Import"].Late)
	})

	t.Run("displacement carries classification and marks the row synthetic", func(t *testing.T) {
		// given — review Class B: the finalize inference keys on Late, and
		// reconciliation must never see a synthetic row (it can have no
		// spool row by construction). The displaced-row writer dropped BOTH
		// facts: a re-claimed finalize key (same minute, same name) produced
		// an unmarked synthetic that read as an ordinary stream row.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "collection:Import", ObjectId: "obj-a", PayloadRoot: []byte("r")},
		}))

		// when: the resumed finalize re-claims the SAME key with a new id
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "collection:Import", ObjectId: "obj-b", PayloadRoot: []byte("r")},
		}))

		// then: the displaced id's synthetic row is marked synthetic and
		// keeps the late classification of its origin
		records, err := store.ReadEntries(ctx)
		require.NoError(t, err)
		byKey := map[string]EntryRecord{}
		for _, record := range records {
			byKey[record.SourceKey] = record
		}
		require.Len(t, byKey, 2)
		primary := byKey["collection:Import"]
		assert.True(t, primary.Late)
		assert.False(t, primary.Synthetic)
		var synthetic *EntryRecord
		for key, record := range byKey {
			if key != "collection:Import" {
				record := record
				synthetic = &record
			}
		}
		require.NotNil(t, synthetic)
		assert.True(t, synthetic.Synthetic, "a displaced row must be marked synthetic")
		assert.True(t, synthetic.Late, "the late classification must survive displacement")
	})

	t.Run("an effect displacement keeps the displaced row's own late marker", func(t *testing.T) {
		// given: a pre-materialize (non-late) matched effect, displaced by a
		// conflicting create during pass 3 — the synthetic must keep the
		// ORIGIN row's late (false), not inherit the current phase's
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordUpdated(ctx, "k", "obj-old"))
		require.NoError(t, store.SetState(ctx, StateMaterializing))

		// when
		require.NoError(t, store.RecordCreated(ctx, "k", "obj-new"))

		// then
		records, err := store.ReadEntries(ctx)
		require.NoError(t, err)
		for _, record := range records {
			if record.ObjectId == "obj-old" {
				assert.True(t, record.Synthetic)
				assert.False(t, record.Late, "the displaced row keeps ITS late, not the current phase's")
				assert.Equal(t, "updated", record.Action, "the displaced row keeps its action")
			}
		}
	})

	t.Run("the late marker survives a reopen", func(t *testing.T) {
		// given: a run that reached materializing, then a new process
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		require.NoError(t, store.Close())
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()

		// when: the reopened store records a claim (a resumed finalize)
		require.NoError(t, reopened.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "collection:Import", ObjectId: "obj-1", PayloadRoot: []byte("r")},
		}))

		// then
		records, err := reopened.ReadEntries(ctx)
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.True(t, records[0].Late, "materialize-started must be seeded from the manifest on open")
	})
}

func TestReadFiles(t *testing.T) {
	t.Run("file records read back with ownership intact", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordFile(ctx, "img.png", "file-1", false))
		require.NoError(t, store.RecordFile(ctx, "doc.pdf", "file-2", true))

		// when
		records, err := store.ReadFiles(ctx)

		// then
		require.NoError(t, err)
		byKey := map[string]FileRecord{}
		for _, record := range records {
			byKey[record.SourceKey] = record
		}
		require.Len(t, byKey, 2)
		assert.Equal(t, FileRecord{SourceKey: "img.png", ObjectId: "file-1"}, byKey["img.png"])
		assert.Equal(t, FileRecord{SourceKey: "doc.pdf", ObjectId: "file-2", PreExisting: true}, byKey["doc.pdf"])
	})
}

func TestRootSpecKV(t *testing.T) {
	ctx := context.Background()

	t.Run("an absent root spec reads back not-found", func(t *testing.T) {
		// given
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))

		// when
		_, found, err := store.ReadRootSpec(ctx)

		// then
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("the root spec round-trips through kv and survives reopen", func(t *testing.T) {
		// given — pass 2's RootSpec is a pass-2 output pass 3 consumes; a
		// restart has no converter to re-produce it (DM spec §4.1).
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		spec := importv2.RootSpec{
			CollectionName: "Notion Import",
			RootObjectKey:  "root-key",
			WidgetLayout:   model.BlockContentWidget_CompactList,
		}

		// when
		require.NoError(t, store.SetRootSpec(ctx, spec))
		require.NoError(t, store.Close())
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()
		read, found, err := reopened.ReadRootSpec(ctx)

		// then
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, spec, read)
	})
}

func TestMarkFetched(t *testing.T) {
	t.Run("the pass boundary lands root spec, fetched and materializing in order", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		spec := importv2.RootSpec{CollectionName: "Import"}

		// when
		require.NoError(t, store.MarkFetched(ctx, spec))

		// then
		manifest, err := store.Manifest(ctx)
		require.NoError(t, err)
		assert.Equal(t, StateMaterializing, manifest.State)
		assert.True(t, manifest.MaterializeStarted)
		read, found, err := store.ReadRootSpec(ctx)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, spec, read)
	})
}

func TestBeginResume(t *testing.T) {
	t.Run("begin-resume bumps incarnation and attempts durably before the attempt", func(t *testing.T) {
		// given: a fetched run a previous process left behind
		ctx := context.Background()
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		require.NoError(t, store.SetState(ctx, StateFetched))
		require.NoError(t, store.Close())
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()

		// when
		manifest, err := reopened.BeginResume(ctx)

		// then: the counters moved BEFORE any work, so a crash loop is
		// bounded by the cap however early the crash lands
		require.NoError(t, err)
		assert.Equal(t, 2, manifest.Incarnation)
		assert.Equal(t, 1, manifest.ResumeAttempts)
		assert.Equal(t, StateMaterializing, manifest.State)
		assert.True(t, manifest.MaterializeStarted)
		persisted, err := reopened.Manifest(ctx)
		require.NoError(t, err)
		assert.Equal(t, manifest.Incarnation, persisted.Incarnation)
		assert.Equal(t, manifest.ResumeAttempts, persisted.ResumeAttempts)
	})
}

func TestSpoolSourceKeys(t *testing.T) {
	t.Run("the spool's key set and count read back without decoding snapshots", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		for _, key := range []string{"page-1", "page-2"} {
			require.NoError(t, spool.Append(ctx, &importv2.Object{
				SourceKey: key,
				SbType:    coresb.SmartBlockTypePage,
				Payload:   &importv2.Snapshot{},
			}))
		}

		// when
		keys, count, err := spool.SourceKeys(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		assert.Equal(t, map[string]struct{}{"page-1": {}, "page-2": {}}, keys)
	})
}
