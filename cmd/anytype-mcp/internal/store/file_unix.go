//go:build !windows

package store

import (
	"errors"
	"os"
	"syscall"
)

// mkdir creates one missing folder private to the user. The mode is set
// again after the fact because of the umask.
func (f *File) mkdir(dir string, _ bool) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// checkDir is the folder rule for one path component: a real directory,
// owned by this user, that neither group nor others can write. The items
// folder itself must be exactly 0700.
func (f *File) checkDir(_ string, fi os.FileInfo, items bool) error {
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return denied("a folder on the item path is not a real directory")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return denied("a folder on the item path has no owner")
	}
	if int(st.Uid) != f.uid() {
		return denied("a folder on the item path belongs to another user")
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return denied("a folder on the item path is writable by others")
	}
	if items && fi.Mode().Perm() != 0o700 {
		return denied("the items folder is not private (0700)")
	}
	return nil
}

// readItem is the item rule: the name must be a regular file, and the
// file actually opened must be that same file, owned by this user, mode
// exactly 0600, with one link. Access goes through an os.Root on the items
// folder, so nothing leaves it.
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
		return nil, denied("the item is not a regular file")
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
		return nil, denied("the item is not a regular file")
	case int(st.Uid) != f.uid():
		return nil, denied("the item belongs to another user")
	case fi.Mode().Perm() != 0o600:
		return nil, denied("the item is not private (0600)")
	case st.Nlink != 1:
		return nil, denied("the item has more than one link")
	}
	return readChecked(fh)
}

// createPrivate makes a new 0600 file that did not exist.
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
	if err := fh.Chmod(0o600); err != nil {
		ignore(fh.Close())
		ignore(root.Remove(name))
		return nil, unreachable(err)
	}
	return fh, nil
}

func renameIn(dir, from, to string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Rename(from, to)
}

func removeIn(dir, name string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(name)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
