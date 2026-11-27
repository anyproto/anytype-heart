package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// macOS reads an ACL before the mode bits: a 0700 folder may still grant
// another user access, and hand that on to every download.
func TestFolderWithAnACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "files")
	require.NoError(t, os.Mkdir(path, 0o700))
	if out, err := exec.Command("chmod", "+a", "everyone allow read,write,file_inherit,directory_inherit", path).CombinedOutput(); err != nil {
		t.Skipf("no ACLs here: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("chmod", "-N", path).Run() })
	_, err := Open(path)
	require.ErrorContains(t, err, "access list (ACL) that grants access")

	require.NoError(t, exec.Command("chmod", "-N", path).Run())
	require.NoError(t, exec.Command("chmod", "+a", "everyone deny delete", path).Run())
	d, err := Open(path)
	require.NoError(t, err, "an entry that only denies is fine")
	_ = d.Close()
}
