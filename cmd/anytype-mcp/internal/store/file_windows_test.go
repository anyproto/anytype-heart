//go:build windows

package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func sddl(t *testing.T, path string) string {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	return sd.String()
}

func setSDDL(t *testing.T, path, s string) {
	sd, err := windows.SecurityDescriptorFromString(s)
	require.NoError(t, err)
	dacl, _, err := sd.DACL()
	require.NoError(t, err)
	require.NoError(t, windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil))
}

func TestWindowsDescriptors(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("x")))
	sid, err := userSID()
	require.NoError(t, err)
	me := sid.String()
	assert.Equal(t, "O:"+me+"D:P(A;OICI;FA;;;"+me+")", sddl(t, f.Dir()))
	item := filepath.Join(f.Dir(), "a.item")
	assert.Equal(t, "O:"+me+"D:P(A;;FA;;;"+me+")", sddl(t, item))

	t.Run("an item whose DACL also lets Everyone in", func(t *testing.T) {
		setSDDL(t, item, "D:P(A;;FA;;;"+me+")(A;;FR;;;WD)")
		_, err := f.Read(ctx, "a")
		requireDenied(t, err, "extra Everyone entry")
		setSDDL(t, item, "D:P(A;;FA;;;"+me+")")
		_, err = f.Read(ctx, "a")
		require.NoError(t, err)
	})

	t.Run("an items folder whose DACL also lets Everyone in", func(t *testing.T) {
		setSDDL(t, f.Dir(), "D:P(A;OICI;FA;;;"+me+")(A;OICI;FR;;;WD)")
		_, err := f.Read(ctx, "a")
		requireDenied(t, err, "folder with an Everyone entry")
		requireDenied(t, f.Write(ctx, "a", []byte("y")), "folder with an Everyone entry")
		setSDDL(t, f.Dir(), "D:P(A;OICI;FA;;;"+me+")")
	})
}

func TestWindowsReaderDoesNotBlockTheRename(t *testing.T) {
	f := newFile(t)
	require.NoError(t, f.Write(ctx, "a", []byte("one")))
	// a reader the way the store opens items: sharing delete
	p, err := windows.UTF16PtrFromString(filepath.Join(f.Dir(), "a.item"))
	require.NoError(t, err)
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	defer windows.CloseHandle(h)
	require.NoError(t, f.Write(ctx, "a", []byte("two")))
	raw, err := f.Read(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, "two", string(raw))
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
