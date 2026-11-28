//go:build !windows

package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// checkDir is the folder rule for one path component.
//
// Home must be a real directory owned by this user. Its mode is not
// judged: a home that its group can write (a user-private group) is a
// normal setup, and whoever can write a home can already replace the
// user's shell files. On macOS home may carry deny ACL entries only (every
// Mac home has "everyone deny delete").
//
// Every other folder (.anytype, ai-connect, and for the endpoint verb the
// folders down to the executable) must be a real directory owned by this
// user that neither group nor others can write, with no ACL at all (macOS
// evaluates ACL entries before the mode bits).
func (f *File) checkDir(dir string, fi os.FileInfo, kind int) error {
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return deniedAt(dir, "a folder on the endpoint path is not a real directory")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return deniedAt(dir, "a folder on the endpoint path has no owner")
	}
	if int(st.Uid) != f.uid(dir) {
		return deniedAt(dir, "a folder on the endpoint path belongs to another user")
	}
	acl, err := pathACL(dir)
	if err != nil {
		return unreachable(dir, err)
	}
	if kind == kindHome {
		if acl.allow > 0 {
			return deniedAt(dir, "the home folder has an ACL that grants access")
		}
		return nil
	}
	if acl.entries > 0 {
		return deniedAt(dir, "a folder on the endpoint path has an ACL")
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return deniedAt(dir, "a folder on the endpoint path is writable by others")
	}
	return nil
}

// openChecked is the file rule: the name must be a regular file, and the
// file actually opened must be that same file, owned by this user, with
// one link, writable by nobody else and with no ACL. Access goes through
// an os.Root on the folder, so nothing leaves it. what names the file in
// the rule's text.
func (f *File) openChecked(dir, name, what string) (*os.File, error) {
	path := filepath.Join(dir, name)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, unreachable(dir, err)
	}
	defer root.Close()
	li, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, missing(path)
	}
	if err != nil {
		return nil, unreachable(path, err)
	}
	if !li.Mode().IsRegular() {
		return nil, deniedAt(path, what+" is not a regular file")
	}
	// O_NONBLOCK: a FIFO swapped in after the check must not hang the open
	fh, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, unreachable(path, err)
	}
	refuse := func(rule string) (*os.File, error) {
		_ = fh.Close()
		return nil, deniedAt(path, what+rule)
	}
	fi, err := fh.Stat()
	if err != nil {
		_ = fh.Close()
		return nil, unreachable(path, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case !ok || !fi.Mode().IsRegular() || !os.SameFile(li, fi):
		return refuse(" is not a regular file")
	case int(st.Uid) != f.uid(path):
		return refuse(" belongs to another user")
	case fi.Mode().Perm()&0o022 != 0:
		return refuse(" is writable by others")
	case st.Nlink != 1:
		return refuse(" has more than one link")
	}
	acl, err := fileACL(fh)
	if err != nil {
		_ = fh.Close()
		return nil, unreachable(path, err)
	}
	if acl.entries > 0 {
		return refuse(" has an ACL")
	}
	return fh, nil
}

// aclInfo counts the entries of an extended ACL.
type aclInfo struct {
	entries int
	allow   int
}
