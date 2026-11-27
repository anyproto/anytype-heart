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
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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
	info os.FileInfo // the open folder itself

	// Test hooks: link replaces the hard link of publish; afterLstat runs
	// between the look at an upload's name and its open; beforePublish
	// once a download is written whole.
	link          func(oldname, newname string) error
	afterLstat    func()
	beforePublish func()
}

// Open opens the folder the user named: any directory of their choice.
// It must be an absolute path with no ".." in it; it is created private
// when missing; an existing one is taken as it is, whoever owns it and
// whatever its permissions, and is never changed. The path may be spelled
// through links (macOS /tmp): it is resolved once, here, and the resolved
// directory is the one pinned for the life of the process. It must not
// overlap any of the avoid folders (Anytype's own, where the connection
// keys live): not one of them, not inside one, not around one.
func Open(path string, avoid ...string) (*Dir, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("the files folder must be an absolute path: %s", path)
	}
	// behind a linked ancestor ".." names another place than its text says
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == filepath.Separator }) {
		if part == ".." {
			return nil, fmt.Errorf("the files folder must be an absolute path without \"..\": %s", path)
		}
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		// parents as the system makes them; only the folder itself is
		// ours to make private, and nothing that exists is changed
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("the files folder can't be created: %w", err)
		}
		if err := mkdirPrivate(path); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("the files folder can't be created: %w", err)
		}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("the files folder can't be opened: %w", err)
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, fmt.Errorf("the files folder can't be opened (it must be a directory): %w", err)
	}
	info, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("the files folder can't be read: %w", err)
	}
	d := &Dir{path: resolved, root: root, info: info, link: root.Link}
	for _, other := range avoid {
		if d.overlaps(other) {
			_ = d.Close()
			return nil, fmt.Errorf("the files folder must be apart from Anytype's own folder %s: %s", other, path)
		}
	}
	return d, nil
}

// Temp makes a private folder for this process under the system's
// temporary folder. Nothing removes it: what was downloaded stays until
// the system cleans up.
func Temp() (*Dir, error) {
	for range 10 {
		id, err := randomHex(8)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(os.TempDir(), "anytype-mcp-files-"+id)
		err = mkdirPrivate(path)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("no temporary files folder: %w", err)
		}
		// the system's temporary folder may be spelled through a link
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		return open(path, true)
	}
	return nil, errors.New("no temporary files folder: no free name")
}

// open pins the temporary folder and judges what it pinned: it must be
// the private folder just made, not something that took its name. The
// checks read the open folder, and the root every later access goes
// through must be that same folder.
func open(path string, temp bool) (*Dir, error) {
	h, err := openFolder(path)
	if err != nil {
		if fi, lerr := os.Lstat(path); lerr == nil && (fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !fi.IsDir()) {
			return nil, fmt.Errorf("the files folder is not a real directory (a link is not accepted): %s", path)
		}
		return nil, fmt.Errorf("the files folder can't be opened: %w", err)
	}
	defer h.Close()
	fi, err := h.Stat()
	if err != nil {
		return nil, fmt.Errorf("the files folder can't be read: %w", err)
	}
	// ModeIrregular is how a Windows junction shows
	if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !fi.IsDir() {
		return nil, fmt.Errorf("the files folder is not a real directory (a link is not accepted): %s", path)
	}
	if err := checkFolder(h, fi, path); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("the files folder can't be opened: %w", err)
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(fi, opened) {
		_ = root.Close()
		return nil, fmt.Errorf("the files folder changed while it was being opened: %s", path)
	}
	return &Dir{path: path, temp: temp, root: root, info: opened, link: root.Link}, nil
}

// overlaps reports whether other is the folder, lies inside it, or holds
// it. Folders are compared by identity on the way up from each, so another
// spelling, a link or a letter's case hides nothing.
func (d *Dir) overlaps(other string) bool {
	climb := func(from string, target os.FileInfo) bool {
		for p := filepath.Clean(from); ; p = filepath.Dir(p) {
			if fi, err := os.Stat(p); err == nil && os.SameFile(fi, target) {
				return true
			}
			if filepath.Dir(p) == p {
				return false
			}
		}
	}
	// the folder is other or holds it (other need not exist yet)
	if climb(other, d.info) {
		return true
	}
	// the folder is inside other
	if fi, err := os.Stat(other); err == nil && climb(d.path, fi) {
		return true
	}
	return false
}

// Located reports whether the folder's path still names the folder: it
// may have been moved, and something else put at its path.
func (d *Dir) Located() bool {
	fi, err := os.Lstat(d.path)
	return err == nil && fi.IsDir() && os.SameFile(fi, d.info)
}

// Path is the folder's absolute path with every link in it resolved: the
// directory that is pinned.
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
	climbs := false
	for _, part := range strings.Split(local, string(filepath.Separator)) {
		climbs = climbs || part == ".."
	}
	// what the texts quote of it: one line, bounded
	path = quoted(path)
	if climbs || !filepath.IsLocal(local) {
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
	// A hard link and a file system mounted inside the folder are taken
	// as what they are: something put into the folder by whoever could
	// write there, like a copy.
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
// failure removes the part file and nothing else. A transfer whose ctx
// ended before publication publishes nothing.
func (d *Dir) Save(ctx context.Context, name string, body io.Reader) (saved string, size int64, err error) {
	id, err := randomHex(16)
	if err != nil {
		return "", 0, err
	}
	part := partPrefix + id + partSuffix
	f, err := d.root.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, err
	}
	defer d.root.Remove(part) //nolint:errcheck // ours by its random name; a published file keeps its own link
	size, err = io.Copy(f, body)
	written, serr := f.Stat()
	if err == nil {
		err = serr
	}
	// Unix tells files apart by a number that is free again once the file
	// is gone: the part file stays open, and so stays itself, until what
	// was published has been compared with it. Windows identifies a file
	// for good, and links a closed file more willingly.
	if runtime.GOOS == "windows" {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	} else {
		if cerr := f.Sync(); err == nil {
			err = cerr
		}
		defer f.Close()
	}
	if d.beforePublish != nil {
		d.beforePublish()
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return "", 0, err
	}
	saved, err = d.publish(part, SafeName(name), written)
	if err != nil {
		return "", 0, err
	}
	return saved, size, nil
}

// publish gives the finished part file its name with a hard link: one
// step that fails rather than replace an existing entry, so a file appears
// under its name only complete and never over another. Numbered on a
// collision. Where the file system has no hard links (FAT, exFAT) there is
// no such step, and the download fails rather than risk another file. What
// was published must be the file that was written.
func (d *Dir) publish(part, name string, written os.FileInfo) (string, error) {
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
	for _, candidate := range candidates {
		err := d.link(part, candidate)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", refuse("the download could not be saved in %s: giving it its name needs a hard link, which failed there. "+
				"Ask the user to choose a folder on a file system with hard links (not FAT or exFAT); nothing was saved.", d.path)
		}
		if fi, err := d.root.Lstat(candidate); err != nil || !os.SameFile(fi, written) {
			return "", fmt.Errorf("the files folder changed while %s was being published", candidate)
		}
		return candidate, nil
	}
	return "", refuse("no free file name for the download in %s", d.path)
}
