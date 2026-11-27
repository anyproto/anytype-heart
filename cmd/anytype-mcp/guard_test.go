package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ownPackage  = "github.com/anyproto/anytype-heart/cmd/anytype-mcp"
	heartPrefix = "github.com/anyproto/anytype-heart/"

	// maxDeps bounds everything the forwarder links, the standard library
	// included. core/api/wrapper alone would bring 447 packages.
	maxDeps = 260
	// maxNonStdDeps bounds what is not the standard library: this
	// command's five packages, the lock library and x/sys.
	maxNonStdDeps = 8
)

// shipped are the five targets the forwarder is built for, all without cgo.
var shipped = [][2]string{{"darwin", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}, {"linux", "amd64"}, {"linux", "arm64"}}

func goList(t *testing.T, target [2]string, format string) []string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH")
	}
	cmd := exec.Command(goBin, "list", "-deps", "-f", format, ".")
	cmd.Env = append(os.Environ(), "GOOS="+target[0], "GOARCH="+target[1], "CGO_ENABLED=0")
	out, err := cmd.Output()
	require.NoError(t, err, "go list -deps for %v", target)
	var lines []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// own reports whether pkg is this command or one of its packages; a
// sibling such as …/anytype-mcpx is not.
func own(pkg string) bool { return pkg == ownPackage || strings.HasPrefix(pkg, ownPackage+"/") }

// TestDependencyBudget fails when the forwarder starts importing heart or
// anything heavy, on any target it ships for: it must stay a small static
// binary that starts in milliseconds.
func TestDependencyBudget(t *testing.T) {
	for _, target := range shipped {
		t.Run(target[0]+"-"+target[1], func(t *testing.T) {
			total, nonStd := 0, []string{}
			for _, line := range goList(t, target, "{{.ImportPath}} {{.Standard}}") {
				pkg, std, _ := strings.Cut(line, " ")
				total++
				if std == "true" {
					continue
				}
				nonStd = append(nonStd, pkg)
				if strings.HasPrefix(pkg, heartPrefix) && !own(pkg) {
					t.Errorf("the forwarder must not import heart: %s", pkg)
				}
			}
			assert.LessOrEqual(t, total, maxDeps, "go list -deps ./cmd/anytype-mcp grew")
			assert.LessOrEqual(t, len(nonStd), maxNonStdDeps, "non-standard packages: %v", nonStd)
		})
	}
	assert.True(t, own(ownPackage) && own(ownPackage+"/internal/store"))
	assert.False(t, own(ownPackage+"x/internal"), "a sibling package is not this command")
}

// moduleLine names what a module contributes to the binary: its path and
// version, and where a go.mod replace swaps it, what it is replaced with.
// A replacement without a version (a local directory) can't be pinned and
// is an error.
func moduleLine(path, version, replacePath, replaceVersion string) (string, error) {
	line := path + "@" + version
	if replacePath == "" {
		return line, nil
	}
	if replaceVersion == "" {
		return "", fmt.Errorf("%s is replaced by %s, which has no version: Rev can't cover it", path, replacePath)
	}
	return line + " => " + replacePath + "@" + replaceVersion, nil
}

// modules lists every module version linked into any shipped target.
func modules(t *testing.T) []string {
	const format = "{{with .Module}}{{.Path}}|{{.Version}}|{{with .Replace}}{{.Path}}|{{.Version}}{{end}}{{end}}"
	seen := map[string]bool{}
	for _, target := range shipped {
		for _, m := range goList(t, target, format) {
			f := append(strings.Split(m, "|"), "", "", "")
			line, err := moduleLine(f[0], f[1], f[2], f[3])
			require.NoError(t, err)
			seen[line] = true
		}
	}
	var out []string
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// sources returns the forwarder's non-test Go files, name then content,
// with line endings normalized.
func sources(t *testing.T) []string {
	var files []string
	require.NoError(t, filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "integration" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	}))
	sort.Strings(files)
	var out []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		out = append(out, f+"\n"+strings.ReplaceAll(string(raw), "\r\n", "\n"))
	}
	return out
}

// revHash is what Rev stands for: the sources and every dependency
// version that ends up in a shipped binary.
func revHash(sources, modules []string) string {
	h := sha256.New()
	for _, s := range sources {
		fmt.Fprintf(h, "source %d\n%s\n", len(s), s)
	}
	for _, m := range modules {
		fmt.Fprintf(h, "module %s\n", m)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// updateLock records a new pair. It refuses unless rev is greater than
// the one on record: regenerating the lock is not a way around the bump.
func updateLock(path string, rev int, hash string) error {
	if raw, err := os.ReadFile(path); err == nil {
		field, _, _ := strings.Cut(strings.TrimSpace(string(raw)), " ")
		if old, err := strconv.Atoi(field); err == nil && rev <= old {
			return fmt.Errorf("Rev is %d and the lock already records %d: raise Rev in version.go first", rev, old)
		}
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d %s\n", rev, hash)), 0o644)
}

// TestRevIsBumpedWithTheBinary: a change to the forwarder's sources or to
// a dependency version must raise Rev, because the desktop app decides by
// Rev whether to replace an installed copy. When this fails: raise Rev in
// version.go, then record the new pair:
//
//	UPDATE_REV_LOCK=1 go test -run TestRevIsBumpedWithTheBinary ./cmd/anytype-mcp
//
// A Go toolchain change is not hashed; it gets a manual bump.
func TestRevIsBumpedWithTheBinary(t *testing.T) {
	hash := revHash(sources(t), modules(t))
	want := fmt.Sprintf("%d %s", Rev, hash)
	raw, err := os.ReadFile("rev.lock")
	require.NoError(t, err)
	if strings.TrimSpace(string(raw)) == want {
		return
	}
	if os.Getenv("UPDATE_REV_LOCK") != "" {
		require.NoError(t, updateLock("rev.lock", Rev, hash))
		return
	}
	t.Fatalf("the forwarder's sources or dependencies changed: raise Rev in version.go, then run this test with UPDATE_REV_LOCK=1")
}

func TestRevGuard(t *testing.T) {
	t.Run("a changed module version changes the hash", func(t *testing.T) {
		src := []string{"a.go\npackage a\n"}
		base := revHash(src, []string{"example.com/lock@v0.12.1", "example.com/sys@v0.49.0"})
		assert.NotEqual(t, base, revHash(src, []string{"example.com/lock@v0.12.2", "example.com/sys@v0.49.0"}))
		assert.NotEqual(t, base, revHash(src, []string{"example.com/lock@v0.12.1"}))
		assert.NotEqual(t, base, revHash([]string{"a.go\npackage a // changed\n"}, []string{"example.com/lock@v0.12.1", "example.com/sys@v0.49.0"}))
		assert.Equal(t, base, revHash(src, []string{"example.com/lock@v0.12.1", "example.com/sys@v0.49.0"}))
	})

	t.Run("the real module list names the dependencies with their versions", func(t *testing.T) {
		list := strings.Join(modules(t), "\n")
		assert.Contains(t, list, "github.com/gofrs/flock@v")
		assert.Contains(t, list, "golang.org/x/sys@v")
		assert.NotContains(t, list, "keychain")
		assert.NotContains(t, list, "wincred")
		assert.NotContains(t, list, "dbus")
	})

	t.Run("a replaced module changes the hash; a replacement without a version is refused", func(t *testing.T) {
		plain, err := moduleLine("example.com/lock", "v0.12.1", "", "")
		require.NoError(t, err)
		replaced, err := moduleLine("example.com/lock", "v0.12.1", "example.com/fork/lock", "v0.12.1-patched")
		require.NoError(t, err)
		assert.Equal(t, "example.com/lock@v0.12.1 => example.com/fork/lock@v0.12.1-patched", replaced)
		src := []string{"a.go\npackage a\n"}
		assert.NotEqual(t, revHash(src, []string{plain}), revHash(src, []string{replaced}))
		_, err = moduleLine("example.com/lock", "v0.12.1", "../lock", "")
		require.Error(t, err, "a local directory can't be pinned")
	})

	t.Run("the lock can't be regenerated without a bump", func(t *testing.T) {
		lock := filepath.Join(t.TempDir(), "rev.lock")
		require.NoError(t, updateLock(lock, 3, "aaa"), "no lock yet")
		require.Error(t, updateLock(lock, 3, "bbb"), "same Rev, new hash")
		require.Error(t, updateLock(lock, 2, "bbb"), "a lower Rev")
		raw, err := os.ReadFile(lock)
		require.NoError(t, err)
		assert.Equal(t, "3 aaa\n", string(raw), "a refused update writes nothing")
		require.NoError(t, updateLock(lock, 4, "bbb"))
	})
}
