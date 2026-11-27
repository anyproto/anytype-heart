//go:build darwin

package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chmodACL adds an ACL entry and removes every ACL again at cleanup (a
// deny-delete entry would otherwise keep the temp folder from going away).
func chmodACL(t *testing.T, entry, path string) {
	t.Helper()
	out, err := exec.Command("/bin/chmod", "+a", entry, path).CombinedOutput()
	require.NoError(t, err, string(out))
	t.Cleanup(func() { _ = exec.Command("/bin/chmod", "-R", "-N", path).Run() })
}

func TestACLReading(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	require.NoError(t, os.Mkdir(plain, 0o700))
	got, err := pathACL(plain)
	require.NoError(t, err)
	assert.Equal(t, aclInfo{}, got)

	chmodACL(t, "everyone deny delete", plain)
	got, err = pathACL(plain)
	require.NoError(t, err)
	assert.Equal(t, aclInfo{entries: 1, allow: 0}, got)

	chmodACL(t, "everyone allow read,list,search,file_inherit,directory_inherit", plain)
	got, err = pathACL(plain)
	require.NoError(t, err)
	assert.Equal(t, aclInfo{entries: 2, allow: 1}, got)

	file := filepath.Join(plain, "inherits")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	fh, err := os.Open(file)
	require.NoError(t, err)
	defer fh.Close()
	got, err = fileACL(fh)
	require.NoError(t, err)
	assert.Equal(t, 1, got.entries, "a file created in the folder inherits the allow entry")
	assert.Equal(t, 1, got.allow)

	_, err = pathACL(filepath.Join(dir, "missing"))
	require.Error(t, err)
}

// L3: any ACL on the Anytype folders or the item is refused; home may
// carry deny entries only.
func TestFileACLRule(t *testing.T) {
	const share = "everyone allow read,list,search,file_inherit,directory_inherit"

	t.Run("a home with the stock deny entry works", func(t *testing.T) {
		f := newFile(t)
		chmodACL(t, "everyone deny delete", f.home)
		require.NoError(t, f.Write(ctx, "a", []byte("x")))
		raw, err := f.Read(ctx, "a")
		require.NoError(t, err)
		assert.Equal(t, "x", string(raw))
		require.NoError(t, f.Delete(ctx, "a"))
	})

	t.Run("a shared home (an allow entry) is refused", func(t *testing.T) {
		f := newFile(t)
		chmodACL(t, share, f.home)
		requireDenied(t, f.Write(ctx, "a", []byte("x")), "allow entry on home")
		_, err := os.Lstat(filepath.Join(f.home, ".anytype"))
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("an allow entry on ai-connect: nothing is read, written or deleted", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("kept")))
		dir := filepath.Dir(f.Dir())
		chmodACL(t, share, dir)
		_, err := f.Read(ctx, "a")
		requireDenied(t, err, "read")
		assert.Contains(t, err.Error(), "has an ACL")
		assert.Contains(t, err.Error(), dir)
		requireDenied(t, f.Write(ctx, "a", []byte("new")), "write")
		requireDenied(t, f.Delete(ctx, "a"), "delete")
		raw, err := os.ReadFile(filepath.Join(f.Dir(), "a.item"))
		require.NoError(t, err)
		assert.Equal(t, "kept", string(raw))
	})

	t.Run("a deny entry on an Anytype folder is refused too: no ACL at all", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("x")))
		chmodACL(t, "everyone deny delete", f.Dir())
		_, err := f.Read(ctx, "a")
		requireDenied(t, err, "deny entry on items")
	})

	t.Run("an allow entry on the item alone", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("x")))
		chmodACL(t, "everyone allow read", filepath.Join(f.Dir(), "a.item"))
		_, err := f.Read(ctx, "a")
		requireDenied(t, err, "item ACL")
		assert.Contains(t, err.Error(), "the item has an ACL")
	})
}
