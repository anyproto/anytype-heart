//go:build windows

package store

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	aceObjectInherit    = 0x1
	aceContainerInherit = 0x2
	fileAllAccess       = 0x1F01FF
	accessAllowedACE    = 0
	aceInheritOnly      = 0x8
	accessDeniedACE     = 1
	callbackAllowedACE  = 9
	// rights that let their holder rename or take over what is inside a
	// folder: FILE_DELETE_CHILD, WRITE_DAC, WRITE_OWNER, GENERIC_ALL
	takeoverRights = 0x40 | 0x40000 | 0x80000 | 0x10000000
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

// trusted reports whether sid is the user, SYSTEM or the Administrators
// group. Administrators are trusted: they can read any file anyway.
func trusted(sid, user *windows.SID) bool {
	if sid == nil {
		return false
	}
	return sid.Equals(user) || sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

// checkGrants is the rule for a folder the store did not create with its
// own descriptor. strict (.anytype, ai-connect): the owner and every
// principal an allow entry names for the folder itself must be the user,
// SYSTEM or Administrators; inherited entries are fine, so a standard
// profile passes. Not strict (the home folder): only that nobody else may
// delete or rename its children or take the folder over.
func checkGrants(dir string, strict bool) error {
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return unreachable(err)
	}
	user, err := userSID()
	if err != nil {
		return unreachable(err)
	}
	if strict {
		owner, _, err := sd.Owner()
		if err != nil || !trusted(owner, user) {
			return deniedAt(dir, "a folder on the item path belongs to another user")
		}
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		// no DACL at all grants everything to everyone
		return deniedAt(dir, "a folder on the item path grants access to everyone")
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil || ace == nil {
			return deniedAt(dir, "a folder on the item path has an unreadable access list")
		}
		switch ace.Header.AceType {
		case accessDeniedACE:
			continue
		case accessAllowedACE, callbackAllowedACE:
		default:
			if strict {
				return deniedAt(dir, "a folder on the item path has an access entry of an unknown kind")
			}
			continue
		}
		if ace.Header.AceFlags&aceInheritOnly != 0 {
			continue // applies to children only, and each child is checked itself
		}
		if trusted((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user) {
			continue
		}
		if strict {
			return deniedAt(dir, "a folder on the item path grants access to another account")
		}
		if uint32(ace.Mask)&takeoverRights != 0 {
			return deniedAt(dir, "another account may rename or take over folders in the home folder")
		}
	}
	return nil
}

// ownSD is the descriptor of .anytype and ai-connect when the store
// creates them: a protected DACL for the user, SYSTEM and Administrators,
// so the folder passes its own rule whatever the home folder would have
// handed down (a home that Users may read).
func ownSD() (*windows.SecurityAttributes, error) {
	sid, err := userSID()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}, nil
}

// mkdir creates one missing folder with its descriptor in the same call,
// never tightened afterwards: the items folder private to the user, the
// folders above it for the user, SYSTEM and Administrators.
func (f *File) mkdir(dir string, kind int) error {
	sa, err := securityAttributes(true)
	if kind != kindItems {
		sa, err = ownSD()
	}
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
// symlink or a junction); the home folder, .anytype and ai-connect may
// grant access to the user, SYSTEM and Administrators (see checkGrants);
// the items folder must carry exactly the private descriptor.
func (f *File) checkDir(dir string, fi os.FileInfo, kind int) error {
	if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !fi.IsDir() {
		return deniedAt(dir, "a folder on the item path is not a real directory")
	}
	if kind != kindItems {
		return checkGrants(dir, kind == kindAnytype)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return unreachable(err)
	}
	if err := checkSD(sd, true); err != nil {
		return deniedAt(dir, "the items folder is not private to the user")
	}
	return nil
}

func notFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

// readItem is the item rule. The file is opened without following a
// reparse point and with FILE_SHARE_DELETE, so a reader never blocks the
// replacement of a writer; the open handle must be a plain file with one
// link and exactly the private descriptor.
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
		return nil, deniedAt(filepath.Join(dir, name), "the item is not a regular file")
	}
	if info.NumberOfLinks != 1 {
		return nil, deniedAt(filepath.Join(dir, name), "the item has more than one link")
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, unreachable(err)
	}
	if err := checkSD(sd, false); err != nil {
		return nil, deniedAt(filepath.Join(dir, name), "the item is not private to the user")
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
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, sa,
		windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, unreachable(err)
	}
	return os.NewFile(uintptr(h), path), nil
}

// busy reports a failure that passes when whoever holds the file lets go:
// a handle opened without delete sharing (an antivirus scan).
func busy(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// settle makes a finished rename durable: the item is opened for writing
// and flushed (FlushFileBuffers) before the receipt is given.
func settle(dir, name string) error {
	fh, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if err := fh.Sync(); err != nil {
		ignore(fh.Close())
		return err
	}
	return fh.Close()
}

// syncParent: a new folder is not flushed on Windows; nothing holding a
// key depends on it, and the first write flushes the item itself.
func syncParent(string) error { return nil }

// syncDir flushes the folder's handle (FlushFileBuffers), so a removed
// item is gone durably before the receipt is given. A failure makes the
// delete unconfirmed.
func syncDir(dir string) error {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	err = windows.FlushFileBuffers(h)
	ignore(windows.CloseHandle(h))
	return err
}
