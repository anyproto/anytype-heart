package enginetest

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// The DM-2 equivalence gate (spec §9): kill a run during pass 3 at the
// create, upload and finalize boundaries; resume it from the run dir alone
// (no source, no network — ResumeDurable never sees the markdown tree);
// assert the final object set is IDENTICAL to an uninterrupted run. The
// "kill" is a suspend-shaped stop with no settlement: the engine stops
// without compensating and nothing marks the manifest, leaving exactly
// what a killed process leaves — manifest at materializing, partial
// effects journaled (detached writes land), spool whole.

// crashTree is the shared source: five linked pages (c.md references the
// FIRST page — a late row resolving against a possibly-skipped early row),
// an image, and front-matter deriving a relation and a type.
func crashTree(t *testing.T) string {
	return writeTree(t, map[string]string{
		"index.md":       "---\nAuthor: Roman\ntype: Zettel\n---\n# Home\n\nSee [A](notes/a.md) and ![pic](assets/pic.png)\n",
		"notes/a.md":     "# A\n\nNext: [B](b.md)\n",
		"notes/b.md":     "# B\n",
		"notes/c.md":     "# C\n\nBack to [Home](../index.md)\n",
		"notes/d.md":     "# D\n",
		"assets/pic.png": "png-bytes",
	})
}

// runControl produces the uninterrupted reference run over the same tree.
func runControl(t *testing.T, root string) (*Fixture, *importv2.Result) {
	t.Helper()
	fx := NewFixture(t)
	dir := filepath.Join(t.TempDir(), "run-control")
	result := fx.RunMarkdownDurable(context.Background(), t, root, request(false, false), dir)
	require.NoError(t, result.Err)
	return fx, result
}

// interrupt runs one incarnation with the given hooks armed, expecting the
// suspend-shaped stop, and disarms the hooks for the resume.
func interrupt(t *testing.T, fx *Fixture, root, dir string, arm func(cancel context.CancelCauseFunc)) *importv2.Result {
	t.Helper()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	arm(cancel)
	result := fx.RunMarkdownDurable(ctx, t, root, request(false, false), dir)
	fx.Space.BeforeCreate = nil
	fx.Space.AfterCreate = nil
	fx.Uploader.BeforeUpload = nil
	return result
}

// assertResumedClean is the shared postcondition: the resumed incarnation
// succeeded, reported nothing (no reconcile noise, no rehydrated abort
// records), and its counters continue the ledger's.
func assertResumedClean(t *testing.T, resumed, control *importv2.Result) {
	t.Helper()
	require.NoError(t, resumed.Err)
	assert.False(t, resumed.Suspended)
	assert.Empty(t, resumed.Issues, "a resumed run must not invent issues")
	assert.Zero(t, resumed.Failed)
	assert.Equal(t, control.Created, resumed.Created, "counters must resume, not restart")
	assert.Equal(t, control.Updated, resumed.Updated)
}

func TestCrashResumeMidCreate(t *testing.T) {
	t.Run("killed before a create: the resumed run converges byte-identically", func(t *testing.T) {
		// given: a control run and a run killed at its third tree create
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		var creates atomic.Int32
		inc1 := interrupt(t, fx, root, dir, func(cancel context.CancelCauseFunc) {
			fx.Space.BeforeCreate = func(id string) error {
				if creates.Add(1) == 3 {
					cancel(importv2.ErrSuspended)
					return context.Canceled
				}
				return nil
			}
		})
		require.Error(t, inc1.Err)
		require.True(t, inc1.Suspended, "the stop must be the suspend shape, not an abort")
		require.Less(t, len(fx.Space.Created), len(control.Space.Created),
			"the kill must land mid-materialize or the test proves nothing")

		// when: resumed from the dir alone
		resumed := fx.ResumeDurable(context.Background(), t, dir, request(false, false))

		// then
		assertResumedClean(t, resumed, controlResult)
		assert.Equal(t, control.Dump(), fx.Dump(),
			"the final object set must be identical to the uninterrupted run")
	})
}

func TestResumedCancelCompensatesEveryIncarnation(t *testing.T) {
	t.Run("cancel on a resumed run removes ALL incarnations' objects", func(t *testing.T) {
		// given — the review's Class A: the engine compensates from the
		// in-memory journal, and a resumed incarnation's fresh journal knew
		// nothing about the crash's ledger. A user pressing Cancel on the
		// auto-resumed import at app launch is the ordinary trigger; the
		// wire advertises CancelEffect = RemovesCreated on exactly these
		// runs, so a partial undo is a broken promise plus orphans.
		root := crashTree(t)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		var creates atomic.Int32
		inc1 := interrupt(t, fx, root, dir, func(cancel context.CancelCauseFunc) {
			fx.Space.BeforeCreate = func(id string) error {
				if creates.Add(1) == 3 {
					cancel(importv2.ErrSuspended)
					return context.Canceled
				}
				return nil
			}
		})
		require.Error(t, inc1.Err)
		require.True(t, inc1.Suspended)
		fx.Space.mu.Lock()
		leftBehind := len(fx.Space.Created)
		fx.Space.mu.Unlock()
		require.Positive(t, leftBehind, "incarnation 1 must have created something to orphan")

		// when: the resumed incarnation is cancelled mid-flight with a
		// PLAIN cause (user cancel — the engine compensates)
		ctx, cancel := context.WithCancelCause(context.Background())
		defer cancel(nil)
		var resumeCreates atomic.Int32
		fx.Space.BeforeCreate = func(id string) error {
			if resumeCreates.Add(1) == 2 {
				cancel(nil)
				return context.Canceled
			}
			return nil
		}
		resumed := fx.ResumeDurable(ctx, t, dir, request(false, false))
		fx.Space.BeforeCreate = nil

		// then: the cancel undoes EVERYTHING the run ever created, across
		// incarnations, and reports it truthfully
		require.Error(t, resumed.Err)
		assert.False(t, resumed.Suspended)
		fx.Space.mu.Lock()
		remaining := len(fx.Space.Created)
		fx.Space.mu.Unlock()
		assert.Zero(t, remaining,
			"cancel on a resumed run must remove every incarnation's objects, not only its own")
		assert.Zero(t, resumed.Leaked)
		assert.GreaterOrEqual(t, int64(resumed.Compensated), int64(leftBehind),
			"the compensation count must cover the previous incarnation's objects")
	})
}

func TestCrashResumeTornCreate(t *testing.T) {
	t.Run("killed between the tree write and its effect row: the heal repairs it", func(t *testing.T) {
		// given: a run killed right after page B's tree write. The kill's
		// effect on the ledger — the detached effect write never landed — is
		// reproduced by rewinding B's row to its pre-effect state (claimed),
		// and the possibly-hollow tree by blanking B's recorded state.
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		var tornId atomic.Value
		inc1 := interrupt(t, fx, root, dir, func(cancel context.CancelCauseFunc) {
			fx.Space.AfterCreate = func(id string) {
				fx.Space.mu.Lock()
				name := fx.Space.Created[id].CombinedDetails().GetString(bundle.RelationKeyName)
				fx.Space.mu.Unlock()
				if name == "B" {
					tornId.Store(id)
					cancel(importv2.ErrSuspended)
				}
			}
		})
		require.Error(t, inc1.Err)
		require.True(t, inc1.Suspended)
		id, ok := tornId.Load().(string)
		require.True(t, ok, "page B must have been created before the kill")
		rewindEntryToClaimed(t, dir, id)
		fx.Space.mu.Lock()
		fx.Space.Created[id] = hollowState(id)
		fx.Space.mu.Unlock()

		// when
		resumed := fx.ResumeDurable(context.Background(), t, dir, request(false, false))

		// then: the hollow tree carries the full imported state again
		assertResumedClean(t, resumed, controlResult)
		assert.Equal(t, control.Dump(), fx.Dump(),
			"the heal must leave the object set identical to the uninterrupted run")
	})
}

func TestCrashResumeAtUpload(t *testing.T) {
	t.Run("killed at the upload: the resumed run re-uploads and converges", func(t *testing.T) {
		// given
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interrupt(t, fx, root, dir, func(cancel context.CancelCauseFunc) {
			fx.Uploader.BeforeUpload = func(string) error {
				cancel(importv2.ErrSuspended)
				return context.Canceled
			}
		})
		require.Error(t, inc1.Err)
		require.True(t, inc1.Suspended)
		require.Empty(t, fx.Uploader.Uploads, "the kill must land before the upload recorded")

		// when
		resumed := fx.ResumeDurable(context.Background(), t, dir, request(false, false))

		// then: uploaded exactly once, everything else identical
		assertResumedClean(t, resumed, controlResult)
		assert.Equal(t, control.Dump(), fx.Dump())
	})
}

// killInsideFinalizeCreate arms a kill that fires INSIDE finalize's own
// tree create (the streamCreates+1-th create attempt): the collection is
// CLAIMED — a late, non-terminal row — but never created. The review's
// Class B found the earlier version of this boundary (kill after the last
// stream create) never reached finalize at all: the post-stream guard
// tripped first and the ledger held no collection row, so the
// Late-non-terminal drop rule it claimed to pin was untested.
func killInsideFinalizeCreate(fx *Fixture, streamCreates int32, cancel context.CancelCauseFunc) {
	var creates atomic.Int32
	fx.Space.BeforeCreate = func(id string) error {
		if creates.Add(1) == streamCreates+1 {
			cancel(importv2.ErrSuspended)
			return context.Canceled
		}
		return nil
	}
}

func TestCrashResumeAtFinalize(t *testing.T) {
	t.Run("killed inside finalize's create: the resumed run builds ONE collection", func(t *testing.T) {
		// given: the interrupted finalize claim is a late non-terminal row
		// under the SAME source key the resumed finalize re-claims (the
		// stub factory's name is stable — and the adapter's date suffix has
		// minute granularity, so a fast crash-restart lands on the same key
		// there too): the re-claim displaces the abandoned id.
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		streamCreates := int32(len(control.Space.Created) - 1) // minus the collection
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interrupt(t, fx, root, dir, func(cancel context.CancelCauseFunc) {
			killInsideFinalizeCreate(fx, streamCreates, cancel)
		})
		require.Error(t, inc1.Err)
		require.True(t, inc1.Suspended)
		require.Len(t, fx.Space.Created, int(streamCreates), "the collection must not exist yet")

		// when
		resumed := fx.ResumeDurable(context.Background(), t, dir, request(false, false))

		// then
		assertResumedClean(t, resumed, controlResult)
		assert.Equal(t, 1, countCollections(fx), "exactly one root collection despite the re-claim")
		assert.Equal(t, normalizeDump(control.Dump()), normalizeDump(fx.Dump()),
			"the object set must be identical up to the re-minted collection id")
		assert.NotEmpty(t, resumed.RootCollectionId)
	})

	t.Run("killed inside finalize TWICE: displaced claims never become phantoms", func(t *testing.T) {
		// given — the review's executed Class B repro: two crashes across
		// finalize under one collection key. The second incarnation's
		// re-claim displaces the first's abandoned id into a synthetic row;
		// incarnation 3 must not read that synthetic as a stream row (a
		// phantom 'claimed object was never emitted' on every resume, and a
		// second collection).
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		streamCreates := int32(len(control.Space.Created) - 1)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interrupt(t, fx, root, dir, func(cancel context.CancelCauseFunc) {
			killInsideFinalizeCreate(fx, streamCreates, cancel)
		})
		require.True(t, inc1.Suspended)

		// incarnation 2: same kill, one create later (the collection create
		// is now the FIRST create — everything else replays skipped)
		ctx2, cancel2 := context.WithCancelCause(context.Background())
		defer cancel2(nil)
		fx.Space.BeforeCreate = func(id string) error {
			cancel2(importv2.ErrSuspended)
			return context.Canceled
		}
		inc2 := fx.ResumeDurable(ctx2, t, dir, request(false, false))
		fx.Space.BeforeCreate = nil
		require.Error(t, inc2.Err)
		require.True(t, inc2.Suspended)

		// when: the third incarnation runs to completion
		resumed := fx.ResumeDurable(context.Background(), t, dir, request(false, false))

		// then
		assertResumedClean(t, resumed, controlResult)
		assert.Equal(t, 1, countCollections(fx),
			"two abandoned finalize claims must yield ONE collection, not three")
		assert.Equal(t, normalizeDump(control.Dump()), normalizeDump(fx.Dump()))
	})
}

func TestCrashResumeAfterFinalize(t *testing.T) {
	t.Run("killed after finalize, before disposal: the resumed run changes nothing", func(t *testing.T) {
		// given: the kill lands right after the root collection persisted —
		// its effect row is durable (detached write), its claim flushes at
		// finish, but the run never reached the success gate, so the dir is
		// left mid-materialize with everything actually done. The resumed
		// incarnation must reuse the recorded collection instead of minting
		// a second one.
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interrupt(t, fx, root, dir, func(cancel context.CancelCauseFunc) {
			fx.Space.AfterCreate = func(id string) {
				fx.Space.mu.Lock()
				isCollection := len(fx.Space.Created[id].GetStoreSlice(template.CollectionStoreKey)) > 0
				fx.Space.mu.Unlock()
				if isCollection {
					cancel(importv2.ErrSuspended)
				}
			}
		})
		require.Error(t, inc1.Err)
		require.True(t, inc1.Suspended)
		require.Equal(t, 1, countCollections(fx), "the collection must exist before the resume")

		// when
		resumed := fx.ResumeDurable(context.Background(), t, dir, request(false, false))

		// then: byte-identical, ids included — nothing was re-minted
		assertResumedClean(t, resumed, controlResult)
		assert.Equal(t, 1, countCollections(fx))
		assert.Equal(t, control.Dump(), fx.Dump())
		assert.Equal(t, controlResult.RootCollectionId, resumed.RootCollectionId,
			"the recorded collection must be reused, not rebuilt")
	})
}

// --- helpers ---

// rewindEntryToClaimed reproduces the torn-crash ledger image: the row's
// effect write (status persisted + action) never landed, so the row reads
// exactly as the claim left it. Raw surgery on the closed run.db — the
// production writers have no "unwrite" and must not grow one for a test.
func rewindEntryToClaimed(t *testing.T, dir, objectId string) {
	t.Helper()
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(dir, "run.db"), nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	coll, err := db.Collection(ctx, "entries")
	require.NoError(t, err)
	iter, err := coll.Find(fmt.Sprintf(`{"objectId":%q}`, objectId)).Iter(ctx)
	require.NoError(t, err)
	var rowId string
	for iter.Next() {
		doc, err := iter.Doc()
		require.NoError(t, err)
		rowId = string(doc.Value().GetStringBytes("id"))
	}
	require.NoError(t, iter.Close())
	require.NotEmpty(t, rowId, "the effect row to rewind must exist")
	_, err = coll.UpsertId(ctx, rowId, query.ModifyFunc(
		func(a *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
			v.Set("status", a.NewString("claimed"))
			v.Del("action")
			return v, true, nil
		}))
	require.NoError(t, err)
}

// hollowState models the tree a torn create leaves: the root exists, the
// imported state never applied.
func hollowState(id string) *state.State {
	return state.NewDoc(id, nil).(*state.State)
}

func countCollections(fx *Fixture) int {
	fx.Space.mu.Lock()
	defer fx.Space.mu.Unlock()
	count := 0
	for _, st := range fx.Space.Created {
		if st != nil && len(st.GetStoreSlice(template.CollectionStoreKey)) > 0 {
			count++
		}
	}
	return count
}

// normalizeDump replaces object ids with object names (ids, members, link
// and file targets) so runs whose finalize re-minted the collection id
// compare on content. Names are unique in the crash fixtures.
func normalizeDump(d Dump) Dump {
	nameById := map[string]string{}
	for _, object := range d.Objects {
		if name, ok := object.Details["name"].(string); ok && name != "" {
			nameById[object.Id] = name
		}
	}
	mapId := func(id string) string {
		if name, ok := nameById[id]; ok {
			return name
		}
		return id
	}
	out := Dump{Uploads: d.Uploads}
	for _, object := range d.Objects {
		normalized := object
		normalized.Id = mapId(object.Id)
		if len(object.Members) > 0 {
			normalized.Members = make([]string, 0, len(object.Members))
			for _, member := range object.Members {
				normalized.Members = append(normalized.Members, mapId(member))
			}
		}
		normalized.Blocks = normalizeBlocks(object.Blocks, mapId)
		out.Objects = append(out.Objects, normalized)
	}
	sort.Slice(out.Objects, func(a, b int) bool { return out.Objects[a].Id < out.Objects[b].Id })
	return out
}

func normalizeBlocks(blocks []BlockDump, mapId func(string) string) []BlockDump {
	normalized := make([]BlockDump, 0, len(blocks))
	for _, block := range blocks {
		block.Target = mapId(block.Target)
		block.Children = normalizeBlocks(block.Children, mapId)
		normalized = append(normalized, block)
	}
	return normalized
}
