//go:build windows

package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// These tests have only ever run on the CI Windows job.

func sddl(t *testing.T, path string) string {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	return sd.String()
}

// setDACL replaces a path's DACL. protected says whether it stops
// inheriting from the parent.
func setDACL(t *testing.T, path, s string, protected bool) {
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

// canonical is a descriptor string as Windows prints it: it spells some
// accounts by an alias (the runner's user is "LA").
func canonical(t *testing.T, s string) string {
	sd, err := windows.SecurityDescriptorFromString(s)
	require.NoError(t, err)
	return sd.String()
}

func me(t *testing.T) string {
	sid, err := userSID()
	require.NoError(t, err)
	return sid.String()
}

func TestWindowsDescriptors(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("x")), "the runner's standard inherited DACLs pass")
	user := me(t)
	assert.Equal(t, canonical(t, "O:"+user+"D:P(A;OICI;FA;;;"+user+")"), sddl(t, f.Dir()))
	item := filepath.Join(f.Dir(), "a.item")
	assert.Equal(t, canonical(t, "O:"+user+"D:P(A;;FA;;;"+user+")"), sddl(t, item))

	t.Run("an item whose DACL also lets Everyone in", func(t *testing.T) {
		setDACL(t, item, "D:P(A;;FA;;;"+user+")(A;;FR;;;WD)", true)
		_, err := f.Read(ctx, "a")
		requireDenied(t, err, "extra Everyone entry")
		setDACL(t, item, "D:P(A;;FA;;;"+user+")", true)
		_, err = f.Read(ctx, "a")
		require.NoError(t, err)
	})

	t.Run("an item that inherits its DACL from the folder", func(t *testing.T) {
		plain := filepath.Join(f.Dir(), "plain.item")
		require.NoError(t, os.WriteFile(plain, []byte("x"), 0o600))
		_, err := f.Read(ctx, "plain")
		requireDenied(t, err, "inherited DACL")
	})

	t.Run("an items folder whose DACL also lets Everyone in", func(t *testing.T) {
		setDACL(t, f.Dir(), "D:P(A;OICI;FA;;;"+user+")(A;OICI;FR;;;WD)", true)
		_, err := f.Read(ctx, "a")
		requireDenied(t, err, "folder with an Everyone entry")
		requireDenied(t, f.Write(ctx, "a", []byte("y")), "folder with an Everyone entry")
		setDACL(t, f.Dir(), "D:P(A;OICI;FA;;;"+user+")", true)
	})
}

// L4: .anytype and ai-connect may grant access to the user, SYSTEM and
// Administrators and to nobody else; the home folder only must not let
// anybody else rename its children.
func TestWindowsFolderGrants(t *testing.T) {
	user := me(t)
	trustedOnly := "D:P(A;OICI;FA;;;" + user + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

	t.Run("ai-connect with an entry for Users", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("kept")))
		dir := filepath.Dir(f.Dir())
		setDACL(t, dir, trustedOnly+"(A;;FA;;;BU)", true)
		_, err := f.Read(ctx, "a")
		requireDenied(t, err, "Users on ai-connect")
		assert.Contains(t, err.Error(), dir)
		requireDenied(t, f.Write(ctx, "a", []byte("new")), "Users on ai-connect")
		requireDenied(t, f.Delete(ctx, "a"), "Users on ai-connect")
		setDACL(t, dir, trustedOnly, true)
		raw, err := f.Read(ctx, "a")
		require.NoError(t, err, "the user, SYSTEM and Administrators are fine")
		assert.Equal(t, "kept", string(raw))
	})

	t.Run("an inherit-only entry for others on .anytype does not apply to the folder itself", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("x")))
		// ai-connect, made by the store, has its own protected list and
		// inherits nothing from it
		setDACL(t, filepath.Join(f.home, ".anytype"), trustedOnly+"(A;OICIIO;FR;;;BU)", true)
		_, err := f.Read(ctx, "a")
		require.NoError(t, err)
	})

	t.Run("a home others may read is fine; one where they may delete children is not", func(t *testing.T) {
		f := newFile(t)
		require.NoError(t, f.Write(ctx, "a", []byte("x")))
		setDACL(t, f.home, trustedOnly+"(A;OICI;0x1200a9;;;BU)", true) // read and execute
		_, err := f.Read(ctx, "a")
		require.NoError(t, err)
		setDACL(t, f.home, trustedOnly+"(A;OICI;FA;;;BU)", true)
		_, err = f.Read(ctx, "a")
		requireDenied(t, err, "Users with full control of home")
		setDACL(t, f.home, trustedOnly, true)
	})
}

// N9: the folders the store creates carry their own protected list, so a
// home that Users may read hands nothing down to them. A folder somebody
// else made keeps the rule.
func TestWindowsCreatedFoldersAreProtected(t *testing.T) {
	user := me(t)
	trustedOnly := "D:P(A;OICI;FA;;;" + user + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	f := newFile(t)
	setDACL(t, f.home, trustedOnly+"(A;OICI;0x1200a9;;;BU)", true) // Users may read and execute
	require.NoError(t, f.Write(ctx, "a", []byte("x")), "a first write under a home others may read")
	raw, err := f.Read(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, "x", string(raw))
	want := canonical(t, trustedOnly)
	for _, dir := range []string{filepath.Join(f.home, ".anytype"), filepath.Dir(f.Dir())} {
		sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		require.NoError(t, err)
		assert.Equal(t, want, sd.String(), dir)
	}

	t.Run("an .anytype made by somebody else that inherited the entry is still refused, with the rule named", func(t *testing.T) {
		g := newFile(t)
		setDACL(t, g.home, trustedOnly+"(A;OICI;0x1200a9;;;BU)", true)
		require.NoError(t, os.Mkdir(filepath.Join(g.home, ".anytype"), 0o700))
		err := g.Write(ctx, "a", []byte("x"))
		requireDenied(t, err, "inherited Users entry")
		assert.Contains(t, err.Error(), "grants access to another account")
	})
}

func openItem(t *testing.T, f *File, share uint32) windows.Handle {
	p, err := windows.UTF16PtrFromString(filepath.Join(f.Dir(), "a.item"))
	require.NoError(t, err)
	h, err := windows.CreateFile(p, windows.GENERIC_READ, share, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	return h
}

// L7: a reader that shares delete (the way the store opens items) never
// blocks a replacement.
func TestWindowsReaderDoesNotBlockTheRename(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("one")))
	h := openItem(t, f, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
	defer windows.CloseHandle(h)
	require.NoError(t, f.Write(ctx, "a", []byte("two")))
	raw, err := f.Read(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, "two", string(raw))
	require.NoError(t, f.Delete(ctx, "a"), "nor a delete")
}

// L7: a handle that denies delete sharing makes the write give up after
// the bounded retry: nothing written, the old item intact, no temp file.
func TestWindowsBusyItem(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("one")))
	h := openItem(t, f, windows.FILE_SHARE_READ)
	released := time.AfterFunc(3*time.Second, func() { _ = windows.CloseHandle(h) })
	defer released.Stop()
	began := time.Now()
	err := f.Write(ctx, "a", []byte("two"))
	require.True(t, IsTemporary(err), "busy, nothing written: %v", err)
	assert.GreaterOrEqual(t, time.Since(began), busyRetryFor-200*time.Millisecond)
	assert.Less(t, time.Since(began), 3*time.Second)
	time.Sleep(time.Until(began.Add(3200 * time.Millisecond)))
	raw, err := f.Read(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, "one", string(raw))
	assert.Empty(t, tempFiles(t, f.Dir()))
	require.NoError(t, f.Write(ctx, "a", []byte("two")), "and the next write goes through")
}

func TestWindowsFolderLink(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("x")))
	real := f.Dir() + "-real"
	require.NoError(t, os.Rename(f.Dir(), real))
	if err := os.Symlink(real, f.Dir()); err != nil {
		t.Skip("this account may not create symlinks")
	}
	_, err := f.Read(ctx, "a")
	requireDenied(t, err, "folder symlink")
	requireDenied(t, f.Write(ctx, "a", []byte("y")), "folder symlink")
}

// M4: a delete is flushed before its receipt.
func TestWindowsDeleteIsDurable(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("one")))
	require.NoError(t, syncDir(f.Dir()), "the items folder's handle can be flushed")
	require.NoError(t, f.Delete(ctx, "a"), "a confirmed delete, not an unconfirmed one")
	_, err := f.Read(ctx, "a")
	require.ErrorIs(t, err, ErrNotFound)

	// when the flush fails, the receipt says so
	require.NoError(t, f.Write(ctx, "a", []byte("two")))
	f.hook = func(stage string) error {
		if stage == "after-unlink" {
			return os.ErrInvalid
		}
		return nil
	}
	var unconfirmed *UnconfirmedError
	require.ErrorAs(t, f.Delete(ctx, "a"), &unconfirmed)
}
