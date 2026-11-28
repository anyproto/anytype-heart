package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// newFile returns the endpoint file under a scratch home, never the
// developer's, with its folders made as the desktop app makes them and no
// file yet.
func newFile(t *testing.T) *File {
	home := t.TempDir()
	privateHome(t, home)
	f := OpenAt(home, NamespaceDev)
	require.NoError(t, os.MkdirAll(filepath.Dir(f.Path()), 0o700))
	return f
}

func write(t *testing.T, f *File, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(f.Path(), []byte(content), 0o600))
}

func requireReason(t *testing.T, err error, reason, msg string) *Error {
	t.Helper()
	var e *Error
	require.ErrorAs(t, err, &e, msg)
	require.Equal(t, reason, e.Reason, "%s: %v", msg, err)
	return e
}

func TestNamespaces(t *testing.T) {
	assert.Equal(t, "endpoint.json", FileName(NamespaceRelease))
	assert.Equal(t, "endpoint-dev.json", FileName(NamespaceDev))
	assert.Equal(t, "endpoint-dev.json", FileName("anything else"))
	assert.Equal(t, filepath.Join("h", ".anytype", "ai-connect", "endpoint.json"), OpenAt("h", NamespaceRelease).Path())
}

func TestReadFormat(t *testing.T) {
	for content, port := range map[string]int{
		`{"v":1,"port":31009}`:             31009,
		"{\"port\": 1, \"v\": 1}\n":        1,
		`{"v":1,"port":65535,"extra":"x"}`: 65535,
	} {
		f := newFile(t)
		write(t, f, content)
		got, err := f.Read(ctx)
		require.NoError(t, err, content)
		assert.Equal(t, port, got, content)
	}
	for content, rule := range map[string]string{
		``:                       "not one JSON object",
		`garbage`:                "not one JSON object",
		`[1]`:                    "not one JSON object",
		`{"v":1,"port":1}{}`:     "not one JSON object",
		`{"v":1,"port":"31009"}`: "not one JSON object",
		`{"v":1,"port":1.5}`:     "not one JSON object",
		`{"port":1}`:             "version is not 1",
		`{"v":2,"port":1}`:       "version is not 1",
		`{"v":1}`:                "port is not between 1 and 65535",
		`{"v":1,"port":0}`:       "port is not between 1 and 65535",
		`{"v":1,"port":65536}`:   "port is not between 1 and 65535",
		`{"v":1,"port":1}` + strings.Repeat(" ", 241): "larger than 256 bytes",
	} {
		f := newFile(t)
		write(t, f, content)
		_, err := f.Read(ctx)
		e := requireReason(t, err, ReasonInvalid, content)
		assert.Contains(t, e.Rule, rule, content)
		assert.Equal(t, f.Path(), e.Path)
	}
}

func TestReadMissing(t *testing.T) {
	home := t.TempDir()
	privateHome(t, home)
	f := OpenAt(home, NamespaceDev)
	for _, missing := range []string{filepath.Join(home, ".anytype"), filepath.Dir(f.Path()), f.Path()} {
		_, err := f.Read(ctx)
		e := requireReason(t, err, ReasonMissing, missing)
		assert.Equal(t, missing, e.Path)
		require.NoError(t, os.MkdirAll(missing, 0o700))
		if missing == f.Path() {
			require.NoError(t, os.Remove(missing))
		}
	}
	write(t, f, `{"v":1,"port":7}`)
	port, err := f.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, 7, port)

	_, err = OpenAt(filepath.Join(home, "nobody"), NamespaceDev).Read(ctx)
	requireReason(t, err, ReasonMissing, "a home that does not exist")
}

func TestCheckExecutablePlaces(t *testing.T) {
	f := newFile(t)
	exe := func(path string) string {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0o700))
		return path
	}
	self := func(path string) error { return f.CheckExecutable(path, path) }
	// outside .anytype the executable is not judged
	assert.NoError(t, self(exe(filepath.Join(t.TempDir(), "anytype-mcp"))))
	assert.NoError(t, self(exe(filepath.Join(f.home, ".anytype-other", "anytype-mcp"))))
	assert.NoError(t, self(exe(filepath.Join(f.home, "anytype-mcp"))))
	// under it, a sound copy passes, at any depth
	assert.NoError(t, self(exe(filepath.Join(f.home, ".anytype", "bin", "anytype-mcp"))))
	assert.NoError(t, self(exe(filepath.Join(f.home, ".anytype", "anytype-mcp"))))
	assert.NoError(t, self(exe(filepath.Join(f.home, ".anytype", "a", "b", "anytype-mcp"))))
}

// COR-09: whatever fails in the executable's check is denied, with its
// path, never "missing" as if the endpoint were.
func TestH1R1_COR09_ExecutableFailuresAreDenied(t *testing.T) {
	f := newFile(t)
	gone := filepath.Join(f.home, ".anytype", "bin", "gone")
	require.NoError(t, os.Mkdir(filepath.Dir(gone), 0o700))
	e := requireReason(t, f.CheckExecutable(gone, gone), ReasonDenied, "a missing copy")
	assert.Equal(t, gone, e.Path)
	assert.NotEmpty(t, e.Rule)
	outside := filepath.Join(t.TempDir(), "gone")
	requireReason(t, f.CheckExecutable(outside, outside), ReasonDenied, "a path whose place can't be established")
}

// Theme C: the path the caller started the forwarder by is judged as it
// is spelled, and must be the running file.
func TestH1R1_C_LaunchPathIsTheRunningFile(t *testing.T) {
	f := newFile(t)
	put := func(path string) string {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("binary"), 0o700))
		return path
	}
	launch := put(filepath.Join(f.home, ".anytype", "bin", "anytype-mcp"))
	other := put(filepath.Join(f.home, ".anytype", "bin2", "anytype-mcp"))
	outside := put(filepath.Join(t.TempDir(), "anytype-mcp"))
	require.NoError(t, f.CheckExecutable(launch, launch))
	for _, running := range []string{other, outside} {
		e := requireReason(t, f.CheckExecutable(launch, running), ReasonDenied, running)
		assert.Equal(t, launch, e.Path)
		assert.Contains(t, e.Rule, "not the file this process runs")
	}
	// a running file in .anytype that the launch path does not show
	e := requireReason(t, f.CheckExecutable(outside, launch), ReasonDenied, "unseen")
	assert.Equal(t, launch, e.Path)
	assert.Contains(t, e.Rule, "another spelling or through a link")
}

// COR-08: v and port are matched by their exact names, once each.
func TestH1R1_COR08_ExactMembers(t *testing.T) {
	for content, rule := range map[string]string{
		`{"v":2,"V":1,"port":31009}`:      "version is not 1",
		`{"V":1,"PORT":31009}`:            "version is not 1",
		`{"v":1,"Port":31009}`:            "port is not between",
		`{"v":1,"port":1,"port":2}`:       "names port twice",
		`{"v":1,"v":1,"port":1}`:          "names v twice",
		`{"v":1,"port":1,"x":{"port":2}}`: "",
	} {
		f := newFile(t)
		write(t, f, content)
		port, err := f.Read(ctx)
		if rule == "" {
			require.NoError(t, err, content)
			assert.Equal(t, 1, port)
			continue
		}
		e := requireReason(t, err, ReasonInvalid, content)
		assert.Contains(t, e.Rule, rule, content)
	}
}

// Theme G: a launch path that is not clean and absolute is refused before
// anything is cleaned away from it.
func TestH1R2_G_LaunchPathMustBeClean(t *testing.T) {
	f := newFile(t)
	bin := filepath.Join(f.home, ".anytype", "bin")
	require.NoError(t, os.MkdirAll(filepath.Join(f.home, ".anytype", "shared", "child"), 0o700))
	require.NoError(t, os.MkdirAll(bin, 0o700))
	exe := filepath.Join(bin, "anytype-mcp")
	require.NoError(t, os.WriteFile(exe, []byte("binary"), 0o700))
	require.NoError(t, f.CheckExecutable(exe, exe))
	sep := string(filepath.Separator)
	for _, launch := range []string{
		filepath.Join(f.home, ".anytype", "shared", "child") + sep + ".." + sep + ".." + sep + "bin" + sep + "anytype-mcp",
		filepath.Join(f.home, ".anytype") + sep + "." + sep + "bin" + sep + "anytype-mcp",
		bin + sep + sep + "anytype-mcp",
		filepath.Join(".anytype", "bin", "anytype-mcp"),
	} {
		e := requireReason(t, f.CheckExecutable(launch, exe), ReasonDenied, launch)
		assert.Contains(t, e.Rule, "not clean and absolute", launch)
	}
}
