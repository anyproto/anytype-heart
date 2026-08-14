package runstore

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gogo/protobuf/types"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func spoolObject(key string, root bool) *importv2.Object {
	return &importv2.Object{
		SourceKey: key,
		SbType:    coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{
			Blocks: []*model.Block{{Id: "b1", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "hello " + key},
			}}},
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("Page " + key),
			}),
			ObjectTypes: []string{bundle.TypeKeyPage.String()},
			Key:         "key-" + key,
		},
		IsRootCandidate: root,
		Favorite:        true,
	}
}

func TestSpool(t *testing.T) {
	ctx := context.Background()

	t.Run("append and replay round-trip objects losslessly, in order", func(t *testing.T) {
		// given
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		first := spoolObject("a.md", true)
		second := spoolObject("b.md", false)
		second.File = &importv2.FileSource{
			Path:      "/abs/orig.png",
			Name:      "orig.png",
			URL:       "https://example.org/orig.png",
			ImageKind: model.ImageKind_Cover,
			EncryptionKeys: map[string]string{
				"path": "key-bytes",
			},
		}

		// when
		require.NoError(t, spool.Append(ctx, first))
		require.NoError(t, spool.Append(ctx, second))
		var replayed []*importv2.Object
		require.NoError(t, spool.Replay(ctx, func(o *importv2.Object) error {
			replayed = append(replayed, o)
			return nil
		}))

		// then
		require.Len(t, replayed, 2)
		assert.Equal(t, "a.md", replayed[0].SourceKey)
		assert.Equal(t, coresb.SmartBlockTypePage, replayed[0].SbType)
		assert.True(t, replayed[0].IsRootCandidate)
		assert.True(t, replayed[0].Favorite)
		assert.Equal(t, "key-a.md", replayed[0].Payload.Key)
		assert.Equal(t, "Page a.md", replayed[0].Payload.Details.GetString(bundle.RelationKeyName))
		require.Len(t, replayed[0].Payload.Blocks, 1)
		assert.Equal(t, "hello a.md", replayed[0].Payload.Blocks[0].GetText().GetText())

		require.NotNil(t, replayed[1].File)
		assert.Equal(t, "/abs/orig.png", replayed[1].File.Path)
		assert.Equal(t, "orig.png", replayed[1].File.Name)
		assert.Equal(t, "https://example.org/orig.png", replayed[1].File.URL)
		assert.Equal(t, model.ImageKind_Cover, replayed[1].File.ImageKind)
		assert.Equal(t, map[string]string{"path": "key-bytes"}, replayed[1].File.EncryptionKeys)
		assert.Nil(t, replayed[1].File.Open)
	})

	t.Run("an undrained Open closure cannot be spooled", func(t *testing.T) {
		// given — the engine must drain downloads to the spill dir first; a
		// closure reaching the durable spool is a contract violation, never
		// a silent loss.
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		object := spoolObject("f.md", false)
		object.File = &importv2.FileSource{Open: func(ctx context.Context) (io.ReadCloser, error) { return nil, nil }}

		// when / then
		require.Error(t, spool.Append(ctx, object))
	})

	t.Run("a standalone spool serves volatile runs from a plain dir", func(t *testing.T) {
		// given
		spool, err := OpenStandaloneSpool(ctx, t.TempDir())
		require.NoError(t, err)
		defer spool.Close()

		// when
		require.NoError(t, spool.Append(ctx, spoolObject("a.md", false)))
		count := 0
		require.NoError(t, spool.Replay(ctx, func(o *importv2.Object) error {
			count++
			return nil
		}))

		// then
		assert.Equal(t, 1, count)
	})

	t.Run("replay order survives many appends", func(t *testing.T) {
		// given: enough rows that lexicographic-vs-numeric id ordering would
		// diverge if the sequence key were naive
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		const n = 25
		for i := 0; i < n; i++ {
			require.NoError(t, spool.Append(ctx, spoolObject(spoolKeyName(i), false)))
		}

		// when
		var keys []string
		require.NoError(t, spool.Replay(ctx, func(o *importv2.Object) error {
			keys = append(keys, o.SourceKey)
			return nil
		}))

		// then
		require.Len(t, keys, n)
		for i := 0; i < n; i++ {
			assert.Equal(t, spoolKeyName(i), keys[i])
		}
	})
}

func spoolKeyName(i int) string {
	// deliberately NOT zero-padded so only the spool's own ordering keeps it
	return "page-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i/26))
}

func TestSpoolReopenKeepsSequence(t *testing.T) {
	t.Run("a second spool handle continues the sequence instead of overwriting", func(t *testing.T) {
		// given — E2 (CONFIRMED by two lenses): the per-instance counter
		// restarted at 0, so a reopened spool destroyed rows and reordered
		// the replay.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		first, err := store.Spool(ctx)
		require.NoError(t, err)
		require.NoError(t, first.Append(ctx, spoolObject("a.md", false)))
		require.NoError(t, first.Append(ctx, spoolObject("b.md", false)))

		// when: a fresh handle appends
		second, err := store.Spool(ctx)
		require.NoError(t, err)
		require.NoError(t, second.Append(ctx, spoolObject("c.md", false)))
		var keys []string
		require.NoError(t, second.Replay(ctx, func(o *importv2.Object) error {
			keys = append(keys, o.SourceKey)
			return nil
		}))

		// then
		assert.Equal(t, []string{"a.md", "b.md", "c.md"}, keys)
	})
}

func TestSpoolReplayDoesNotPinTheDb(t *testing.T) {
	t.Run("run.db stays readable while replay is blocked on backpressure", func(t *testing.T) {
		// given — D2 (CONFIRMED): the replay iterator held the single read
		// connection (and pinned the WAL, D1) for the entire materialize
		// pass; a Manifest read still blocked after 3s. DM-2's checkpoint
		// writes during pass 3 would deadlock on day one.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		for i := 0; i < 40; i++ {
			require.NoError(t, spool.Append(ctx, spoolObject(spoolKeyName(i), false)))
		}
		blocked := make(chan struct{})
		release := make(chan struct{})
		replayDone := make(chan error, 1)
		emitted := 0
		go func() {
			replayDone <- spool.Replay(ctx, func(o *importv2.Object) error {
				emitted++
				if emitted == 5 {
					close(blocked)
					<-release // simulates persist backpressure mid-pass
				}
				return nil
			})
		}()
		<-blocked

		// when: a manifest read arrives mid-replay
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, err = store.Manifest(readCtx)

		// then
		require.NoError(t, err, "the db must stay readable during pass 3")
		close(release)
		require.NoError(t, <-replayDone)
		assert.Equal(t, 40, emitted)
	})
}

// TestSpoolRoundTripEveryField is the field-completeness half of the
// equivalence gate (F1): the goldens exercise the markdown-shaped subset,
// so every field — including the Notion-shaped ones no golden carries —
// is pinned here, on both legs of the serialization.
func TestSpoolRoundTripEveryField(t *testing.T) {
	ctx := context.Background()
	spool, err := OpenStandaloneSpool(ctx, t.TempDir())
	require.NoError(t, err)
	defer spool.Close()

	original := &importv2.Object{
		SourceKey: "notion-page-1",
		SbType:    coresb.SmartBlockTypeFileObject,
		Payload: &importv2.Snapshot{
			Blocks: []*model.Block{{Id: "b1", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "hello"},
			}}},
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				"aString":     domain.String("s"),
				"anInt":       domain.Int64(42),
				"aFloat":      domain.Float64(4.5),
				"aBool":       domain.Bool(true),
				"aStringList": domain.StringList([]string{"x", "y"}),
				"aFloatList":  domain.Float64List([]float64{1.5, 2.5}),
			}),
			FileKeys:                 &types.Struct{Fields: map[string]*types.Value{"k": {Kind: &types.Value_StringValue{StringValue: "v"}}}},
			ExtraRelations:           []*model.Relation{{Key: "extraRel", Format: model.RelationFormat_longtext}},
			ObjectTypes:              []string{"ot-page"},
			Collections:              &types.Struct{Fields: map[string]*types.Value{"objects": {Kind: &types.Value_StringValue{StringValue: "m"}}}},
			RemovedCollectionKeys:    []string{"removed-1"},
			RelationLinks:            []*model.RelationLink{{Key: "linked", Format: model.RelationFormat_date}},
			Key:                      "internal-key",
			OriginalCreatedTimestamp: 1700000123,
			FileInfo:                 &model.FileInfo{FileId: "bafyfile", EncryptionKeys: []*model.FileEncryptionKey{{Path: "/0/", Key: "kk"}}},
		},
		File: &importv2.FileSource{
			Path:           "/spill/img.png",
			Name:           "img.png",
			URL:            "https://prod-files-secure.s3.amazonaws.com/img.png?sig=x",
			ImageKind:      model.ImageKind_Icon,
			EncryptionKeys: map[string]string{"/0/": "enc"},
		},
		IsRootCandidate: true,
		Favorite:        true,
		Archived:        true,
	}

	require.NoError(t, spool.Append(ctx, original))
	var replayed *importv2.Object
	require.NoError(t, spool.Replay(ctx, func(o *importv2.Object) error {
		replayed = o
		return nil
	}))
	require.NotNil(t, replayed)

	// envelope
	assert.Equal(t, original.SourceKey, replayed.SourceKey)
	assert.Equal(t, original.SbType, replayed.SbType)
	assert.Equal(t, original.IsRootCandidate, replayed.IsRootCandidate)
	assert.Equal(t, original.Favorite, replayed.Favorite)
	assert.Equal(t, original.Archived, replayed.Archived)
	// file source, field by field
	require.NotNil(t, replayed.File)
	assert.Equal(t, original.File.Path, replayed.File.Path)
	assert.Equal(t, original.File.Name, replayed.File.Name)
	assert.Equal(t, original.File.URL, replayed.File.URL)
	assert.Equal(t, original.File.ImageKind, replayed.File.ImageKind)
	assert.Equal(t, original.File.EncryptionKeys, replayed.File.EncryptionKeys)
	// the whole snapshot, on the wire representation
	assert.Equal(t, original.Payload.ToProto(), replayed.Payload.ToProto())
}

// TestSpoolKnownCoercions pins the two deliberate/known deviations of the
// reverse leg so any change to them is loud (F3 + the nil-Details change).
func TestSpoolKnownCoercions(t *testing.T) {
	ctx := context.Background()
	spool, err := OpenStandaloneSpool(ctx, t.TempDir())
	require.NoError(t, err)
	defer spool.Close()

	t.Run("an EMPTY float list round-trips as an empty string list", func(t *testing.T) {
		// F3 (CONFIRMED): domain.ValueFromProto cannot distinguish empty
		// list kinds and falls through to StringList. Value-empty either
		// way; pinned so a domain-layer change is noticed here.
		object := spoolObject("empty-list.md", false)
		object.Payload.Details = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"emptyFloats": domain.Float64List(nil),
		})
		require.NoError(t, spool.Append(ctx, object))
		var replayed *importv2.Object
		require.NoError(t, spool.Replay(ctx, func(o *importv2.Object) error {
			if o.SourceKey == "empty-list.md" {
				replayed = o
			}
			return nil
		}))
		require.NotNil(t, replayed)
		value := replayed.Payload.Details.Get("emptyFloats")
		_, isStringList := value.TryStringList()
		assert.True(t, isStringList, "known coercion: empty lists come back as string lists")
	})

	t.Run("nil Details come back as empty Details, not nil", func(t *testing.T) {
		// Previously a nil-Details object panicked into a per-object
		// invariant issue at persist; through the spool it arrives as an
		// empty, non-nil Details. Documented behaviour change.
		object := spoolObject("nil-details.md", false)
		object.Payload.Details = nil
		require.NoError(t, spool.Append(ctx, object))
		var replayed *importv2.Object
		require.NoError(t, spool.Replay(ctx, func(o *importv2.Object) error {
			if o.SourceKey == "nil-details.md" {
				replayed = o
			}
			return nil
		}))
		require.NotNil(t, replayed)
		require.NotNil(t, replayed.Payload.Details)
		assert.Equal(t, 0, replayed.Payload.Details.Len())
	})
}
