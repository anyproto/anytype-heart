package runstore

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
