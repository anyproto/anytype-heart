package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows has no non-blocking open of a file by name, and no FIFOs in a
// folder.
const openNonblock = 0

const (
	accessAllowedACE   = 0
	accessDeniedACE    = 1
	callbackAllowedACE = 9
)

func userSID() (*windows.SID, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return tu.User.Sid, nil
}

// mkdirPrivate creates one folder with its own protected access list:
// full access for the user, SYSTEM and Administrators, inherited by what
// is created in it, and nothing inherited from the parent.
func mkdirPrivate(path string) error {
	user, err := userSID()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(p, sa)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return fs.ErrExist
	}
	return err
}

// openFolder opens the folder itself, never what a link or a junction at
// its name points to.
func openFolder(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

func trusted(sid, user *windows.SID) bool {
	return sid != nil && (sid.Equals(user) || sid.IsWellKnown(windows.WinLocalSystemSid) ||
		sid.IsWellKnown(windows.WinBuiltinAdministratorsSid))
}

// checkFolder is the folder rule, read off the open folder: it belongs to
// this user or to Administrators (the owner Windows records for what an
// elevated user creates), and its access list grants access to the user,
// SYSTEM and Administrators only, entries that only its children inherit
// included: a download inherits them.
func checkFolder(h *os.File, _ os.FileInfo, path string) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(h.Fd()), windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("the files folder's access list can't be read: %w", err)
	}
	user, err := userSID()
	if err != nil {
		return fmt.Errorf("the current user can't be read: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !(owner.Equals(user) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid)) {
		return fmt.Errorf("the files folder belongs to another user: %s", path)
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("the files folder grants access to everyone: %s", path)
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil || ace == nil {
			return fmt.Errorf("the files folder has an unreadable access list: %s", path)
		}
		switch ace.Header.AceType {
		case accessDeniedACE:
			continue
		case accessAllowedACE, callbackAllowedACE:
		default:
			return fmt.Errorf("the files folder has an access entry of an unknown kind: %s", path)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		// CREATOR OWNER stands for whoever creates a child: this user
		if trusted(sid, user) || sid.IsWellKnown(windows.WinCreatorOwnerSid) {
			continue
		}
		return fmt.Errorf("the files folder grants access to another account (%s): %s", sid.String(), path)
	}
	return nil
}
