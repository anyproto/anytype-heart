package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxFileBytes bounds the endpoint file: a longer file is not an endpoint.
const MaxFileBytes = 256

// File is the endpoint file: <home>/.anytype/ai-connect/<FileName>. The
// folders on its path are checked before every read (the folder rule) and
// the file again through its open handle (the file rule). A check that
// fails reads nothing.
type File struct {
	home    string
	homeErr error
	name    string

	// uid is the user each path must belong to; tests replace it because
	// a non-root process can't chown.
	uid func(path string) int //nolint:unused // read by the Unix rules only
}

// Open returns the endpoint file of a namespace under the user's home
// directory, which comes from os.UserHomeDir and nothing else.
func Open(namespace string) *File {
	home, err := os.UserHomeDir()
	f := OpenAt(home, namespace)
	if err != nil || home == "" {
		f.homeErr = errors.New("the home directory is unknown")
	}
	return f
}

// OpenAt returns the endpoint file of a namespace under an explicit home.
func OpenAt(home, namespace string) *File {
	return &File{home: home, name: FileName(namespace), uid: func(string) int { return os.Geteuid() }}
}

// OwnerRule replaces the uid each path must belong to. It exists for tests
// of the owner rule; no flag, file or environment variable reaches it.
func (f *File) OwnerRule(uid func(path string) int) { f.uid = uid }

// Path is the endpoint file's path.
func (f *File) Path() string { return filepath.Join(f.home, ".anytype", "ai-connect", f.name) }

// anytype is <home>/.anytype.
func (f *File) anytype() string { return filepath.Join(f.home, ".anytype") }

// folder kinds: the folder rule differs for each.
const (
	kindHome   = iota // the user's home: its mode is not judged
	kindStrict        // .anytype, ai-connect and the folders below .anytype
)

// checkChain applies the folder rule to each folder, in order: home's
// own rule to home, the strict one to every other. A missing folder is
// ReasonMissing.
func (f *File) checkChain(dirs ...string) error {
	for _, dir := range dirs {
		fi, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			return missing(dir)
		}
		if err != nil {
			return unreachable(dir, err)
		}
		kind := kindStrict
		if dir == f.home {
			kind = kindHome
		}
		if err := f.checkDir(dir, fi, kind); err != nil {
			return err
		}
	}
	return nil
}

// Read reads the port: home, .anytype and ai-connect pass the folder rule,
// the file passes the file rule, and it holds exactly {"v":1,"port":N}.
func (f *File) Read(context.Context) (int, error) {
	if f.homeErr != nil {
		return 0, unreachable("", f.homeErr)
	}
	dir := filepath.Dir(f.Path())
	if err := f.checkChain(f.home, f.anytype(), dir); err != nil {
		return 0, err
	}
	fh, err := f.openChecked(dir, f.name, "the endpoint file")
	if err != nil {
		return 0, err
	}
	defer fh.Close()
	data, err := io.ReadAll(io.LimitReader(fh, MaxFileBytes+1))
	if err != nil {
		return 0, unreachable(f.Path(), err)
	}
	port, rule := parse(data)
	if rule != "" {
		return 0, invalid(f.Path(), rule)
	}
	return port, nil
}

// parse reads {"v":1,"port":N}: one object and nothing after it, v exactly
// 1, port 1–65535, each named exactly and once; other members are
// ignored. It returns the rule that failed, never the content.
func parse(data []byte) (int, string) {
	const notObject = "the endpoint file is not one JSON object"
	if len(data) > MaxFileBytes {
		return 0, "the endpoint file is larger than 256 bytes"
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return 0, notObject
	}
	var v, port *int
	for dec.More() {
		tok, err := dec.Token()
		name, ok := tok.(string)
		if err != nil || !ok {
			return 0, notObject
		}
		var into **int
		switch name {
		case "v":
			into = &v
		case "port":
			into = &port
		default:
			var skip json.RawMessage
			if dec.Decode(&skip) != nil {
				return 0, notObject
			}
			continue
		}
		if *into != nil {
			return 0, "the endpoint file names " + name + " twice"
		}
		var n int
		if dec.Decode(&n) != nil {
			return 0, notObject
		}
		*into = &n
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return 0, notObject
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return 0, notObject
	}
	switch {
	case v == nil || *v != 1:
		return 0, "the endpoint file's version is not 1"
	case port == nil || *port < 1 || *port > 65535:
		return 0, "the endpoint file's port is not between 1 and 65535"
	}
	return *port, ""
}

// CheckExecutable is the endpoint verb's rule for the program an AI app
// would give its key to. launch is the path this process was started by,
// as the caller spelled it; running is the file the system says it runs.
// launch must be clean and absolute: a ".." or "." would hide the folders
// it really passes. When it is spelled under <home>/.anytype, exactly as
// the home is spelled, every folder from .anytype down to it passes the
// folder rule and the file itself the file rule, none of them followed,
// and it must be the running file. A launch path that passes .anytype
// under another spelling (a link, another case, an 8.3 name) is refused,
// and so is a running file that really lies in it. Elsewhere nothing is
// judged; what lies above .anytype falls under the home rule. Every
// failure is ReasonDenied.
func (f *File) CheckExecutable(launch, running string) error {
	err := f.checkExecutable(launch, running)
	if err == nil {
		return nil
	}
	e := AsError(err)
	rule := e.Rule
	if rule == "" {
		rule = "the forwarder's executable can't be checked (" + e.Reason + ")"
		if e.Err != nil {
			rule += ": " + e.Err.Error()
		}
	}
	return &Error{Reason: ReasonDenied, Path: e.Path, Rule: rule}
}

func (f *File) checkExecutable(launch, running string) error {
	if f.homeErr != nil {
		return unreachable("", f.homeErr)
	}
	if !filepath.IsAbs(launch) || filepath.Clean(launch) != launch {
		return deniedAt(launch, "the path that started the forwarder is not clean and absolute")
	}
	top := f.anytype()
	if strings.HasPrefix(launch, top+string(filepath.Separator)) {
		var dirs []string
		for dir := filepath.Dir(launch); len(dir) >= len(top); dir = filepath.Dir(dir) {
			dirs = append([]string{dir}, dirs...)
		}
		if err := f.checkChain(dirs...); err != nil {
			return err
		}
		fh, err := f.openChecked(filepath.Dir(launch), filepath.Base(launch), "the forwarder's executable")
		if err != nil {
			return err
		}
		defer fh.Close()
		checked, err := fh.Stat()
		if err != nil {
			return unreachable(launch, err)
		}
		if fi, err := os.Stat(running); err != nil || !os.SameFile(checked, fi) {
			return deniedAt(launch, "the forwarder's executable is not the file this process runs")
		}
		return nil
	}
	anytype, err := os.Stat(top)
	if err != nil {
		return unreachable(top, err)
	}
	const alias = "the forwarder's executable lies in .anytype under another spelling or through a link"
	// spelled elsewhere: no folder the path passes may be .anytype, checked
	// for each folder as it is spelled, before a later link can lead away
	if passes(filepath.Dir(launch), anytype) {
		return deniedAt(launch, alias)
	}
	// and the running file must not really lie in it
	fh, err := os.Open(running)
	if err != nil {
		return unreachable(running, err)
	}
	defer fh.Close()
	real, err := FinalPath(fh)
	if err != nil {
		return unreachable(running, err)
	}
	if passes(filepath.Dir(real), anytype) {
		return deniedAt(running, alias)
	}
	return nil
}

// passes reports whether dir or a folder above it is the folder target,
// by identity.
func passes(dir string, target os.FileInfo) bool {
	for ; ; dir = filepath.Dir(dir) {
		if fi, err := os.Stat(dir); err == nil && os.SameFile(fi, target) {
			return true
		}
		if filepath.Dir(dir) == dir {
			return false
		}
	}
}
