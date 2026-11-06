package api

import (
	"context"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/headsync/headstorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/anystorage"
)

// fakeObjectGetter hands the adapter a prepared smartblock.
type fakeObjectGetter struct{ sb smartblock.SmartBlock }

func (f fakeObjectGetter) GetObject(context.Context, string) (smartblock.SmartBlock, error) {
	return f.sb, nil
}

func (f fakeObjectGetter) GetObjectByFullID(context.Context, domain.FullID) (smartblock.SmartBlock, error) {
	return f.sb, nil
}

// TestObjectReadAdapter covers the load-bearing C7/§8 invariant: the snapshot
// and the heads (from which the etag derives) are captured from the same
// smartblock under one lock, so the returned etag and content are consistent.
func TestObjectReadAdapter(t *testing.T) {
	sb := smarttest.New("obj1")
	sb.AddBlock(simple.New(&model.Block{Id: "obj1", ChildrenIds: []string{"p1"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "p1",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "hello"}}}))

	adapter := newObjectReadAdapter(fakeObjectGetter{sb: sb}, nil)
	read, err := adapter.ReadObject(context.Background(), "space1", "obj1")
	require.NoError(t, err)

	// the heads returned are exactly the smartblock's heads at read time
	assert.Equal(t, sb.GetDocInfo().Heads, read.Heads)
	require.NotNil(t, read.Snapshot)

	// the snapshot carries the state's content (proves it was read, not empty)
	var texts []string
	for _, b := range read.Snapshot.Blocks {
		if txt := b.GetText(); txt != nil {
			texts = append(texts, txt.Text)
		}
	}
	assert.Contains(t, texts, "hello")
}

// headsOnlyStorage is a space storage that serves just its head storage.
type headsOnlyStorage struct {
	anystorage.ClientSpaceStorage
	heads headstorage.HeadStorage
}

func (s headsOnlyStorage) HeadStorage() headstorage.HeadStorage { return s.heads }

type fakeSpaces struct {
	spc clientspace.Space
	err error
}

func (f fakeSpaces) Get(context.Context, string) (clientspace.Space, error) { return f.spc, f.err }

func TestObjectReadAdapterIsDeleted(t *testing.T) {
	// given: a real head storage holding one entry per deletion status
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "heads.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	heads, err := headstorage.New(ctx, db)
	require.NoError(t, err)
	for id, status := range map[string]headstorage.DeletedStatus{
		"live":    headstorage.DeletedStatusNotDeleted,
		"queued":  headstorage.DeletedStatusQueued,
		"deleted": headstorage.DeletedStatusDeleted,
	} {
		require.NoError(t, heads.UpdateEntry(ctx, headstorage.HeadsUpdate{Id: id, Heads: []string{"head"}, DeletedStatus: &status}))
	}
	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().Storage().Return(headsOnlyStorage{heads: heads})
	sb := smarttest.New("live")
	adapter := newObjectReadAdapter(fakeObjectGetter{sb: sb}, fakeSpaces{spc: spc})

	t.Run("the mark decides, and an id with no entry is not deleted", func(t *testing.T) {
		for id, want := range map[string]bool{"live": false, "queued": true, "deleted": true, "never": false} {
			got, err := adapter.IsDeleted(ctx, "space1", id)

			require.NoError(t, err, id)
			assert.Equal(t, want, got, id)
		}
	})

	t.Run("a read of a deleted object fails as deleted before loading it", func(t *testing.T) {
		_, err := adapter.ReadObject(ctx, "space1", "queued")

		assert.ErrorIs(t, err, spacestorage.ErrTreeStorageAlreadyDeleted)
	})

	t.Run("a read of a live object goes on", func(t *testing.T) {
		_, err := adapter.ReadObject(ctx, "space1", "live")

		require.NoError(t, err)
	})

	t.Run("a failing space lookup is an error from the probe; the read itself goes on", func(t *testing.T) {
		failing := newObjectReadAdapter(fakeObjectGetter{sb: sb}, fakeSpaces{err: assert.AnError})

		_, probeErr := failing.IsDeleted(ctx, "space1", "live")
		_, readErr := failing.ReadObject(ctx, "space1", "live")

		require.ErrorIs(t, probeErr, assert.AnError)
		require.NoError(t, readErr, "the load that follows is what decides a read")
	})

	t.Run("without a space service the probe is an error, not a verdict", func(t *testing.T) {
		_, err := newObjectReadAdapter(fakeObjectGetter{sb: sb}, nil).IsDeleted(ctx, "space1", "live")

		require.Error(t, err)
	})
}
