package runstore

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaims(t *testing.T) {
	ctx := context.Background()

	t.Run("a claim batch writes entries and payload rows in one shot", func(t *testing.T) {
		// given
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))

		// when
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "page-1", ObjectId: "obj-1", PayloadRoot: []byte("root-1"), PayloadHeads: []string{"obj-1"}},
			{SourceKey: "page-2", ObjectId: "obj-2", Matched: true}, // dedup match: no payload
		}))

		// then: before materialization begins, claims are pure intent —
		// nothing enters the compensation view (A1)
		inputs, err := store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Empty(t, inputs.Created)
		assert.Empty(t, inputs.Updated)

		// and: once pass 3 starts, the minted claim is the crash window of a
		// possible create (deletable, not-found tolerated); matched stays out
		// of the delete set
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		inputs, err = store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1"}, inputs.Created,
			"a minted claim is attributable from claim time (write-ahead intent)")
		assert.Equal(t, []string{"obj-2"}, inputs.Updated)

		// and: the payload row exists for the minted claim only
		root, heads, err := store.readPayloadForTest(ctx, "obj-1")
		require.NoError(t, err)
		assert.Equal(t, []byte("root-1"), root)
		assert.Equal(t, []string{"obj-1"}, heads)
		_, _, err = store.readPayloadForTest(ctx, "obj-2")
		require.Error(t, err)
	})

	t.Run("an effect over a claimed row flips status but keeps mode, rank and id", func(t *testing.T) {
		// given — claims are the write-ahead intent; the later effect must
		// mark completion without disturbing what compensation reads.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "page-1", ObjectId: "obj-1", PayloadRoot: []byte("r")},
		}))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "page-2", ObjectId: "obj-2", PayloadRoot: []byte("r")},
		}))

		// when: the second claim's object persists first
		require.NoError(t, store.RecordCreated(ctx, "page-2", "obj-2"))
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))

		// then: delete order still follows claim rank (first write), and
		// both rows read persisted
		inputs, err := store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-2", "obj-1"}, inputs.Created)
		assert.Equal(t, "persisted", store.readEntryStatusForTest(t, ctx, "page-1"))
		assert.Equal(t, "persisted", store.readEntryStatusForTest(t, ctx, "page-2"))
	})
}

func TestClaimMergeRules(t *testing.T) {
	ctx := context.Background()

	t.Run("a claim over a persisted effect row never downgrades it", func(t *testing.T) {
		// given — E1 (CONFIRMED, latent until DM-2 re-records claims): the
		// blind upsert let a later claim erase a run-created id.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordCreated(ctx, "k", "obj-minted"))
		require.NoError(t, store.SetState(ctx, StateMaterializing))

		// when: a claim arrives for the same key with the same id
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{{SourceKey: "k", ObjectId: "obj-minted"}}))

		// then: the effect survives — still persisted, still deletable
		inputs, err := store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-minted"}, inputs.Created)
		assert.Equal(t, "persisted", store.readEntryStatusForTest(t, ctx, "k"))
	})

	t.Run("a claim with a DIFFERENT id preserves the displaced id", func(t *testing.T) {
		// given
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordCreated(ctx, "k", "obj-a"))
		require.NoError(t, store.SetState(ctx, StateMaterializing))

		// when
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{{SourceKey: "k", ObjectId: "obj-b", PayloadRoot: []byte("r")}}))

		// then: neither id vanished from the ledger
		inputs, err := store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Contains(t, inputs.Created, "obj-a")
	})
}

func TestDisplacedStatusCarries(t *testing.T) {
	t.Run("a displaced pass-1 claim stays behind the materialize gate", func(t *testing.T) {
		// given — found by all three reviewers: recordSyntheticEntry
		// stamped statusPersisted unconditionally, so a displaced CLAIM
		// bypassed the MaterializeStarted gate and was deletable for an
		// object that never existed.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{{SourceKey: "k", ObjectId: "obj-a", PayloadRoot: []byte("r")}}))

		// when: a conflicting claim displaces obj-b into a synthetic row,
		// still BEFORE materialization
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{{SourceKey: "k", ObjectId: "obj-b", PayloadRoot: []byte("r")}}))
		inputs, err := store.CompensationInputs(ctx)

		// then: nothing is deletable yet — both ids are pure intent
		require.NoError(t, err)
		assert.Empty(t, inputs.Created, "a displaced claim must carry the claimed status, not persisted")

		// and: after the gate opens, both ids join the delete set
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		inputs, err = store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"obj-a", "obj-b"}, inputs.Created)
	})

	t.Run("a same-id occupant never has its mode flipped", func(t *testing.T) {
		// given — the occupancy probe keyed on objectId alone: a same-id
		// minted displacement could overwrite a matched synthetic row,
		// flipping a never-deletable id into the delete set.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		// a matched synthetic row for obj-x under key "k"
		require.NoError(t, store.RecordCreated(ctx, "k", "obj-keep"))
		require.NoError(t, store.RecordUpdated(ctx, "k", "obj-x")) // displaced matched
		// a minted displacement of the SAME id (identity violation upstream)
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{{SourceKey: "k#dup-obj-x", ObjectId: "obj-y", PayloadRoot: []byte("r")}}))
		require.NoError(t, store.RecordCreated(ctx, "k#dup-obj-x", "obj-x"))

		// when
		inputs, err := store.CompensationInputs(ctx)

		// then: obj-x's matched identity survives somewhere in the ledger
		require.NoError(t, err)
		assert.Contains(t, inputs.Updated, "obj-x",
			"a matched (pre-existing, never-deletable) id must never flip to minted")
	})
}

func TestFileDisplacedWrite(t *testing.T) {
	t.Run("a displaced file id cannot clobber a real files row", func(t *testing.T) {
		// given — confirmed by two reviewers: RecordFile's displaced write
		// was a blind UpsertOne; markdown file source keys are raw archive
		// entry names (attacker-shaped), exactly why entries was hardened.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		// a REAL file whose source key has the synthetic shape
		require.NoError(t, store.RecordFile(ctx, "f#dup-file-b", "file-real", false))
		// key "f" records file-a, then a conflicting re-record displaces file-b
		require.NoError(t, store.RecordFile(ctx, "f", "file-a", false))
		require.NoError(t, store.RecordFile(ctx, "f", "file-b", false))

		// when
		inputs, err := store.CompensationInputs(ctx)

		// then: all three run-owned file ids stay in the delete set
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"file-real", "file-a", "file-b"}, inputs.OwnedFiles)
	})
}

func TestPayloadOccupancy(t *testing.T) {
	ctx := context.Background()

	t.Run("a differing payload re-record never replaces the first", func(t *testing.T) {
		// given — §9.1 item 1: the payload row is the write-ahead create
		// payload and the id IS the hash of the root bytes, so a different
		// root under one id is an identity violation upstream. The blind
		// UpsertOne made it last-writer-wins — the wrong default for the row
		// a pass-3 restart mints nothing without.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "k1", ObjectId: "obj-1", PayloadRoot: []byte("root-first"), PayloadHeads: []string{"obj-1"}},
		}))

		// when: a conflicting claim under another source key carries the
		// same minted id with different payload bytes
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{
			{SourceKey: "k2", ObjectId: "obj-1", PayloadRoot: []byte("root-second"), PayloadHeads: []string{"other"}},
		}))

		// then: first record wins entirely
		root, heads, err := store.readPayloadForTest(ctx, "obj-1")
		require.NoError(t, err)
		assert.Equal(t, []byte("root-first"), root)
		assert.Equal(t, []string{"obj-1"}, heads)
	})

	t.Run("an identical payload re-record is a no-op", func(t *testing.T) {
		// given
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		claim := ClaimRecord{SourceKey: "k1", ObjectId: "obj-1", PayloadRoot: []byte("root"), PayloadHeads: []string{"obj-1"}}
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{claim}))

		// when
		claim.SourceKey = "k2"
		require.NoError(t, store.RecordClaims(ctx, []ClaimRecord{claim}))

		// then
		root, heads, err := store.readPayloadForTest(ctx, "obj-1")
		require.NoError(t, err)
		assert.Equal(t, []byte("root"), root)
		assert.Equal(t, []string{"obj-1"}, heads)
	})
}

func TestDisplacedRankFrozen(t *testing.T) {
	t.Run("re-displacing the same entry id does not reorder the delete set", func(t *testing.T) {
		// given — the last instance of the frozen-rank defect class:
		// recordEntry freezes rank at the row's first write (compensation
		// ordering depends on it), but the placeRow write callbacks stamped
		// it unconditionally, so an idempotent re-displacement re-ranked the
		// row and reordered compensation. Creation order A, B, Z must come
		// back newest-first as Z, B, A whatever repeats itself.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		require.NoError(t, store.RecordCreated(ctx, "k", "A")) // keeps row "k"
		require.NoError(t, store.RecordCreated(ctx, "k", "B")) // displaced → synthetic row
		require.NoError(t, store.RecordCreated(ctx, "k", "Z")) // displaced → synthetic row

		// when: B's displacement repeats (identical row — placeRow's
		// idempotent branch)
		require.NoError(t, store.RecordCreated(ctx, "k", "B"))

		// then: first-write order still decides deletion order
		inputs, err := store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"Z", "B", "A"}, inputs.Created,
			"rank is frozen at first write; a re-displacement must not re-stamp it")
	})

	t.Run("re-displacing the same file id does not reorder the delete set", func(t *testing.T) {
		// given — the sibling site: RecordFile's displaced write callback
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordFile(ctx, "f", "file-a", false))
		require.NoError(t, store.RecordFile(ctx, "f", "file-b", false)) // displaced
		require.NoError(t, store.RecordFile(ctx, "f", "file-c", false)) // displaced

		// when
		require.NoError(t, store.RecordFile(ctx, "f", "file-b", false)) // idempotent re-record

		// then
		inputs, err := store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"file-c", "file-b", "file-a"}, inputs.OwnedFiles)
	})
}

func TestSyntheticKeyCollision(t *testing.T) {
	t.Run("a source key shaped like a synthetic key cannot collide", func(t *testing.T) {
		// given — E5: filenames can contain '#'; the old "#dup-" suffix let
		// a legitimate key overwrite a synthetic preservation row.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.SetState(ctx, StateMaterializing))
		// a REAL source key that happens to have the synthetic shape...
		require.NoError(t, store.RecordCreated(ctx, "k#dup-obj-b", "obj-c"))
		// ...then key "k" displaces obj-b: the synthetic write must not
		// clobber the real row
		require.NoError(t, store.RecordCreated(ctx, "k", "obj-a"))
		require.NoError(t, store.RecordUpdated(ctx, "k", "obj-b"))
		inputs, err := store.CompensationInputs(ctx)

		// then: all three ids are present
		require.NoError(t, err)
		assert.Contains(t, inputs.Created, "obj-a")
		assert.Contains(t, inputs.Created, "obj-c")
		assert.Contains(t, inputs.Updated, "obj-b")
	})
}

func TestIssues(t *testing.T) {
	t.Run("the issue sequence survives a reopen", func(t *testing.T) {
		// given — the sibling divergence: rank and the spool seq were
		// seeded on open, issueSeq still restarted at 0 (CONFIRMED: 3 rows
		// + reopen + 2 appends left 3 rows, silently overwritten).
		ctx := context.Background()
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		for i := 0; i < 3; i++ {
			require.NoError(t, store.AppendIssue(ctx, IssueRecord{Code: fmt.Sprintf("first-%d", i)}))
		}
		require.NoError(t, store.Close())

		// when
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()
		require.NoError(t, reopened.AppendIssue(ctx, IssueRecord{Code: "second-0"}))
		require.NoError(t, reopened.AppendIssue(ctx, IssueRecord{Code: "second-1"}))
		records, err := reopened.ReadIssues(ctx)

		// then
		require.NoError(t, err)
		require.Len(t, records, 5, "a reopened issue ledger must append, not overwrite")
		assert.Equal(t, "second-1", records[4].Code)
	})

	t.Run("issues append durably in order", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))

		// when
		require.NoError(t, store.AppendIssue(ctx, IssueRecord{
			Severity: 1, Code: "missingTarget", SourceKey: "page-1", Message: "gone",
		}))
		require.NoError(t, store.AppendIssue(ctx, IssueRecord{
			Severity: 2, Code: "objectFailed", ObjectId: "obj-2", Error: "boom",
		}))

		// then
		records, err := store.ReadIssues(ctx)
		require.NoError(t, err)
		require.Len(t, records, 2)
		assert.Equal(t, "missingTarget", records[0].Code)
		assert.Equal(t, "page-1", records[0].SourceKey)
		assert.Equal(t, "objectFailed", records[1].Code)
		assert.Equal(t, "boom", records[1].Error)
	})
}

// test-only raw readers

func (s *Store) readPayloadForTest(ctx context.Context, objectId string) (root []byte, heads []string, err error) {
	coll, err := s.db.Collection(ctx, "payloads")
	if err != nil {
		return nil, nil, err
	}
	doc, err := coll.FindId(ctx, objectId)
	if err != nil {
		return nil, nil, err
	}
	root = doc.Value().GetBytes("root")
	for _, head := range doc.Value().GetArray("heads") {
		heads = append(heads, string(head.GetStringBytes()))
	}
	return root, heads, nil
}

func (s *Store) readEntryStatusForTest(t *testing.T, ctx context.Context, sourceKey string) string {
	t.Helper()
	doc, err := s.entries.FindId(ctx, sourceKey)
	require.NoError(t, err)
	return string(doc.Value().GetStringBytes("status"))
}

// silence the unused-import guard if anystore types stop being referenced
var _ = anystore.ErrDocNotFound
