//go:build !windows

package store

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mode(t *testing.T, path string) os.FileMode {
	fi, err := os.Lstat(path)
	require.NoError(t, err)
	return fi.Mode().Perm()
}

func TestFileModes(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("x")))
	assert.Equal(t, os.FileMode(0o700), mode(t, f.Dir()))
	assert.Equal(t, os.FileMode(0o700), mode(t, filepath.Dir(f.Dir())))
	assert.Equal(t, os.FileMode(0o600), mode(t, filepath.Join(f.Dir(), "a.item")))
	unlock, err := f.Lock(ctx, "a")
	require.NoError(t, err)
	unlock()
	assert.Equal(t, os.FileMode(0o600), mode(t, filepath.Join(f.Dir(), "a.lock")))
}

// refusedEverything asserts the folder rule failed: nothing is read,
// written or deleted.
func refusedEverything(t *testing.T, f *File, why string) {
	t.Helper()
	_, err := f.Read(ctx, "a")
	requireDenied(t, err, why+": read")
	requireDenied(t, f.Write(ctx, "a", []byte("new")), why+": write")
	requireDenied(t, f.Delete(ctx, "a"), why+": delete")
	requireDenied(t, f.Probe(ctx), why+": probe")
	_, err = f.Lock(ctx, "a")
	requireDenied(t, err, why+": lock")
}

func TestFileFolderRule(t *testing.T) {
	seeded := func(t *testing.T) (*File, string) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("kept")))
		return f, filepath.Join(f.Dir(), "a.item")
	}
	kept := func(t *testing.T, item string) {
		t.Helper()
		raw, err := os.ReadFile(item)
		require.NoError(t, err)
		assert.Equal(t, "kept", string(raw), "the item is untouched")
	}

	t.Run("the items folder at 0755", func(t *testing.T) {
		f, item := seeded(t)
		require.NoError(t, os.Chmod(f.Dir(), 0o755))
		refusedEverything(t, f, "0755")
		kept(t, item)
	})

	t.Run("a group-writable ancestor", func(t *testing.T) {
		f, item := seeded(t)
		require.NoError(t, os.Chmod(filepath.Join(f.home, ".anytype"), 0o770))
		refusedEverything(t, f, "group-writable ancestor")
		kept(t, item)
	})

	t.Run("the items folder is a symlink", func(t *testing.T) {
		f, _ := seeded(t)
		real := f.Dir() + "-real"
		require.NoError(t, os.Rename(f.Dir(), real))
		require.NoError(t, os.Symlink(real, f.Dir()))
		refusedEverything(t, f, "folder symlink")
		kept(t, filepath.Join(real, "a.item"))
	})

	t.Run("an ancestor is a symlink", func(t *testing.T) {
		f, _ := seeded(t)
		dot := filepath.Join(f.home, ".anytype")
		real := filepath.Join(f.home, "elsewhere")
		require.NoError(t, os.Rename(dot, real))
		require.NoError(t, os.Symlink(real, dot))
		refusedEverything(t, f, "ancestor symlink")
		kept(t, filepath.Join(real, "ai-connect", "items-dev", "a.item"))
	})

	t.Run("a folder owned by another user", func(t *testing.T) {
		f, item := seeded(t)
		f.uid = func() int { return os.Geteuid() + 1 }
		refusedEverything(t, f, "wrong owner")
		kept(t, item)
	})

	t.Run("a new store is not created inside a folder that fails the rule", func(t *testing.T) {
		f := newFile(t)
		dot := filepath.Join(f.home, ".anytype")
		require.NoError(t, os.Mkdir(dot, 0o777))
		require.NoError(t, os.Chmod(dot, 0o777))
		requireDenied(t, f.Write(ctx, "a", []byte("x")), "world-writable ancestor")
		_, err := os.Lstat(filepath.Join(dot, "ai-connect"))
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestFileItemRule(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "seed", []byte("x")))
	dir := f.Dir()
	item := func(id string) string { return filepath.Join(dir, id+".item") }
	outside := filepath.Join(f.home, "outside")
	require.NoError(t, os.WriteFile(outside, []byte("secret elsewhere"), 0o600))

	plant := map[string]func(t *testing.T, path string){
		"a symlink to a file outside the folder": func(t *testing.T, path string) {
			require.NoError(t, os.Symlink(outside, path))
		},
		"a symlink to an item inside the folder": func(t *testing.T, path string) {
			require.NoError(t, os.Symlink("seed.item", path))
		},
		"mode 0644": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
			require.NoError(t, os.Chmod(path, 0o644))
		},
		"mode 0400": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
			require.NoError(t, os.Chmod(path, 0o400))
		},
		"a hard link (two links)": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
			require.NoError(t, os.Link(path, filepath.Join(f.home, "second-link")))
		},
		"a FIFO": func(t *testing.T, path string) {
			require.NoError(t, syscall.Mkfifo(path, 0o600))
		},
		"a directory": func(t *testing.T, path string) {
			require.NoError(t, os.Mkdir(path, 0o700))
		},
	}
	n := 0
	for name, do := range plant {
		n++
		id := "case" + string(rune('a'+n))
		t.Run(name, func(t *testing.T) {
			do(t, item(id))
			done := make(chan error, 1)
			go func() {
				_, err := f.Read(ctx, id)
				done <- err
			}()
			requireDenied(t, <-done, name)
		})
	}

	t.Run("an item owned by another user", func(t *testing.T) {
		g := newFile(t)
		require.NoError(t, g.Write(ctx, "a", []byte("x")))
		calls := 0
		// the folders pass (three checks); the item's owner differs
		g.uid = func() int {
			calls++
			if calls > 3 {
				return os.Geteuid() + 1
			}
			return os.Geteuid()
		}
		_, err := g.Read(ctx, "a")
		requireDenied(t, err, "wrong item owner")
	})

	t.Run("a lock that is not a regular file", func(t *testing.T) {
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, "linked.lock")))
		_, err := f.Lock(ctx, "linked")
		requireDenied(t, err, "lock symlink")
	})

	raw, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "secret elsewhere", string(raw))
}

func TestFileUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("x")))
	require.NoError(t, os.Chmod(f.Dir(), 0o000))
	t.Cleanup(func() { _ = os.Chmod(f.Dir(), 0o700) })
	_, err := f.Read(ctx, "a")
	require.True(t, IsTemporary(err), "try again, never 'gone': %v", err)
	require.True(t, IsTemporary(f.Write(ctx, "a", []byte("y"))))
	require.True(t, IsTemporary(f.Delete(ctx, "a")))
}
