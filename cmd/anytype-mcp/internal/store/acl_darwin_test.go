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

// Any ACL on .anytype, ai-connect, the folders below .anytype, the
// endpoint file or the executable is refused; home may carry deny entries
// only.
func TestFileACLRule(t *testing.T) {
	const share = "everyone allow read,list,search,file_inherit,directory_inherit"

	t.Run("a home with the stock deny entry works", func(t *testing.T) {
		f := seeded(t)
		chmodACL(t, "everyone deny delete", f.home)
		_, err := f.Read(ctx)
		require.NoError(t, err)
	})

	t.Run("a shared home (an allow entry) is refused", func(t *testing.T) {
		f := seeded(t)
		chmodACL(t, share, f.home)
		_, err := f.Read(ctx)
		deniedOn(t, err, f.home, "ACL that grants access")
	})

	for _, entry := range []string{share, "everyone deny delete"} {
		t.Run("ai-connect with "+entry, func(t *testing.T) {
			f := seeded(t)
			dir := filepath.Dir(f.Path())
			chmodACL(t, entry, dir)
			_, err := f.Read(ctx)
			deniedOn(t, err, dir, "has an ACL")
		})
	}

	t.Run("an allow entry on the file alone", func(t *testing.T) {
		f := seeded(t)
		chmodACL(t, "everyone allow read", f.Path())
		_, err := f.Read(ctx)
		deniedOn(t, err, f.Path(), "the endpoint file has an ACL")
	})

	t.Run("bin that grants another user write, and an executable with an ACL", func(t *testing.T) {
		f := seeded(t)
		bin := filepath.Join(f.home, ".anytype", "bin")
		require.NoError(t, os.Mkdir(bin, 0o700))
		exe := filepath.Join(bin, "anytype-mcp")
		require.NoError(t, os.WriteFile(exe, []byte("binary"), 0o755))
		chmodACL(t, "everyone allow read", exe)
		deniedOn(t, f.CheckExecutable(exe, exe), exe, "the forwarder's executable has an ACL")
		chmodACL(t, "everyone allow add_file,delete_child", bin)
		deniedOn(t, f.CheckExecutable(exe, exe), bin, "has an ACL")
	})
}
