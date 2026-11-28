//go:build windows

package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// These tests run only on the CI Windows job.

// setDACL replaces a path's DACL. protected says whether it stops
// inheriting from the parent.
func setDACL(t *testing.T, path, s string, protected bool) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(s)
	require.NoError(t, err)
	dacl, _, err := sd.DACL()
	require.NoError(t, err)
	info := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	if protected {
		info = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	require.NoError(t, windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, info, nil, nil, dacl, nil))
}

func trustedOnly(t *testing.T) string {
	sid, err := userSID()
	require.NoError(t, err)
	user := sid.String()
	return "D:P(A;OICI;FA;;;" + user + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
}

// privateHome gives a scratch home a normal profile's access list (the
// user, SYSTEM and Administrators, inherited by what is made in it), so no
// test depends on what the runner's temporary folder hands down.
func privateHome(t *testing.T, home string) { setDACL(t, home, trustedOnly(t), true) }

func seeded(t *testing.T) *File {
	f := newFile(t)
	write(t, f, `{"v":1,"port":31009}`)
	_, err := f.Read(ctx)
	require.NoError(t, err, "a normal profile: everything inherits the user's, SYSTEM's and Administrators' entries")
	return f
}

func deniedOn(t *testing.T, err error, path, rule string) {
	t.Helper()
	e := requireReason(t, err, ReasonDenied, rule)
	assert.Equal(t, path, e.Path)
	assert.Contains(t, e.Rule, rule)
}

// .anytype, ai-connect, the file and the executable may grant access to
// the user, SYSTEM and Administrators and to nobody else; inherited
// entries are judged like their own.
func TestWindowsGrants(t *testing.T) {
	trusted := trustedOnly(t)

	t.Run("ai-connect with an entry for Users", func(t *testing.T) {
		f := seeded(t)
		dir := filepath.Dir(f.Path())
		setDACL(t, dir, trusted+"(A;;FA;;;BU)", true)
		_, err := f.Read(ctx)
		deniedOn(t, err, dir, "grants access to another account")
		setDACL(t, dir, trusted, true)
		_, err = f.Read(ctx)
		require.NoError(t, err)
	})

	t.Run("the file with a read entry for Users", func(t *testing.T) {
		f := seeded(t)
		setDACL(t, f.Path(), trusted+"(A;;FR;;;BU)", true)
		_, err := f.Read(ctx)
		deniedOn(t, err, f.Path(), "grants access to another account")
	})

	t.Run("an inherit-only entry for others on .anytype does not apply to it", func(t *testing.T) {
		f := seeded(t)
		dir := filepath.Dir(f.Path())
		// ai-connect keeps its own list, so nothing is handed down to it
		setDACL(t, dir, trusted, true)
		setDACL(t, filepath.Join(f.home, ".anytype"), trusted+"(A;OICIIO;FR;;;BU)", true)
		_, err := f.Read(ctx)
		require.NoError(t, err)
	})

	t.Run("an .anytype that inherited an entry for Users from home", func(t *testing.T) {
		home := t.TempDir()
		setDACL(t, home, trusted+"(A;OICI;0x1200a9;;;BU)", true) // Users may read and execute
		f := OpenAt(home, NamespaceDev)
		require.NoError(t, os.MkdirAll(filepath.Dir(f.Path()), 0o700))
		write(t, f, `{"v":1,"port":1}`)
		_, err := f.Read(ctx)
		deniedOn(t, err, filepath.Join(home, ".anytype"), "grants access to another account")
	})

	t.Run("a home others may read is fine; one where they may delete children is not", func(t *testing.T) {
		f := seeded(t)
		// .anytype keeps its own list, so home's entries stop there
		setDACL(t, filepath.Join(f.home, ".anytype"), trusted, true)
		setDACL(t, f.home, trusted+"(A;OICI;0x1200a9;;;BU)", true)
		_, err := f.Read(ctx)
		require.NoError(t, err)
		setDACL(t, f.home, trusted+"(A;OICI;FA;;;BU)", true)
		_, err = f.Read(ctx)
		deniedOn(t, err, f.home, "rename or take over")
	})

	t.Run("an executable whose folder grants Users", func(t *testing.T) {
		f := seeded(t)
		bin := filepath.Join(f.home, ".anytype", "bin")
		require.NoError(t, os.Mkdir(bin, 0o700))
		exe := filepath.Join(bin, "anytype-mcp.exe")
		require.NoError(t, os.WriteFile(exe, []byte("binary"), 0o700))
		require.NoError(t, f.CheckExecutable(exe, exe))
		setDACL(t, bin, trusted+"(A;;FA;;;BU)", true)
		deniedOn(t, f.CheckExecutable(exe, exe), bin, "grants access to another account")
	})
}

func TestWindowsLinks(t *testing.T) {
	t.Run("ai-connect as a junction", func(t *testing.T) {
		f := seeded(t)
		dir := filepath.Dir(f.Path())
		real := dir + "-real"
		require.NoError(t, os.Rename(dir, real))
		out, err := exec.Command("cmd", "/c", "mklink", "/J", dir, real).CombinedOutput()
		if err != nil {
			t.Skipf("no junctions here: %v: %s", err, out)
		}
		_, err = f.Read(ctx)
		deniedOn(t, err, dir, "not a real directory")
	})

	t.Run("the file as a symlink", func(t *testing.T) {
		f := seeded(t)
		good := filepath.Join(t.TempDir(), "good.json")
		require.NoError(t, os.WriteFile(good, []byte(`{"v":1,"port":1}`), 0o600))
		require.NoError(t, os.Remove(f.Path()))
		if err := os.Symlink(good, f.Path()); err != nil {
			t.Skip("this account may not create symlinks")
		}
		_, err := f.Read(ctx)
		deniedOn(t, err, f.Path(), "not a regular file")
	})

	t.Run("the file and the executable with two links", func(t *testing.T) {
		f := seeded(t)
		require.NoError(t, os.Link(f.Path(), filepath.Join(f.home, "second")))
		_, err := f.Read(ctx)
		deniedOn(t, err, f.Path(), "more than one link")

		bin := filepath.Join(f.home, ".anytype", "bin")
		require.NoError(t, os.Mkdir(bin, 0o700))
		exe := filepath.Join(bin, "anytype-mcp.exe")
		require.NoError(t, os.WriteFile(exe, []byte("binary"), 0o700))
		require.NoError(t, os.Link(exe, filepath.Join(f.home, "copy.exe")))
		deniedOn(t, f.CheckExecutable(exe, exe), exe, "more than one link")
	})
}

// Theme E: the owner of every component is judged, home included: an
// owner may rewrite the access list whatever it says now.
func TestH1R1_E_HomeOwnerIsJudged(t *testing.T) {
	sid, err := userSID()
	require.NoError(t, err)
	for owner, ok := range map[string]bool{sid.String(): true, "SY": true, "BA": true, "BU": false, "WD": false} {
		sd, err := windows.SecurityDescriptorFromString("O:" + owner + "D:P(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)")
		require.NoError(t, err)
		for _, strict := range []bool{false, true} {
			err := checkSD(sd, `C:\home`, strict)
			if ok {
				assert.NoError(t, err, "%s strict=%v", owner, strict)
				continue
			}
			deniedOn(t, err, `C:\home`, "belongs to another user")
		}
	}
}

// BLD-06: a folder at the endpoint's name is refused by the file rule.
func TestH1R1_BLD06_FolderAtTheEndpointName(t *testing.T) {
	f := newFile(t)
	require.NoError(t, os.Mkdir(f.Path(), 0o700))
	_, err := f.Read(ctx)
	deniedOn(t, err, f.Path(), "not a regular file")
}

// COR-12: a reader holds the file open with delete sharing, so the
// desktop's replacement and removal never wait for it.
func TestH1R1_COR12_ReaderDoesNotBlockTheWriter(t *testing.T) {
	f := seeded(t)
	fh, err := f.openChecked(filepath.Dir(f.Path()), filepath.Base(f.Path()), "the endpoint file")
	require.NoError(t, err)
	defer fh.Close()
	next := f.Path() + ".tmp"
	require.NoError(t, os.WriteFile(next, []byte(`{"v":1,"port":2}`), 0o600))
	require.NoError(t, os.Rename(next, f.Path()), "a replacement while the reader is open")
	port, err := f.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, port)
	require.NoError(t, os.Remove(f.Path()), "and a removal")
}
