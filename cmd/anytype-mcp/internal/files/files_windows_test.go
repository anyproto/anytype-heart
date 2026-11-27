package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func sddl(t *testing.T, path string) string {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	return sd.String()
}

func setDACL(t *testing.T, path, s string) {
	sd, err := windows.SecurityDescriptorFromString(s)
	require.NoError(t, err)
	dacl, _, err := sd.DACL()
	require.NoError(t, err)
	require.NoError(t, windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil))
}

func private(t *testing.T) string {
	user, err := userSID()
	require.NoError(t, err)
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	require.NoError(t, err)
	return sd.String()
}

// Private by construction: what the forwarder creates has its own
// protected access list, whatever the parent would hand down.
func TestCreatedFoldersArePrivate(t *testing.T) {
	// a parent that hands every child an entry for Users
	parent := t.TempDir()
	setDACL(t, parent, private(t)+"(A;OICI;FA;;;BU)")

	t.Run("the temporary default", func(t *testing.T) {
		t.Setenv("TMP", parent)
		t.Setenv("TEMP", parent)
		d, err := Temp()
		require.NoError(t, err)
		defer d.Close()
		resolvedParent, err := filepath.EvalSymlinks(parent)
		require.NoError(t, err)
		require.Equal(t, resolvedParent, filepath.Dir(d.Path()))
		assert.Equal(t, private(t), sddl(t, d.Path()))
		saved, _, err := d.Save(ctx, "a.txt", strings.NewReader("x"))
		require.NoError(t, err)
		assert.NotContains(t, sddl(t, filepath.Join(d.Path(), saved)), ";BU)", "a download inherits the folder's list only")
	})

	t.Run("a missing --files-dir", func(t *testing.T) {
		path := filepath.Join(parent, "made")
		d, err := Open(path)
		require.NoError(t, err)
		defer d.Close()
		assert.Equal(t, private(t), sddl(t, path))
	})

	t.Run("an existing folder is taken with the access list it has, and keeps it", func(t *testing.T) {
		path := filepath.Join(parent, "inherits")
		require.NoError(t, os.Mkdir(path, 0o700))
		before := sddl(t, path)
		d, err := Open(path)
		require.NoError(t, err)
		_ = d.Close()
		assert.Equal(t, before, sddl(t, path))
	})
}

// A junction needs no privilege to make, so it is the link a prompt-steered
// agent could plant on Windows.
func junction(t *testing.T, target, name string) {
	t.Helper()
	out, err := exec.Command("cmd", "/c", "mklink", "/J", name, target).CombinedOutput()
	if err != nil {
		t.Skipf("no junctions here: %v: %s", err, out)
	}
}

func TestJunctions(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.Mkdir(outside, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not yours"), 0o600))

	t.Run("a junction as the folder's path is the user's choice: its target is pinned", func(t *testing.T) {
		target := filepath.Join(base, "target")
		require.NoError(t, os.Mkdir(target, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(target, "a.txt"), []byte("chosen"), 0o600))
		junction(t, target, filepath.Join(base, "linked"))
		d, err := Open(filepath.Join(base, "linked"))
		require.NoError(t, err)
		defer d.Close()
		t.Logf("pinned as %s", d.Path())
		assert.True(t, d.Located(), "the path still names the pinned folder")
		f, _, err := d.OpenUpload("a.txt")
		require.NoError(t, err)
		_ = f.Close()
		_, _, err = d.OpenUpload(`..\outside\secret.txt`)
		require.Error(t, err)
	})

	t.Run("a junction inside the folder that leads out is not followed", func(t *testing.T) {
		d, err := Open(filepath.Join(base, "files"))
		require.NoError(t, err)
		defer d.Close()
		root := d.Path()
		junction(t, outside, filepath.Join(root, "out"))
		_, _, err = d.OpenUpload("out/secret.txt")
		assert.Contains(t, refusal(t, err), root)
		_, _, err = d.OpenUpload(`out\secret.txt`)
		assert.Contains(t, refusal(t, err), root)
	})

	t.Run("Windows spellings of a way out are refused", func(t *testing.T) {
		d, err := Open(filepath.Join(base, "files2"))
		require.NoError(t, err)
		defer d.Close()
		root := d.Path()
		for _, path := range []string{`..\outside\secret.txt`, `sub\..\..\outside\secret.txt`, filepath.Join(outside, "secret.txt"),
			`\\?\` + filepath.Join(outside, "secret.txt"), `\\localhost\c$\Windows\win.ini`, "NUL", "CON", `sub\COM1`, "a.txt:stream", "a.txt::$DATA"} {
			_, _, err := d.OpenUpload(path)
			assert.Contains(t, refusal(t, err), root, path)
		}
	})
}
