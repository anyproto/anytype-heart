//go:build windows

package store

import (
	"errors"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	aceObjectInherit    = 0x1
	aceContainerInherit = 0x2
	fileAllAccess       = 0x1F01FF
	accessAllowedACE    = 0
	renameRetryFor      = 2 * time.Second
)

// userSID is the SID of the user this process runs as.
func userSID() (*windows.SID, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return tu.User.Sid, nil
}

// privateSD is the only descriptor an items folder or an item may have:
// owned by the user, a protected DACL with one entry, full access for the
// user. A folder's entry is inherited by what is created in it.
func privateSD(folder bool) (*windows.SECURITY_DESCRIPTOR, error) {
	sid, err := userSID()
	if err != nil {
		return nil, err
	}
	flags := ""
	if folder {
		flags = "OICI"
	}
	return windows.SecurityDescriptorFromString("O:" + sid.String() + "D:P(A;" + flags + ";FA;;;" + sid.String() + ")")
}

// checkSD requires exactly privateSD: the owner, a protected DACL, and one
// allow entry of full access for the user with the expected inheritance.
func checkSD(sd *windows.SECURITY_DESCRIPTOR, folder bool) error {
	sid, err := userSID()
	if err != nil {
		return unreachable(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.Equals(sid) {
		return denied("the item path belongs to another user")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return denied("the item path inherits access from its parent")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		return denied("the item path grants access to others")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil || ace == nil {
		return denied("the item path grants access to others")
	}
	want := uint8(0)
	if folder {
		want = aceObjectInherit | aceContainerInherit
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if ace.Header.AceType != accessAllowedACE || ace.Header.AceFlags != want ||
		uint32(ace.Mask) != fileAllAccess || !aceSID.Equals(sid) {
		return denied("the item path grants access to others")
	}
	return nil
}

func securityAttributes(folder bool) (*windows.SecurityAttributes, error) {
	sd, err := privateSD(folder)
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}, nil
}

// mkdir creates one missing folder. The items folder is created with its
// private descriptor in the same call, never tightened afterwards.
func (f *File) mkdir(dir string, items bool) error {
	if !items {
		return os.Mkdir(dir, 0o700)
	}
	sa, err := securityAttributes(true)
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	err = windows.CreateDirectory(p, sa)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return os.ErrExist
	}
	return err
}

// checkDir is the folder rule: no component may be a reparse point (a
// symlink or a junction), and the items folder must carry exactly the
// private descriptor.
func (f *File) checkDir(dir string, fi os.FileInfo, items bool) error {
	if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !fi.IsDir() {
		return denied("a folder on the item path is not a real directory")
	}
	if !items {
		return nil
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return unreachable(err)
	}
	return checkSD(sd, true)
}

func notFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

// readItem is the item rule. The file is opened without following a
// reparse point and with FILE_SHARE_DELETE, so a reader never blocks the
// rename of a writer; the open handle must be a plain file with one link
// and exactly the private descriptor.
func (f *File) readItem(dir, name string) ([]byte, error) {
	path := filepath.Join(dir, name)
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, unreachable(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		if notFound(err) {
			return nil, ErrNotFound
		}
		return nil, unreachable(err)
	}
	fh := os.NewFile(uintptr(h), path)
	defer fh.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return nil, unreachable(err)
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		return nil, denied("the item is not a regular file")
	}
	if info.NumberOfLinks != 1 {
		return nil, denied("the item has more than one link")
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, unreachable(err)
	}
	if err := checkSD(sd, false); err != nil {
		return nil, err
	}
	return readChecked(fh)
}

// createPrivate makes a new file that did not exist, with the private
// descriptor from its first moment.
func (f *File) createPrivate(dir, name string) (*os.File, error) {
	sa, err := securityAttributes(false)
	if err != nil {
		return nil, unreachable(err)
	}
	path := filepath.Join(dir, name)
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, unreachable(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, unreachable(err)
	}
	return os.NewFile(uintptr(h), path), nil
}

// renameIn replaces the item. Something that holds the old item open
// without FILE_SHARE_DELETE (an antivirus scan) makes the rename fail for
// a moment: it is retried for up to two seconds.
func renameIn(dir, from, to string) error {
	src, err := windows.UTF16PtrFromString(filepath.Join(dir, from))
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(filepath.Join(dir, to))
	if err != nil {
		return err
	}
	deadline := time.Now().Add(renameRetryFor)
	for {
		err = windows.MoveFileEx(src, dst, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		busy := errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
		if err == nil || !busy || time.Now().After(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func removeIn(dir, name string) error { return os.Remove(filepath.Join(dir, name)) }

// syncDir: NTFS journals the rename; there is no directory handle to sync.
func syncDir(string) error { return nil }
