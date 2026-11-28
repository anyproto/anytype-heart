//go:build !windows

package store

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// privateHome: a scratch home on Unix is already a normal one.
func privateHome(*testing.T, string) {}

// seeded is a sound endpoint file.
func seeded(t *testing.T) *File {
	f := newFile(t)
	write(t, f, `{"v":1,"port":31009}`)
	_, err := f.Read(ctx)
	require.NoError(t, err)
	return f
}

// foreign makes one path belong to another user, as the rules see it.
func foreign(f *File, path string) {
	me := os.Geteuid()
	f.OwnerRule(func(p string) int {
		if p == path {
			return me + 1
		}
		return me
	})
}

func deniedOn(t *testing.T, err error, path, rule string) {
	t.Helper()
	e := requireReason(t, err, ReasonDenied, rule)
	assert.Equal(t, path, e.Path)
	assert.Contains(t, e.Rule, rule)
}

// .anytype and ai-connect: real directories of this user that nobody
// else can write. Their mode is otherwise free.
func TestFolderRule(t *testing.T) {
	for _, mode := range []os.FileMode{0o700, 0o755, 0o711} {
		f := seeded(t)
		require.NoError(t, os.Chmod(filepath.Dir(f.Path()), mode))
		_, err := f.Read(ctx)
		assert.NoError(t, err, "%o", mode)
	}
	for _, name := range []string{".anytype", "ai-connect"} {
		t.Run(name, func(t *testing.T) {
			for mode, rule := range map[os.FileMode]string{0o775: "writable by others", 0o707: "writable by others"} {
				f := seeded(t)
				dir := filepath.Join(f.home, ".anytype")
				if name == "ai-connect" {
					dir = filepath.Dir(f.Path())
				}
				require.NoError(t, os.Chmod(dir, mode))
				_, err := f.Read(ctx)
				deniedOn(t, err, dir, rule)
			}

			f := seeded(t)
			dir := filepath.Join(f.home, ".anytype")
			if name == "ai-connect" {
				dir = filepath.Dir(f.Path())
			}
			foreign(f, dir)
			_, err := f.Read(ctx)
			deniedOn(t, err, dir, "belongs to another user")

			real := dir + "-real"
			require.NoError(t, os.Rename(dir, real))
			require.NoError(t, os.Symlink(real, dir))
			_, err = OpenAt(f.home, NamespaceDev).Read(ctx)
			deniedOn(t, err, dir, "not a real directory")
		})
	}
}

// The file: regular, one link, this user's, nobody else may write it.
// Others may read it: it holds no secret.
func TestFileRule(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o644, 0o400} {
		f := seeded(t)
		require.NoError(t, os.Chmod(f.Path(), mode))
		_, err := f.Read(ctx)
		assert.NoError(t, err, "%o", mode)
	}
	for name, hurt := range map[string]func(t *testing.T, f *File) string{
		"group-writable": func(t *testing.T, f *File) string {
			require.NoError(t, os.Chmod(f.Path(), 0o660))
			return "writable by others"
		},
		"other-writable": func(t *testing.T, f *File) string {
			require.NoError(t, os.Chmod(f.Path(), 0o602))
			return "writable by others"
		},
		"a symlink to a sound file": func(t *testing.T, f *File) string {
			good := filepath.Join(t.TempDir(), "good.json")
			require.NoError(t, os.WriteFile(good, []byte(`{"v":1,"port":1}`), 0o600))
			require.NoError(t, os.Remove(f.Path()))
			require.NoError(t, os.Symlink(good, f.Path()))
			return "not a regular file"
		},
		"a FIFO": func(t *testing.T, f *File) string {
			require.NoError(t, os.Remove(f.Path()))
			require.NoError(t, syscall.Mkfifo(f.Path(), 0o600))
			return "not a regular file"
		},
		"a folder": func(t *testing.T, f *File) string {
			require.NoError(t, os.Remove(f.Path()))
			require.NoError(t, os.Mkdir(f.Path(), 0o700))
			return "not a regular file"
		},
		"two links": func(t *testing.T, f *File) string {
			require.NoError(t, os.Link(f.Path(), filepath.Join(f.home, "second")))
			return "more than one link"
		},
		"owned by another user": func(t *testing.T, f *File) string {
			foreign(f, f.Path())
			return "belongs to another user"
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := seeded(t)
			rule := hurt(t, f)
			_, err := f.Read(ctx)
			deniedOn(t, err, f.Path(), rule)
			assert.Contains(t, err.Error(), "the endpoint file")
		})
	}
}

func TestUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	f := seeded(t)
	dir := filepath.Dir(f.Path())
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	_, err := f.Read(ctx)
	requireReason(t, err, ReasonUnreachable, "an unreadable folder")
}

// The chain starts at home: no other user may own it or swap it for a
// link; its mode is not judged.
func TestHomeRule(t *testing.T) {
	f := seeded(t)
	require.NoError(t, os.Chmod(f.home, 0o775))
	_, err := f.Read(ctx)
	require.NoError(t, err, "a group-writable home is a normal setup")

	foreign(f, f.home)
	_, err = f.Read(ctx)
	deniedOn(t, err, f.home, "belongs to another user")

	real := seeded(t)
	link := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.Symlink(real.home, link))
	_, err = OpenAt(link, NamespaceDev).Read(ctx)
	deniedOn(t, err, link, "not a real directory")
}

// The endpoint verb's rule for its own executable under .anytype: the
// folders down to it are strict folders, the file a strict file.
func TestCheckExecutableRules(t *testing.T) {
	setup := func(t *testing.T) (*File, string) {
		f := seeded(t)
		bin := filepath.Join(f.home, ".anytype", "bin")
		require.NoError(t, os.Mkdir(bin, 0o700))
		exe := filepath.Join(bin, "anytype-mcp")
		require.NoError(t, os.WriteFile(exe, []byte("binary"), 0o755))
		require.NoError(t, f.CheckExecutable(exe, exe))
		return f, exe
	}
	t.Run("a group-writable bin", func(t *testing.T) {
		f, exe := setup(t)
		require.NoError(t, os.Chmod(filepath.Dir(exe), 0o775))
		deniedOn(t, f.CheckExecutable(exe, exe), filepath.Dir(exe), "writable by others")
	})
	t.Run("bin owned by another user", func(t *testing.T) {
		f, exe := setup(t)
		foreign(f, filepath.Dir(exe))
		deniedOn(t, f.CheckExecutable(exe, exe), filepath.Dir(exe), "belongs to another user")
	})
	t.Run("a group-writable .anytype", func(t *testing.T) {
		f, exe := setup(t)
		require.NoError(t, os.Chmod(filepath.Join(f.home, ".anytype"), 0o775))
		deniedOn(t, f.CheckExecutable(exe, exe), filepath.Join(f.home, ".anytype"), "writable by others")
	})
	t.Run("an executable with two links", func(t *testing.T) {
		f, exe := setup(t)
		require.NoError(t, os.Link(exe, filepath.Join(f.home, "copy")))
		deniedOn(t, f.CheckExecutable(exe, exe), exe, "the forwarder's executable has more than one link")
	})
	t.Run("a group-writable executable", func(t *testing.T) {
		f, exe := setup(t)
		require.NoError(t, os.Chmod(exe, 0o775))
		deniedOn(t, f.CheckExecutable(exe, exe), exe, "writable by others")
	})
	t.Run("a launch path that reaches .anytype through a link is refused, not judged resolved", func(t *testing.T) {
		f, exe := setup(t)
		for _, alias := range []string{filepath.Join(t.TempDir(), "bin"), filepath.Join(t.TempDir(), "anytype-mcp")} {
			target := filepath.Dir(exe)
			if filepath.Base(alias) == "anytype-mcp" {
				target = exe
			}
			require.NoError(t, os.Symlink(target, alias))
			launch := alias
			if filepath.Base(alias) == "bin" {
				launch = filepath.Join(alias, "anytype-mcp")
			}
			// the path passes no .anytype by its spelling; the file it names
			// really lies there. The denial names it as spelled, whatever
			// the system reports for the running file (Linux: resolved)
			deniedOn(t, f.CheckExecutable(launch, exe), launch, "another spelling or through a link")
			deniedOn(t, f.CheckExecutable(launch, launch), launch, "another spelling or through a link")
		}
		// a home spelled through a link while the path is spelled resolved
		link := filepath.Join(t.TempDir(), "home")
		require.NoError(t, os.Symlink(f.home, link))
		real, err := filepath.EvalSymlinks(exe)
		require.NoError(t, err)
		deniedOn(t, OpenAt(link, NamespaceDev).CheckExecutable(real, exe), real, "another spelling or through a link")
	})
	t.Run("a bin that is a link to a folder outside", func(t *testing.T) {
		f, exe := setup(t)
		outside := filepath.Join(t.TempDir(), "outside")
		require.NoError(t, os.Mkdir(outside, 0o700))
		require.NoError(t, os.Rename(exe, filepath.Join(outside, "anytype-mcp")))
		require.NoError(t, os.Remove(filepath.Dir(exe)))
		require.NoError(t, os.Symlink(outside, filepath.Dir(exe)))
		deniedOn(t, f.CheckExecutable(exe, filepath.Join(outside, "anytype-mcp")), filepath.Dir(exe), "not a real directory")
	})
	t.Run("an executable that is a symlink", func(t *testing.T) {
		f, exe := setup(t)
		real := filepath.Join(t.TempDir(), "real")
		require.NoError(t, os.WriteFile(real, []byte("binary"), 0o755))
		require.NoError(t, os.Remove(exe))
		require.NoError(t, os.Symlink(real, exe))
		deniedOn(t, f.CheckExecutable(exe, exe), exe, "not a regular file")
	})
}
