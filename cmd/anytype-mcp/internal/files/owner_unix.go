//go:build !windows

package files

import (
	"fmt"
	"os"
	"syscall"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

const openNonblock = syscall.O_NONBLOCK

// mkdirPrivate creates one folder for the user alone. The mode is never
// set afterwards by name: the umask can only narrow it.
func mkdirPrivate(path string) error { return os.Mkdir(path, 0o700) }

// openFolder opens the folder itself, never what a link at its name
// points to.
func openFolder(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// checkFolder is the folder rule, read off the open folder: it belongs to
// this user; neither its group nor others can write it (they could plant
// a file for upload or swap a download); and no ACL grants anybody access
// beside the mode bits (macOS).
func checkFolder(h *os.File, fi os.FileInfo, path string) error {
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
	grants, err := store.ACLGrants(h)
	if err != nil {
		return fmt.Errorf("the files folder's access list can't be read: %w", err)
	}
	if grants > 0 {
		return fmt.Errorf("the files folder has an access list (ACL) that grants access: %s", path)
	}
	return nil
}
