//go:build !windows

package files

import (
	"fmt"
	"os"
	"syscall"
)

const openNonblock = syscall.O_NONBLOCK

// checkOwner is the folder rule: it belongs to this user, and neither its
// group nor others can write it (they could plant a file for upload or
// swap a download).
func checkOwner(path string, fi os.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("the files folder has no owner: %s", path)
	}
	if int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("the files folder belongs to another user: %s", path)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("the files folder is writable by other users (chmod go-w): %s", path)
	}
	return nil
}
