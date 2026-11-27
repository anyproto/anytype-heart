package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ownPrefix   = "github.com/anyproto/anytype-heart/cmd/anytype-mcp"
	heartPrefix = "github.com/anyproto/anytype-heart/"

	// maxDeps bounds everything the forwarder links, the standard library
	// included. core/api/wrapper alone would bring 447 packages.
	maxDeps = 260
	// maxNonStdDeps bounds what is not the standard library: this
	// command's own packages plus the keychain, lock and bus libraries.
	maxNonStdDeps = 12
)

// TestDependencyBudget fails when the forwarder starts importing heart or
// anything heavy: it must stay a small static binary that starts in
// milliseconds.
func TestDependencyBudget(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH")
	}
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			cmd := exec.Command(goBin, "list", "-deps", "-f", "{{.ImportPath}} {{.Standard}}", ".")
			cmd.Env = append(os.Environ(), "GOOS="+goos)
			out, err := cmd.Output()
			require.NoError(t, err, "go list -deps")
			total, nonStd := 0, []string{}
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				pkg, std, _ := strings.Cut(line, " ")
				total++
				if std == "true" {
					continue
				}
				nonStd = append(nonStd, pkg)
				if strings.HasPrefix(pkg, heartPrefix) && !strings.HasPrefix(pkg, ownPrefix) {
					t.Errorf("the forwarder must not import heart: %s", pkg)
				}
			}
			assert.LessOrEqual(t, total, maxDeps, "go list -deps ./cmd/anytype-mcp grew")
			assert.LessOrEqual(t, len(nonStd), maxNonStdDeps, "non-standard packages: %v", nonStd)
		})
	}
}

// sourcesHash hashes the forwarder's non-test Go sources, names included,
// with line endings normalised.
func sourcesHash(t *testing.T) string {
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
	h := sha256.New()
	for _, f := range files {
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		fmt.Fprintf(h, "%s\n", f)
		h.Write([]byte(strings.ReplaceAll(string(raw), "\r\n", "\n")))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestRevIsBumpedWithTheSources: a change to the forwarder's sources must
// raise Rev, because the desktop app decides by Rev whether to replace an
// installed copy. When this fails: raise Rev in version.go if the change
// can alter behaviour, then record the new pair:
//
//	UPDATE_REV_LOCK=1 go test -run TestRevIsBumpedWithTheSources ./cmd/anytype-mcp
func TestRevIsBumpedWithTheSources(t *testing.T) {
	want := fmt.Sprintf("%d %s", Rev, sourcesHash(t))
	if os.Getenv("UPDATE_REV_LOCK") != "" {
		require.NoError(t, os.WriteFile("rev.lock", []byte(want+"\n"), 0o644))
	}
	raw, err := os.ReadFile("rev.lock")
	require.NoError(t, err)
	assert.Equal(t, want, strings.TrimSpace(string(raw)),
		"the forwarder's sources changed: raise Rev in version.go, then run this test with UPDATE_REV_LOCK=1")
}
