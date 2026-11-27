package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// newFile returns a store under a scratch home: an explicit root, never
// the developer's home.
func newFile(t *testing.T) *File {
	home := t.TempDir()
	// macOS temp folders sit behind a symlink; the store only looks at its
	// own three folders
	return OpenAt(home, NamespaceDev)
}

func requireDenied(t *testing.T, err error, msg string) {
	t.Helper()
	var te *TemporaryError
	require.ErrorAs(t, err, &te, msg)
	require.Equal(t, ReasonDenied, te.Reason, "%s: %v", msg, err)
}

func tempFiles(t *testing.T, dir string) []string {
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestFileRoundTrip(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Probe(ctx), "a store nobody has used yet is usable")
	_, err := f.Read(ctx, "a")
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, f.Delete(ctx, "a"), ErrNotFound)
	_, err = os.Lstat(filepath.Join(f.home, ".anytype"))
	require.ErrorIs(t, err, os.ErrNotExist, "reading creates nothing")

	require.NoError(t, f.Write(ctx, "a", []byte("one")))
	require.NoError(t, f.Write(ctx, "a", []byte("two")))
	raw, err := f.Read(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, "two", string(raw))
	assert.Empty(t, tempFiles(t, f.Dir()))
	assert.Equal(t, "items-dev", filepath.Base(f.Dir()))
	assert.NotEqual(t, f.Dir(), OpenAt(f.home, NamespaceRelease).Dir(), "the namespaces use different folders")
	_, err = OpenAt(f.home, NamespaceRelease).Read(ctx, "a")
	require.ErrorIs(t, err, ErrNotFound, "the release namespace can't see a development item")

	require.NoError(t, f.Delete(ctx, "a"))
	_, err = f.Read(ctx, "a")
	require.ErrorIs(t, err, ErrNotFound)

	unlock, err := f.Lock(ctx, "a")
	require.NoError(t, err)
	unlock()
}

func TestFileTooLargeIsNotAnItem(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte(strings.Repeat("x", MaxItemBytes+1))))
	_, err := f.Read(ctx, "a")
	require.ErrorIs(t, err, ErrInvalidItem)
}

func TestFileWriteFailures(t *testing.T) {
	t.Run("a failed rename leaves the old item and no temp file", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("old")))
		f.hook = func(stage string) error {
			if stage == "before-rename" {
				return errors.New("injected")
			}
			return nil
		}
		err := f.Write(ctx, "a", []byte("new"))
		require.True(t, IsTemporary(err), "nothing was written: %v", err)
		f.hook = nil
		raw, err := f.Read(ctx, "a")
		require.NoError(t, err)
		assert.Equal(t, "old", string(raw))
		assert.Empty(t, tempFiles(t, f.Dir()))
	})

	t.Run("a failure after the rename is unconfirmed, and the new item is in place", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("old")))
		f.hook = func(stage string) error {
			if stage == "after-rename" {
				return errors.New("injected")
			}
			return nil
		}
		err := f.Write(ctx, "a", []byte("new"))
		var unconfirmed *UnconfirmedError
		require.ErrorAs(t, err, &unconfirmed)
		assert.False(t, IsTemporary(err))
		raw, err := f.Read(ctx, "a")
		require.NoError(t, err)
		assert.Equal(t, "new", string(raw))
	})

	t.Run("a stale temp file is removed by the next write and never read", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("one")))
		stale := filepath.Join(f.Dir(), "a.0011223344556677.tmp")
		other := filepath.Join(f.Dir(), "ab.0011223344556677.tmp")
		require.NoError(t, os.WriteFile(stale, []byte("a crashed write, key included"), 0o600))
		require.NoError(t, os.WriteFile(other, []byte("another connection's"), 0o600))
		raw, err := f.Read(ctx, "a")
		require.NoError(t, err)
		assert.Equal(t, "one", string(raw))
		require.NoError(t, f.Write(ctx, "a", []byte("two")))
		_, err = os.Lstat(stale)
		require.ErrorIs(t, err, os.ErrNotExist)
		_, err = os.Lstat(other)
		require.NoError(t, err, "only this connection's leftovers are swept")
	})
}

// L8: a receipt means the change is durable and the item is the only file
// that holds the key (or no file at all after a delete).
func TestFileReceipts(t *testing.T) {
	failAt := func(stage string) func(string) error {
		return func(s string) error {
			if s == stage {
				return errors.New("injected at " + stage)
			}
			return nil
		}
	}
	leftover := func(t *testing.T, f *File) string {
		p := filepath.Join(f.Dir(), "a.0011223344556677.tmp")
		require.NoError(t, os.WriteFile(p, []byte("a crashed rotation, candidate key included"), 0o600))
		return p
	}

	t.Run("delete removes a crashed rotation's temp file too", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("one")))
		stale := leftover(t, f)
		require.NoError(t, f.Delete(ctx, "a"))
		_, err := os.Lstat(stale)
		require.ErrorIs(t, err, os.ErrNotExist)
		assert.Empty(t, tempFiles(t, f.Dir()))
	})

	t.Run("a failed folder sync after the unlink is unconfirmed, and the item is gone", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("one")))
		f.hook = failAt("after-unlink")
		err := f.Delete(ctx, "a")
		var unconfirmed *UnconfirmedError
		require.ErrorAs(t, err, &unconfirmed)
		f.hook = nil
		_, err = f.Read(ctx, "a")
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("a sweep that fails stops the verb before it changes anything", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("one")))
		stale := leftover(t, f)
		f.hook = failAt("sweep")
		require.True(t, IsTemporary(f.Delete(ctx, "a")))
		require.True(t, IsTemporary(f.Write(ctx, "a", []byte("two"))))
		f.hook = nil
		raw, err := f.Read(ctx, "a")
		require.NoError(t, err)
		assert.Equal(t, "one", string(raw), "the item is unchanged")
		_, err = os.Lstat(stale)
		require.NoError(t, err)
	})

	t.Run("a new folder that can't be made durable: nothing is written", func(t *testing.T) {
		f := newFile(t)
		f.hook = failAt("sync-parent")
		require.True(t, IsTemporary(f.Write(ctx, "a", []byte("one"))))
		f.hook = nil
		_, err := f.Read(ctx, "a")
		require.ErrorIs(t, err, ErrNotFound)
	})
}
