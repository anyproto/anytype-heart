//go:build !windows

package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// mkdir creates one missing folder private to the user. The mode is set
// again after the fact because of the umask.
func (f *File) mkdir(dir string, _ int) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// checkDir is the folder rule for one path component.
//
// Home must be a real directory owned by this user. Its mode is not
// judged: a home that its group can write (a user-private group) is a
// normal setup, and whoever can write a home can already replace the
// user's shell files. On macOS home may carry deny ACL entries only (every
// Mac home has "everyone deny delete").
//
// .anytype, ai-connect and the items folder must be real directories owned
// by this user that neither group nor others can write, with no ACL at all
// (macOS evaluates ACL entries before the mode bits). The items folder
// must be exactly 0700.
func (f *File) checkDir(dir string, fi os.FileInfo, kind int) error {
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return deniedAt(dir, "a folder on the item path is not a real directory")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return deniedAt(dir, "a folder on the item path has no owner")
	}
	if int(st.Uid) != f.uid() {
		return deniedAt(dir, "a folder on the item path belongs to another user")
	}
	acl, err := pathACL(dir)
	if err != nil {
		return unreachable(err)
	}
	if kind == kindHome {
		if acl.allow > 0 {
			return deniedAt(dir, "the home folder has an ACL that grants access")
		}
		return nil
	}
	if acl.entries > 0 {
		return deniedAt(dir, "a folder on the item path has an ACL")
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return deniedAt(dir, "a folder on the item path is writable by others")
	}
	if kind == kindItems && fi.Mode().Perm() != 0o700 {
		return deniedAt(dir, "the items folder is not private (0700)")
	}
	return nil
}

// readItem is the item rule: the name must be a regular file, and the
// file actually opened must be that same file, owned by this user, mode
// exactly 0600, with one link and no ACL. Access goes through an os.Root
// on the items folder, so nothing leaves it.
func (f *File) readItem(dir, name string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, unreachable(err)
	}
	defer root.Close()
	li, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, unreachable(err)
	}
	if !li.Mode().IsRegular() {
		return nil, deniedAt(filepath.Join(dir, name), "the item is not a regular file")
	}
	// O_NONBLOCK: a FIFO swapped in after the check must not hang the open
	fh, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, unreachable(err)
	}
	defer fh.Close()
	fi, err := fh.Stat()
	if err != nil {
		return nil, unreachable(err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case !ok || !fi.Mode().IsRegular() || !os.SameFile(li, fi):
		return nil, deniedAt(filepath.Join(dir, name), "the item is not a regular file")
	case int(st.Uid) != f.uid():
		return nil, deniedAt(filepath.Join(dir, name), "the item belongs to another user")
	case fi.Mode().Perm() != 0o600:
		return nil, deniedAt(filepath.Join(dir, name), "the item is not private (0600)")
	case st.Nlink != 1:
		return nil, deniedAt(filepath.Join(dir, name), "the item has more than one link")
	}
	acl, err := fileACL(fh)
	if err != nil {
		return nil, unreachable(err)
	}
	if acl.entries > 0 {
		return nil, deniedAt(filepath.Join(dir, name), "the item has an ACL")
	}
	return readChecked(fh)
}

// createPrivate makes a new 0600 file that did not exist. It is checked
// for an inherited ACL before any key byte goes into it.
func (f *File) createPrivate(dir, name string) (*os.File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, unreachable(err)
	}
	defer root.Close()
	fh, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, unreachable(err)
	}
	discard := func(err error) (*os.File, error) {
		ignore(fh.Close())
		ignore(root.Remove(name))
		return nil, err
	}
	if err := fh.Chmod(0o600); err != nil {
		return discard(unreachable(err))
	}
	acl, err := fileACL(fh)
	if err != nil {
		return discard(unreachable(err))
	}
	if acl.entries > 0 {
		return discard(deniedAt(filepath.Join(dir, name), "the new item would inherit an ACL"))
	}
	return fh, nil
}

// busy: nothing on Unix blocks a rename or an unlink.
func busy(error) bool { return false }

// settle makes a finished rename durable.
func settle(dir, _ string) error { return syncDir(dir) }

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// aclInfo counts the entries of an extended ACL.
type aclInfo struct {
	entries int
	allow   int
}
