package anystoremigrate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	anystorev1 "github.com/anyproto/any-store"
	v1enc "github.com/anyproto/any-store/anyenc"
	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

var ctx = context.Background()

const (
	chatCollection    = "chat1chats"
	historyCollection = "chat1chatMessageHistory"
	plainCollection   = "objects"
)

// v1Doc is a document of the test v1 database, built field by field.
type v1Doc func(a *v1enc.Arena, doc *v1enc.Value)

// chatMessage is a stored chat message: the order id lives in _o.id, a stub has none.
func chatMessage(id, orderId string, more ...func(a *v1enc.Arena, doc *v1enc.Value)) v1Doc {
	return func(a *v1enc.Arena, doc *v1enc.Value) {
		doc.Set("id", a.NewString(id))
		orders := a.NewObject()
		if orderId != "" {
			orders.Set("id", a.NewString(orderId))
		} else {
			orders.Set(chatmodel.ReactionsKey, a.NewString("!!$3"))
		}
		doc.Set(chatmodel.OrderKey, orders)
		doc.Set(chatmodel.CreatorKey, a.NewString("bob"))
		for _, f := range more {
			f(a, doc)
		}
	}
}

func cid(tail string) string {
	return "bafyreihdb7g4mnw3ebhnm6wl2ddtwy4ejbryyhlrrwa4dxxkem" + tail
}

// fixture is a v1 database at path, in its own directory, and the migrator to run on it.
type fixture struct {
	*Migrator
	dir, path string
	free      uint64
}

func newFixture(t *testing.T, opts ...func(o *Options)) *fixture {
	t.Helper()
	dir := t.TempDir()
	fx := &fixture{dir: dir, path: filepath.Join(dir, "crdt.db"), free: 1 << 40}
	o := Options{FreeSpace: func(string) (uint64, error) { return fx.free, nil }}
	for _, f := range opts {
		f(&o)
	}
	fx.Migrator = New(o)
	fx.seedV1(t)
	return fx
}

// seedV1 writes a v1 database with a plain collection (unique and sparse indexes, explicit nulls,
// binary values), a chat messages collection (concurrent messages sharing an order id, stubs, a
// pin, a tombstone, reactions) and a chat history collection.
func (fx *fixture) seedV1(t *testing.T) {
	t.Helper()
	db, err := anystorev1.Open(ctx, fx.path, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	arena := &v1enc.Arena{}
	insert := func(coll anystorev1.Collection, docs ...v1Doc) {
		for _, build := range docs {
			arena.Reset()
			doc := arena.NewObject()
			build(arena, doc)
			require.NoError(t, coll.Insert(ctx, doc))
		}
	}

	plain, err := db.CreateCollection(ctx, plainCollection)
	require.NoError(t, err)
	require.NoError(t, plain.EnsureIndex(ctx,
		anystorev1.IndexInfo{Fields: []string{"key"}, Unique: true},
		anystorev1.IndexInfo{Fields: []string{"maybe"}, Sparse: true},
		anystorev1.IndexInfo{Fields: []string{"key", "-rank"}},
	))
	insert(plain,
		func(a *v1enc.Arena, doc *v1enc.Value) {
			doc.Set("id", a.NewString("o1"))
			doc.Set("key", a.NewString("k1"))
			doc.Set("rank", a.NewNumberInt(1))
			doc.Set("maybe", a.NewNull())
			doc.Set("blob", a.NewBinary([]byte{0, 1, 2, 0xff}))
		},
		func(a *v1enc.Arena, doc *v1enc.Value) {
			doc.Set("id", a.NewString("o2"))
			doc.Set("key", a.NewString("k2"))
			doc.Set("rank", a.NewNumberInt(2))
			doc.Set("maybe", a.NewString("yes"))
		},
		func(a *v1enc.Arena, doc *v1enc.Value) {
			doc.Set("id", a.NewString("o3"))
			doc.Set("key", a.NewString("k3"))
			doc.Set("rank", a.NewNumberInt(3))
		},
	)

	chat, err := db.CreateCollection(ctx, chatCollection)
	require.NoError(t, err)
	require.NoError(t, chat.EnsureIndex(ctx,
		anystorev1.IndexInfo{Fields: []string{"_o.id"}},
		anystorev1.IndexInfo{Fields: []string{chatmodel.PinnedKey}, Sparse: true},
		anystorev1.IndexInfo{Fields: []string{chatmodel.CreatorKey, "_o.id"}},
	))
	insert(chat,
		chatMessage(cid("aaaa5"), "!!$5"),
		chatMessage(cid("aaaa2"), "!!$2"),
		chatMessage(cid("bbbb2"), "!!$2"), // concurrent with aaaa2
		chatMessage(cid("pinn3"), "!!$3", func(a *v1enc.Arena, doc *v1enc.Value) { doc.Set(chatmodel.PinnedKey, a.NewTrue()) }),
		chatMessage(cid("dead4"), "!!$4", func(a *v1enc.Arena, doc *v1enc.Value) { doc.Set(chatmodel.DeletedKey, a.NewTrue()) }),
		chatMessage(cid("reac6"), "!!$6", func(a *v1enc.Arena, doc *v1enc.Value) {
			reactions := a.NewObject()
			ids := a.NewArray()
			ids.SetArrayItem(0, a.NewString("alice"))
			reactions.Set("👍", ids)
			doc.Set(chatmodel.ReactionsKey, reactions)
			doc.Set(chatmodel.ReactionUnreadOrderIdKey, a.NewString("!!$7"))
		}),
		chatMessage(cid("stub1"), ""),
		chatMessage(cid("stub2"), ""),
	)

	history, err := db.CreateCollection(ctx, historyCollection)
	require.NoError(t, err)
	for _, id := range []string{cid("aaaa5"), cid("aaaa2"), cid("stub1")} {
		insert(history, func(a *v1enc.Arena, doc *v1enc.Value) { doc.Set("id", a.NewString(id)) })
	}
}

func openV2(t *testing.T, path string) anystore.DB {
	t.Helper()
	db, err := anystore.Open(ctx, path, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func indexInfos(coll anystore.Collection) map[string]anystore.IndexInfo {
	infos := make(map[string]anystore.IndexInfo)
	for _, idx := range coll.GetIndexes() {
		info := idx.Info()
		infos[info.Name] = anystore.IndexInfo{Name: info.Name, Fields: info.Fields, Unique: info.Unique, Sparse: info.Sparse}
	}
	return infos
}

func TestMigrateFile(t *testing.T) {
	// given
	fx := newFixture(t)
	wantReport := Report{
		Path:            fx.path,
		Collections:     3,
		Documents:       3 + 6 + 3,
		ChatCollections: 1,
		StubsDropped:    2,
	}
	wantChatOrder := []string{
		chatmodel.StorageKey("!!$2", cid("aaaa2")),
		chatmodel.StorageKey("!!$2", cid("bbbb2")),
		chatmodel.StorageKey("!!$3", cid("pinn3")),
		chatmodel.StorageKey("!!$4", cid("dead4")),
		chatmodel.StorageKey("!!$5", cid("aaaa5")),
		chatmodel.StorageKey("!!$6", cid("reac6")),
	}
	wantPlainIndexes := map[string]anystore.IndexInfo{
		"key":       {Name: "key", Fields: []string{"key"}, Unique: true},
		"maybe":     {Name: "maybe", Fields: []string{"maybe"}, Sparse: true},
		"key,-rank": {Name: "key,-rank", Fields: []string{"key", "-rank"}},
	}
	wantChatIndexes := make(map[string]anystore.IndexInfo)
	for _, info := range chatmodel.MessagesCollectionIndexes() {
		wantChatIndexes[info.Name] = info
	}

	// when
	got, err := fx.MigrateFile(ctx, fx.path)

	// then
	require.NoError(t, err)
	assert.Equal(t, wantReport, Report{
		Path: got.Path, Collections: got.Collections, Documents: got.Documents,
		ChatCollections: got.ChatCollections, StubsDropped: got.StubsDropped,
	})
	assert.Positive(t, got.SrcBytes)
	assert.Positive(t, got.DstBytes)
	assert.Equal(t, []string{"crdt.db", "crdt.db" + V1BackupSuffix}, dirNames(t, fx.dir), "no temp file is left")

	db := openV2(t, fx.path)
	names, err := db.GetCollectionNames(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{plainCollection, chatCollection, historyCollection}, names)

	t.Run("plain collections keep their documents, types and indexes", func(t *testing.T) {
		plain, err := db.OpenCollection(ctx, plainCollection)
		require.NoError(t, err)
		count, err := plain.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
		doc, err := plain.FindId(ctx, "o1")
		require.NoError(t, err)
		assert.Equal(t, anyenc.TypeBinary, doc.Value().Get("blob").Type())
		assert.Equal(t, []byte{0, 1, 2, 0xff}, doc.Value().GetBytes("blob"))
		assert.Equal(t, anyenc.TypeNull, doc.Value().Get("maybe").Type())
		assert.Equal(t, wantPlainIndexes, indexInfos(plain))
		// v2 holds an explicit null in a sparse index (v1 left it out)
		withMaybe, err := plain.Find(map[string]any{"maybe": map[string]any{"$exists": true}}).Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 2, withMaybe)
	})

	t.Run("a chat is keyed by the storage key and loses its stubs", func(t *testing.T) {
		chat, err := db.OpenCollection(ctx, chatCollection)
		require.NoError(t, err)
		assert.Equal(t, chatmodel.StorageKeyField, chat.PrimaryKey())
		assert.Equal(t, wantChatIndexes, indexInfos(chat))
		iter, err := chat.Find(nil).Sort(chatmodel.SortStorageKeyAsc).Iter(ctx)
		require.NoError(t, err)
		var keys []string
		for iter.Next() {
			doc, err := iter.Doc()
			require.NoError(t, err)
			keys = append(keys, doc.Value().GetString(chatmodel.StorageKeyField))
		}
		require.NoError(t, iter.Close())
		assert.Equal(t, wantChatOrder, keys)
		// still found by id, through the unique index
		pinned, err := chat.Find(map[string]any{"id": cid("pinn3")}).Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, pinned)
		stubs, err := chat.Find(map[string]any{"id": map[string]any{"$in": []any{cid("stub1"), cid("stub2")}}}).Count(ctx)
		require.NoError(t, err)
		assert.Zero(t, stubs)
	})

	t.Run("the history is copied as it is", func(t *testing.T) {
		history, err := db.OpenCollection(ctx, historyCollection)
		require.NoError(t, err)
		assert.Equal(t, "id", history.PrimaryKey())
		count, err := history.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("the v1 file is kept beside it, whole", func(t *testing.T) {
		isV1, err := IsV1File(fx.path + V1BackupSuffix)
		require.NoError(t, err)
		assert.True(t, isV1)
		v1, err := anystorev1.Open(ctx, fx.path+V1BackupSuffix, nil)
		require.NoError(t, err)
		defer v1.Close()
		chat, err := v1.OpenCollection(ctx, chatCollection)
		require.NoError(t, err)
		count, err := chat.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, 8, count)
	})
}

func v1Counts(t *testing.T, path string) map[string]int {
	t.Helper()
	db, err := anystorev1.Open(ctx, path, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	names, err := db.GetCollectionNames(ctx)
	require.NoError(t, err)
	counts := make(map[string]int, len(names))
	for _, name := range names {
		coll, err := db.OpenCollection(ctx, name)
		require.NoError(t, err)
		counts[name], err = coll.Count(ctx)
		require.NoError(t, err)
	}
	return counts
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestMigrateFileOfAMigratedDatabaseIsANoOp(t *testing.T) {
	// given
	fx := newFixture(t)
	_, err := fx.MigrateFile(ctx, fx.path)
	require.NoError(t, err)
	before, err := os.ReadFile(fx.path)
	require.NoError(t, err)

	// when
	got, err := fx.MigrateFile(ctx, fx.path)

	// then
	require.NoError(t, err)
	assert.Equal(t, Report{}, got)
	after, err := os.ReadFile(fx.path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestMigrateFileFailureLeavesTheV1Database(t *testing.T) {
	// given: a copy that dies half way
	fx := newFixture(t, func(o *Options) { o.failAfterDocs = 2 })
	want := map[string]int{plainCollection: 3, chatCollection: 8, historyCollection: 3}

	// when
	_, err := fx.MigrateFile(ctx, fx.path)

	// then
	require.ErrorIs(t, err, errInjected)
	assert.Equal(t, []string{"crdt.db"}, dirNames(t, fx.dir), "no temp file, no backup")
	// opening a v1 database bumps SQLite's change counter, nothing else changes
	assert.Equal(t, want, v1Counts(t, fx.path), "the v1 database is whole")

	t.Run("and the next attempt migrates it", func(t *testing.T) {
		fx.opts.failAfterDocs = 0
		got, err := fx.MigrateFile(ctx, fx.path)
		require.NoError(t, err)
		assert.Equal(t, 12, got.Documents)
	})
}

func TestMigrateFileRemovesALeftoverTempFile(t *testing.T) {
	// given: what a migration that crashed left
	fx := newFixture(t)
	for _, name := range tempFiles(fx.path + tempSuffix) {
		require.NoError(t, os.WriteFile(name, []byte("junk"), 0o600))
	}

	// when
	got, err := fx.MigrateFile(ctx, fx.path)

	// then
	require.NoError(t, err)
	assert.Equal(t, 12, got.Documents)
	assert.Equal(t, []string{"crdt.db", "crdt.db" + V1BackupSuffix}, dirNames(t, fx.dir))
}

func TestMigrateFileWithoutEnoughSpace(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.free = 1 << 20
	before, err := os.ReadFile(fx.path)
	require.NoError(t, err)

	// when
	_, err = fx.MigrateFile(ctx, fx.path)

	// then
	require.ErrorIs(t, err, ErrNotEnoughSpace)
	var spaceErr *NotEnoughSpaceError
	require.ErrorAs(t, err, &spaceErr)
	assert.Equal(t, uint64(1<<20), spaceErr.Available)
	assert.Equal(t, spaceNeeded(int64(len(before))), spaceErr.Needed)
	assert.Equal(t, []string{"crdt.db"}, dirNames(t, fx.dir), "nothing is written")
	after, err := os.ReadFile(fx.path)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestCheckSpace(t *testing.T) {
	// three v1 databases of one size, and a v2 one CheckSpace leaves out
	seed := newFixture(t)
	data, err := os.ReadFile(seed.path)
	require.NoError(t, err)
	size := int64(len(data))
	src := uint64(size)
	allowance := spaceNeeded(size) - src // WAL allowance and margin of one build

	for _, tc := range []struct {
		name       string
		limit      int
		removeV1   bool
		noLinks    bool
		wantNeeded uint64
	}{
		{name: "one at a time, v1 files kept: every new file, one allowance", limit: 1, wantNeeded: 3*src + allowance},
		{name: "two at a time, v1 files kept: every new file, two allowances", limit: 2, wantNeeded: 3*src + 2*allowance},
		{name: "two at a time, v1 files kept as copies: every new file and copy, two allowances", limit: 2, noLinks: true, wantNeeded: 6*src + 2*allowance},
		{name: "two at a time, v1 files removed: two builds", limit: 2, removeV1: true, wantNeeded: 2 * (src + allowance)},
		{name: "two at a time, v1 files removed, copies until then: two builds and their copies", limit: 2, removeV1: true, noLinks: true, wantNeeded: 2 * (2*src + allowance)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			fx := newFixture(t, func(o *Options) {
				o.Concurrency = tc.limit
				o.RemoveV1AfterVerify = tc.removeV1
				o.linksSupported = func(string) bool { return !tc.noLinks }
			})
			paths := []string{fx.path, filepath.Join(fx.dir, "objects.db"), filepath.Join(fx.dir, "other.db")}
			for _, p := range paths[1:] {
				require.NoError(t, os.WriteFile(p, data, 0o600))
			}
			v2Path := filepath.Join(fx.dir, "v2.db")
			_ = openV2(t, v2Path)
			paths = append(paths, v2Path)

			// when
			fx.free = tc.wantNeeded - 1
			short := fx.CheckSpace(paths)
			fx.free = tc.wantNeeded
			enough := fx.CheckSpace(paths)

			// then
			var spaceErr *NotEnoughSpaceError
			require.ErrorAs(t, short, &spaceErr)
			assert.Equal(t, tc.wantNeeded, spaceErr.Needed)
			assert.NoError(t, enough)
		})
	}
}

func TestLinksSupported(t *testing.T) {
	// the test volume (APFS, ext4, NTFS) has hard links, and the probe leaves nothing behind
	dir := t.TempDir()
	assert.True(t, linksSupported(dir))
	assert.Empty(t, dirNames(t, dir))
	assert.False(t, linksSupported(filepath.Join(dir, "missing")), "a probe that can't be made counts as no links")
}

func TestMigrateFileRemovingTheV1File(t *testing.T) {
	// given
	fx := newFixture(t, func(o *Options) { o.RemoveV1AfterVerify = true })

	// when
	_, err := fx.MigrateFile(ctx, fx.path)

	// then
	require.NoError(t, err)
	assert.Equal(t, []string{"crdt.db"}, dirNames(t, fx.dir))
	isV1, err := IsV1File(fx.path)
	require.NoError(t, err)
	assert.False(t, isV1)
}
