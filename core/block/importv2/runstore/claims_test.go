package runstore

import (
	"context"
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
