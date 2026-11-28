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
	accessAllowedACE   = 0
	aceInheritOnly     = 0x8
	accessDeniedACE    = 1
	callbackAllowedACE = 9
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

// trusted reports whether sid is the user, SYSTEM or the Administrators
// group. Administrators are trusted: they can read any file anyway.
func trusted(sid, user *windows.SID) bool {
	if sid == nil {
		return false
	}
	return sid.Equals(user) || sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

// checkSD judges a descriptor. The owner must be the user, SYSTEM or
// Administrators. strict (.anytype, ai-connect, the folders below
// .anytype, the endpoint file and the executable): so must every principal
// an allow entry names for the object itself; inherited entries are fine,
// so a standard profile passes. Not strict (the home folder): only that
// nobody else may delete or rename its children or take the folder over.
func checkSD(sd *windows.SECURITY_DESCRIPTOR, path string, strict bool) error {
	user, err := userSID()
	if err != nil {
		return unreachable(path, err)
	}
	// an owner may always rewrite the access list, so it is judged on
	// every component, home included
	owner, _, err := sd.Owner()
	if err != nil || !trusted(owner, user) {
		return deniedAt(path, "a file or folder on the endpoint path belongs to another user")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		// no DACL at all grants everything to everyone
		return deniedAt(path, "a file or folder on the endpoint path grants access to everyone")
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil || ace == nil {
			return deniedAt(path, "a file or folder on the endpoint path has an unreadable access list")
		}
		switch ace.Header.AceType {
		case accessDeniedACE:
			continue
		case accessAllowedACE, callbackAllowedACE:
		default:
			if strict {
				return deniedAt(path, "a file or folder on the endpoint path has an access entry of an unknown kind")
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
			return deniedAt(path, "a file or folder on the endpoint path grants access to another account")
		}
		if uint32(ace.Mask)&takeoverRights != 0 {
			return deniedAt(path, "another account may rename or take over folders in the home folder")
		}
	}
	return nil
}

// checkDir is the folder rule: no component may be a reparse point (a
// symlink or a junction), and its descriptor passes checkSD.
func (f *File) checkDir(dir string, fi os.FileInfo, kind int) error {
	if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !fi.IsDir() {
		return deniedAt(dir, "a folder on the endpoint path is not a real directory")
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return unreachable(dir, err)
	}
	return checkSD(sd, dir, kind == kindStrict)
}

func notFound(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

// openChecked is the file rule. The file is opened without following a
// reparse point; the open handle must be a plain file with one link whose
// descriptor passes the strict rule. what names the file in the rule's
// text.
func (f *File) openChecked(dir, name, what string) (*os.File, error) {
	path := filepath.Join(dir, name)
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, unreachable(path, err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		// BACKUP_SEMANTICS opens a folder too, so one at this name is
		// refused by the rule below, not reported as unreadable
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		if notFound(err) {
			return nil, missing(path)
		}
		return nil, unreachable(path, err)
	}
	fh := os.NewFile(uintptr(h), path)
	fail := func(err error) (*os.File, error) {
		_ = fh.Close()
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return fail(unreachable(path, err))
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		return fail(deniedAt(path, what+" is not a regular file"))
	}
	if info.NumberOfLinks != 1 {
		return fail(deniedAt(path, what+" has more than one link"))
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fail(unreachable(path, err))
	}
	if err := checkSD(sd, path, true); err != nil {
		return fail(err)
	}
	return fh, nil
}
