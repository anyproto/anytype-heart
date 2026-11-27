package files

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Windows has no non-blocking open of a file by name, and no FIFOs in a
// folder.
const openNonblock = 0

// checkOwner is the folder rule: it belongs to this user, or to the
// Administrators group (the owner Windows records for what an elevated
// user creates).
func checkOwner(path string, _ os.FileInfo) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("the files folder's owner can't be read: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("the files folder has no owner: %s", path)
	}
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("the current user can't be read: %w", err)
	}
	if owner.Equals(tu.User.Sid) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		return nil
	}
	return fmt.Errorf("the files folder belongs to another user: %s", path)
}
