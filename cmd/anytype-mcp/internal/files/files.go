// Package files is the one directory the forwarder's local file tools may
// touch. The directory is fixed when serve starts: the folder the user
// named with --files-dir, or a private temporary folder made for this
// process. Every access goes through an os.Root on it, so a path with
// "..", an absolute path, or a link or junction that leads out is refused
// by the system call itself, whatever happens to the folder meanwhile.
//
// The model chooses the paths, and a prompt can steer the model: it must
// not be able to send an arbitrary file of the user's to a space, only a
// file somebody put into this folder on purpose.
package files

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Refusal is an error whose text the model reads: it names the folder, so
// the model can tell the user where files go.
type Refusal struct{ Text string }

func (r *Refusal) Error() string { return r.Text }

func refuse(format string, args ...any) error {
	return &Refusal{Text: fmt.Sprintf(format, args...)}
}

// Dir is the open folder.
type Dir struct {
	path string
	temp bool
	root *os.Root

	// Test hooks: link replaces the hard link of publish; afterLstat runs
	// between the look at an upload's name and its open.
	link       func(oldname, newname string) error
	afterLstat func()
}

// Open opens the folder the user named. It must be an absolute path; it is
// created private (0700) when missing; it must be a real directory (not a
// link or a junction) that belongs to this user and that no other user can
// write.
func Open(path string) (*Dir, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("the files folder must be an absolute path: %s", path)
	}
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return nil, fmt.Errorf("the files folder can't be created: %w", err)
		}
		// the mode again, because of the umask
		if err := os.Chmod(path, 0o700); err != nil {
			return nil, fmt.Errorf("the files folder can't be made private: %w", err)
		}
	}
	return open(path, false)
}

// Temp makes a private folder for this process under the system's
// temporary folder. Nothing removes it: what was downloaded stays until
// the system cleans up.
func Temp() (*Dir, error) {
	path, err := os.MkdirTemp("", "anytype-mcp-files-")
	if err != nil {
		return nil, fmt.Errorf("no temporary files folder: %w", err)
	}
	return open(path, true)
}

func open(path string, temp bool) (*Dir, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("the files folder can't be read: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return nil, fmt.Errorf("the files folder is not a real directory (a link is not accepted): %s", path)
	}
	if err := checkOwner(path, fi); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("the files folder can't be opened: %w", err)
	}
	// the folder opened is the folder judged
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(fi, opened) {
		_ = root.Close()
		return nil, fmt.Errorf("the files folder changed while it was being opened: %s", path)
	}
	return &Dir{path: path, temp: temp, root: root, link: root.Link}, nil
}

// Path is the folder's absolute path, as the user and the model know it.
func (d *Dir) Path() string { return d.path }

// Temporary reports whether the folder was made for this process.
func (d *Dir) Temporary() bool { return d.temp }

// Close releases the folder. Transfers still running fail.
func (d *Dir) Close() error { return d.root.Close() }

var driveRe = regexp.MustCompile(`^[a-zA-Z]:`)

// absolute reports a path that names a place by itself, on any system: it
// is decided on the text alone, so nothing is ever looked up for it.
func absolute(path string) bool {
	return strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) || strings.HasPrefix(path, "~") ||
		driveRe.MatchString(path) || filepath.IsAbs(path)
}

// quoted is a path as a refusal shows it.
func quoted(path string) string {
	path = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(path, "?"))
	const most = 120
	if r := []rune(path); len(r) > most {
		path = string(r[:most]) + "…"
	}
	return path
}

// OpenUpload opens a regular file in the folder for reading. path is
// relative to the folder. The file's own name is never followed as a
// link, and the file opened is the file that was looked at: a name
// swapped for a link in between is refused.
func (d *Dir) OpenUpload(path string) (f *os.File, name string, err error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", refuse("path must name a file in %s, relative to it", d.path)
	}
	if strings.ContainsRune(path, 0) || absolute(path) {
		return nil, "", refuse("path must be relative to %s; got an absolute path. Ask the user to copy the file into %s.", d.path, d.path)
	}
	local := filepath.FromSlash(path)
	// what the texts quote of it: one line, bounded
	path = quoted(path)
	if !filepath.IsLocal(local) {
		return nil, "", refuse("%s is outside %s. Ask the user to copy the file into %s.", path, d.path, d.path)
	}
	local = filepath.Clean(local)
	li, err := d.root.Lstat(local)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, "", refuse("there is no file %s in %s", path, d.path)
	case err != nil:
		// a folder on the way is a link that leads out, or can't be read
		return nil, "", refuse("%s can't be reached inside %s: links that lead out of it are not followed. Ask the user to copy the file itself into %s.", path, d.path, d.path)
	case li.Mode()&os.ModeSymlink != 0:
		return nil, "", refuse("%s in %s is a link, which is not followed. Ask the user to copy the file itself into %s.", path, d.path, d.path)
	case !li.Mode().IsRegular():
		return nil, "", refuse("%s in %s is not a regular file", path, d.path)
	}
	if d.afterLstat != nil {
		d.afterLstat()
	}
	// non-blocking: should the name have become a FIFO, the open must not
	// wait for a writer
	f, err = d.root.OpenFile(local, os.O_RDONLY|openNonblock, 0)
	if err != nil {
		return nil, "", refuse("%s in %s can't be opened", path, d.path)
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || !os.SameFile(li, fi) {
		_ = f.Close()
		return nil, "", refuse("%s in %s changed while it was being opened", path, d.path)
	}
	return f, filepath.Base(local), nil
}

const partPrefix, partSuffix = ".anytype-mcp-", ".part"

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Save stores body in the folder as a new file named after name and
// returns the name it got. The bytes go to a private part file first; only
// a complete file is published, under a name nothing else holds, and a
// failure removes the part file and nothing else.
func (d *Dir) Save(name string, body io.Reader) (saved string, size int64, err error) {
	id, err := randomHex(16)
	if err != nil {
		return "", 0, err
	}
	part := partPrefix + id + partSuffix
	f, err := d.root.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, err
	}
	// ours by its random name; a published file keeps its own link
	defer d.root.Remove(part) //nolint:errcheck // gone already when a rename published it
	size, err = io.Copy(f, body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", 0, err
	}
	saved, err = d.publish(part, SafeName(name))
	if err != nil {
		return "", 0, err
	}
	return saved, size, nil
}

// publish gives the finished part file its name: numbered on a collision,
// never over an existing entry. A hard link does it in one step. Where the
// file system has none (FAT, exFAT), the name is first taken with an empty
// file of our own, which the part file then replaces.
func (d *Dir) publish(part, name string) (string, error) {
	ext := extensionOf(name)
	stem := strings.TrimSuffix(name, ext)
	candidates := make([]string, 0, 101)
	candidates = append(candidates, stem+ext)
	for n := 2; n <= 100; n++ {
		candidates = append(candidates, fmt.Sprintf("%s (%d)%s", stem, n, ext))
	}
	if tail, err := randomHex(4); err == nil {
		candidates = append(candidates, stem+"-"+tail+ext)
	}
	linked := true
	for _, candidate := range candidates {
		if linked {
			err := d.link(part, candidate)
			if err == nil {
				return candidate, nil
			}
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			linked = false
		}
		held, err := d.root.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_ = held.Close()
		if err := d.root.Rename(part, candidate); err != nil {
			d.root.Remove(candidate) //nolint:errcheck // the empty file is ours; the rename's error is the one to report
			return "", err
		}
		return candidate, nil
	}
	return "", refuse("no free file name for the download in %s", d.path)
}
