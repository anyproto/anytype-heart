package block

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureWithinScope(t *testing.T) {
	scope := t.TempDir()

	t.Run("dir inside scope is allowed", func(t *testing.T) {
		in := filepath.Join(scope, "anytype-download")
		got, err := ensureWithinScope(scope, in)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(resolved(t, scope), "anytype-download"), got)
	})

	t.Run("scope root itself is allowed", func(t *testing.T) {
		got, err := ensureWithinScope(scope, scope)
		require.NoError(t, err)
		assert.Equal(t, resolved(t, scope), got)
	})

	t.Run("sibling with shared prefix is rejected", func(t *testing.T) {
		_, err := ensureWithinScope(scope, scope+"-evil")
		require.Error(t, err)
	})

	t.Run("parent traversal is rejected", func(t *testing.T) {
		_, err := ensureWithinScope(scope, filepath.Join(scope, "..", "evil"))
		require.Error(t, err)
	})

	t.Run("unrelated absolute path is rejected", func(t *testing.T) {
		_, err := ensureWithinScope(scope, filepath.Join(string(os.PathSeparator), "etc"))
		require.Error(t, err)
	})

	// A shared temp location is writable by other local users, so the path may
	// pass the textual prefix check and still resolve outside the scope.
	t.Run("symlink out of the scope is rejected", func(t *testing.T) {
		// given
		outside := t.TempDir()
		link := filepath.Join(scope, "escape")
		require.NoError(t, os.Symlink(outside, link))

		// then
		_, err := ensureWithinScope(scope, link)
		require.Error(t, err)
	})

	t.Run("not-yet-created dir under an escaping symlink is rejected", func(t *testing.T) {
		// given
		outside := t.TempDir()
		link := filepath.Join(scope, "escape-parent")
		require.NoError(t, os.Symlink(outside, link))

		// then: the leaf does not exist yet, which is the normal case — the
		// containment decision still has to resolve the parent.
		_, err := ensureWithinScope(scope, filepath.Join(link, "anytype-download"))
		require.Error(t, err)
	})

	t.Run("symlink pointing back into the scope is allowed and resolved", func(t *testing.T) {
		// given
		target := filepath.Join(scope, "real")
		require.NoError(t, os.Mkdir(target, 0755))
		link := filepath.Join(scope, "alias")
		require.NoError(t, os.Symlink(target, link))

		// when
		got, err := ensureWithinScope(scope, filepath.Join(link, "anytype-download"))

		// then
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(resolved(t, target), "anytype-download"), got)
	})

	// macOS hands out temp dirs under /var, itself a symlink to /private/var,
	// so the scope and the request can spell the same directory differently.
	t.Run("scope reached through a symlinked ancestor is allowed", func(t *testing.T) {
		// given
		realScope := t.TempDir()
		aliasRoot := filepath.Join(t.TempDir(), "alias")
		require.NoError(t, os.Symlink(realScope, aliasRoot))

		// when
		got, err := ensureWithinScope(aliasRoot, filepath.Join(realScope, "anytype-download"))

		// then
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(resolved(t, realScope), "anytype-download"), got)
	})
}

// resolved is the real path of dir, which is what the containment check returns
// — on macOS the temp root is reached through /var, a symlink to /private/var.
func resolved(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return real
}

func TestDownloadTargetDir(t *testing.T) {
	newService := func(tempDir string, sandboxed bool) *Service {
		s := &Service{tempDirProvider: stubTempDirProvider(tempDir)}
		if sandboxed {
			s.downloadScopeDir = tempDir
		}
		return s
	}

	t.Run("sandboxed: an empty path falls back to the temp default", func(t *testing.T) {
		// given
		tempDir := t.TempDir()
		s := newService(tempDir, true)

		// when
		got, err := s.downloadTargetDir("")

		// then
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(resolved(t, tempDir), "anytype-download"), got)
	})

	t.Run("sandboxed: a path inside the scope is allowed", func(t *testing.T) {
		// given
		tempDir := t.TempDir()
		s := newService(tempDir, true)

		// when
		got, err := s.downloadTargetDir(filepath.Join(tempDir, "sub", "dir"))

		// then
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(resolved(t, tempDir), "sub", "dir"), got)
	})

	t.Run("sandboxed: a path outside the scope is refused", func(t *testing.T) {
		// given
		tempDir := t.TempDir()
		outside := filepath.Join(t.TempDir(), ".zlogin")
		s := newService(tempDir, true)

		// when
		_, err := s.downloadTargetDir(outside)

		// then
		require.Error(t, err)
	})

	t.Run("sandboxed: traversal out of the scope is refused", func(t *testing.T) {
		// given
		tempDir := t.TempDir()
		s := newService(tempDir, true)

		// when
		_, err := s.downloadTargetDir(filepath.Join(tempDir, "..", "elsewhere"))

		// then
		require.Error(t, err)
	})

	// Mobile calls the middleware in-process, so the caller is the app itself
	// and its chosen destination is honored.
	t.Run("unsandboxed: an arbitrary path is honored", func(t *testing.T) {
		// given
		tempDir := t.TempDir()
		outside := filepath.Join(t.TempDir(), "Downloads")
		s := newService(tempDir, false)

		// when
		got, err := s.downloadTargetDir(outside)

		// then
		require.NoError(t, err)
		assert.Equal(t, outside, got)
	})

	t.Run("unsandboxed: an empty path falls back to the temp default", func(t *testing.T) {
		// given
		tempDir := t.TempDir()
		s := newService(tempDir, false)

		// when
		got, err := s.downloadTargetDir("")

		// then
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(tempDir, "anytype-download"), got)
	})
}

// The name comes from file metadata, which the party that shared the file
// controls, so it must never widen where the write lands.
func TestDownloadFileName(t *testing.T) {
	t.Run("a plain name is kept", func(t *testing.T) {
		assert.Equal(t, "report.pdf", downloadFileName("report.pdf"))
	})

	t.Run("traversal is stripped", func(t *testing.T) {
		assert.Equal(t, "evil", downloadFileName(filepath.Join("..", "..", "evil")))
	})

	t.Run("an absolute name is reduced to its last element", func(t *testing.T) {
		assert.Equal(t, "passwd", downloadFileName(filepath.Join(string(os.PathSeparator), "etc", "passwd")))
	})

	t.Run("names that are not a file element get a fallback", func(t *testing.T) {
		for _, name := range []string{"", ".", "..", string(os.PathSeparator)} {
			got := downloadFileName(name)
			assert.NotContains(t, got, string(os.PathSeparator))
			assert.NotEqual(t, ".", got)
			assert.NotEqual(t, "..", got)
			assert.NotEmpty(t, got)
		}
	})
}

type stubTempDirProvider string

func (p stubTempDirProvider) TempDir() string { return string(p) }

func TestDownloadScopeForGOOS(t *testing.T) {
	const tmp = "/var/tmp/anytype"

	t.Run("mobile platforms get no scope", func(t *testing.T) {
		assert.Empty(t, downloadScopeForGOOS("ios", tmp))
		assert.Empty(t, downloadScopeForGOOS("android", tmp))
	})

	t.Run("desktop and server platforms scope to the temp dir", func(t *testing.T) {
		assert.Equal(t, tmp, downloadScopeForGOOS("darwin", tmp))
		assert.Equal(t, tmp, downloadScopeForGOOS("linux", tmp))
		assert.Equal(t, tmp, downloadScopeForGOOS("windows", tmp))
	})
}
