package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// resolved is a path with its links resolved, as Dir.Path reports it.
func resolved(t *testing.T, path string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return r
}

func openDir(t *testing.T) (*Dir, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "files")
	d, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d, d.Path()
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func symlink(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
}

func refusal(t *testing.T, err error) string {
	t.Helper()
	var r *Refusal
	require.ErrorAs(t, err, &r)
	return r.Text
}

func TestOpenRules(t *testing.T) {
	t.Run("a relative path is refused", func(t *testing.T) {
		for _, path := range []string{"", "files", "./files", "../files", "~/files"} {
			_, err := Open(path)
			require.ErrorContains(t, err, "must be an absolute path", path)
		}
	})

	t.Run("a path with .. in it is refused: behind a link it names another place", func(t *testing.T) {
		base := t.TempDir()
		for _, path := range []string{filepath.Join(base, "a") + string(filepath.Separator) + ".." + string(filepath.Separator) + "b", base + "/x/../y", base + string(filepath.Separator) + ".."} {
			_, err := Open(path)
			require.ErrorContains(t, err, `without ".."`, path)
		}
		assert.Empty(t, entries(t, base), "nothing was created")
	})

	t.Run("creating the folder changes nothing that exists", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("mode bits")
		}
		parent := filepath.Join(t.TempDir(), "parent")
		require.NoError(t, os.Mkdir(parent, 0o755))
		require.NoError(t, os.Chmod(parent, 0o755))
		d, err := Open(filepath.Join(parent, "files"))
		require.NoError(t, err)
		defer d.Close()
		fi, err := os.Stat(parent)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), fi.Mode().Perm())
		// an existing folder keeps its mode too
		own := filepath.Join(parent, "own")
		require.NoError(t, os.Mkdir(own, 0o755))
		require.NoError(t, os.Chmod(own, 0o755))
		d2, err := Open(own)
		require.NoError(t, err)
		defer d2.Close()
		fi, err = os.Stat(own)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), fi.Mode().Perm())
	})

	t.Run("the folder is never Anytype's own, inside it, or around it", func(t *testing.T) {
		home := t.TempDir()
		anytype := filepath.Join(home, ".anytype")
		for _, path := range []string{home, filepath.Dir(home)} {
			_, err := Open(path, anytype)
			require.ErrorContains(t, err, "apart from Anytype's own folder", "around a folder that is not there yet: %s", path)
		}
		items := filepath.Join(anytype, "ai-connect", "items")
		require.NoError(t, os.MkdirAll(items, 0o700))
		alias := filepath.Join(home, "alias")
		linked := os.Symlink(anytype, alias) == nil
		refused := []string{home, anytype, items, filepath.Join(anytype, "new"), filepath.Join(anytype, "ai-connect")}
		if linked {
			refused = append(refused, filepath.Join(alias, "ai-connect", "items"), filepath.Join(alias, "new2"))
		}
		for _, path := range refused {
			_, err := Open(path, anytype)
			require.ErrorContains(t, err, "apart from Anytype's own folder", path)
		}
		d, err := Open(filepath.Join(home, "ai files"), anytype)
		require.NoError(t, err, "a sibling is fine")
		_ = d.Close()
	})

	t.Run("a link above the home folder hides nothing: folders are compared where they really lie", func(t *testing.T) {
		// /home -> /srv/storage/homes, and the user picks /srv/storage
		base := t.TempDir()
		storage := filepath.Join(base, "srv", "storage")
		require.NoError(t, os.MkdirAll(filepath.Join(storage, "homes", "user", ".anytype", "ai-connect"), 0o700))
		symlink(t, filepath.Join(storage, "homes"), filepath.Join(base, "home"))
		anytype := filepath.Join(base, "home", "user", ".anytype")
		for _, path := range []string{storage, filepath.Join(base, "srv"), filepath.Join(storage, "homes"), filepath.Join(storage, "homes", "user"),
			filepath.Join(storage, "homes", "user", ".anytype", "ai-connect")} {
			_, err := Open(path, anytype)
			require.ErrorContains(t, err, "apart from Anytype's own folder", path)
		}
		// and with .anytype not there yet
		require.NoError(t, os.RemoveAll(filepath.Join(storage, "homes", "user", ".anytype")))
		_, err := Open(storage, anytype)
		require.ErrorContains(t, err, "apart from Anytype's own folder")
		d, err := Open(filepath.Join(storage, "elsewhere"), anytype)
		require.NoError(t, err)
		_ = d.Close()
	})

	t.Run("a missing folder is created private, parents included", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a", "b", "files")
		d, err := Open(path)
		require.NoError(t, err)
		defer d.Close()
		assert.Equal(t, resolved(t, path), d.Path())
		assert.False(t, d.Temporary())
		fi, err := os.Lstat(path)
		require.NoError(t, err)
		assert.True(t, fi.IsDir())
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
		}
	})

	t.Run("an existing folder of the user's is taken as it is", func(t *testing.T) {
		path := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(path, "kept.txt"), []byte("x"), 0o600))
		d, err := Open(path + string(filepath.Separator))
		require.NoError(t, err)
		defer d.Close()
		assert.Equal(t, resolved(t, path), d.Path())
		assert.Equal(t, []string{"kept.txt"}, entries(t, path))
	})

	t.Run("a path spelled through a link is resolved once and that folder is pinned", func(t *testing.T) {
		base := t.TempDir()
		real := filepath.Join(base, "real")
		require.NoError(t, os.Mkdir(real, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(real, "a.txt"), []byte("chosen"), 0o600))
		symlink(t, real, filepath.Join(base, "link"))
		d, err := Open(filepath.Join(base, "link"))
		require.NoError(t, err)
		defer d.Close()
		assert.Equal(t, resolved(t, real), d.Path(), "the path reported is the resolved one")
		// the link moves on; the folder pinned at the start stays
		other := filepath.Join(base, "other")
		require.NoError(t, os.Mkdir(other, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(other, "a.txt"), []byte("not chosen"), 0o600))
		require.NoError(t, os.Remove(filepath.Join(base, "link")))
		symlink(t, other, filepath.Join(base, "link"))
		f, _, err := d.OpenUpload("a.txt")
		require.NoError(t, err)
		raw, _ := io.ReadAll(f)
		_ = f.Close()
		assert.Equal(t, "chosen", string(raw))
		assert.True(t, d.Located())
	})

	t.Run("a file where the folder should be is refused", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "files")
		require.NoError(t, os.WriteFile(path, []byte("not a directory"), 0o600))
		_, err := Open(path)
		require.ErrorContains(t, err, "must be a directory")
		_, err = Open(filepath.Join(path, "below"))
		require.Error(t, err)
	})

	t.Run("any folder of the user's choice is taken, and left as it is", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("mode bits")
		}
		for _, mode := range []os.FileMode{0o775, 0o777, 0o750, 0o700} {
			path := filepath.Join(t.TempDir(), "files")
			require.NoError(t, os.Mkdir(path, 0o700))
			require.NoError(t, os.Chmod(path, mode))
			d, err := Open(path)
			require.NoError(t, err, mode.String())
			_ = d.Close()
			fi, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, mode, fi.Mode().Perm(), "the folder's mode is the user's business")
		}
		// a folder of another owner too
		if d, err := Open(os.TempDir()); err == nil {
			_ = d.Close()
		} else {
			t.Errorf("the system's temporary folder: %v", err)
		}
	})
}

func TestTemp(t *testing.T) {
	a, err := Temp()
	require.NoError(t, err)
	b, err := Temp()
	require.NoError(t, err)
	for _, d := range []*Dir{a, b} {
		t.Cleanup(func() {
			_ = d.Close()
			_ = os.RemoveAll(d.Path())
		})
		assert.True(t, d.Temporary())
		assert.True(t, filepath.IsAbs(d.Path()))
		assert.Equal(t, resolved(t, os.TempDir()), filepath.Dir(d.Path()))
		fi, err := os.Lstat(d.Path())
		require.NoError(t, err)
		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
		}
		assert.Empty(t, entries(t, d.Path()))
	}
	assert.NotEqual(t, a.Path(), b.Path())
}

func TestOpenUpload(t *testing.T) {
	read := func(t *testing.T, d *Dir, path string) (string, string, error) {
		f, name, err := d.OpenUpload(path)
		if err != nil {
			return "", "", err
		}
		defer f.Close()
		raw, err := io.ReadAll(f)
		require.NoError(t, err)
		return string(raw), name, nil
	}

	t.Run("opens a regular file by a relative path", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "today.txt"), []byte("hello"), 0o600))
		for _, path := range []string{"notes/today.txt", "./notes/today.txt", "notes//today.txt", "notes/./today.txt"} {
			content, name, err := read(t, d, path)
			require.NoError(t, err, path)
			assert.Equal(t, "hello", content)
			assert.Equal(t, "today.txt", name)
		}
	})

	t.Run("any .. in the path is refused, wherever it leads", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "today.txt"), []byte("hello"), 0o600))
		for _, path := range []string{"notes/missing/../today.txt", "notes/../notes/today.txt", "..", "../x", "notes/.."} {
			_, _, err := d.OpenUpload(path)
			assert.Contains(t, refusal(t, err), "is outside", path)
		}
	})

	t.Run("a hard link in the folder is a file in the folder", func(t *testing.T) {
		d, root := openDir(t)
		source := filepath.Join(filepath.Dir(root), "source.txt")
		require.NoError(t, os.WriteFile(source, []byte("linked in on purpose"), 0o600))
		if err := os.Link(source, filepath.Join(root, "linked.txt")); err != nil {
			t.Skipf("no hard links here: %v", err)
		}
		content, _, err := read(t, d, "linked.txt")
		require.NoError(t, err)
		assert.Equal(t, "linked in on purpose", content)
	})

	t.Run("a folder on the way may be a link that stays inside", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "notes"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "notes", "today.txt"), []byte("hello"), 0o600))
		symlink(t, "notes", filepath.Join(root, "alias"))
		content, _, err := read(t, d, "alias/today.txt")
		require.NoError(t, err)
		assert.Equal(t, "hello", content)
	})

	t.Run("the folder is the one opened, whatever takes its path later", func(t *testing.T) {
		base := t.TempDir()
		path := filepath.Join(base, "files")
		d, err := Open(path)
		require.NoError(t, err)
		defer d.Close()
		require.NoError(t, os.WriteFile(filepath.Join(path, "a.txt"), []byte("chosen"), 0o600))
		other := filepath.Join(base, "other")
		require.NoError(t, os.Mkdir(other, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(other, "a.txt"), []byte("not yours"), 0o600))
		assert.True(t, d.Located())
		if err := os.Rename(path, filepath.Join(base, "moved")); err != nil {
			t.Skipf("an open folder can't be renamed here: %v", err)
		}
		assert.False(t, d.Located(), "nothing is at the path")
		symlink(t, other, path)
		assert.False(t, d.Located(), "the path names another folder now")
		content, _, err := read(t, d, "a.txt")
		require.NoError(t, err)
		assert.Equal(t, "chosen", content)
		saved, _, err := d.Save(ctx, "b.txt", strings.NewReader("x"))
		require.NoError(t, err)
		assert.Equal(t, []string{"a.txt"}, entries(t, other), "nothing is written through the path")
		assert.Contains(t, entries(t, filepath.Join(base, "moved")), saved)
	})

	t.Run("a name swapped for a link during the call is refused", func(t *testing.T) {
		for name, target := range map[string]func(base, root string) string{
			"to a file outside": func(base, _ string) string { return filepath.Join(base, "secret.txt") },
			"to a file inside":  func(string, string) string { return "other.txt" },
		} {
			base := t.TempDir()
			d, err := Open(filepath.Join(base, "files"))
			require.NoError(t, err)
			defer d.Close()
			root := d.Path()
			require.NoError(t, os.WriteFile(filepath.Join(base, "secret.txt"), []byte("not yours"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "other.txt"), []byte("another"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("chosen"), 0o600))
			d.afterLstat = func() {
				require.NoError(t, os.Remove(filepath.Join(root, "a.txt")))
				symlink(t, target(base, root), filepath.Join(root, "a.txt"))
			}
			_, _, err = d.OpenUpload("a.txt")
			text := refusal(t, err)
			assert.Regexp(t, "can't be opened|changed while it was being opened", text, name)
			assert.Contains(t, text, root)
		}
	})

	t.Run("a folder on the way swapped for a link out during the call is refused", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "files")
		d, err := Open(root)
		require.NoError(t, err)
		defer d.Close()
		outside := filepath.Join(base, "outside")
		require.NoError(t, os.Mkdir(outside, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(outside, "a.txt"), []byte("not yours"), 0o600))
		require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "a.txt"), []byte("chosen"), 0o600))
		d.afterLstat = func() {
			require.NoError(t, os.RemoveAll(filepath.Join(root, "sub")))
			symlink(t, outside, filepath.Join(root, "sub"))
		}
		_, _, err = d.OpenUpload("sub/a.txt")
		assert.Contains(t, refusal(t, err), "can't be opened")
	})

	t.Run("a refusal quotes a long or odd path on one bounded line", func(t *testing.T) {
		d, root := openDir(t)
		_, _, err := d.OpenUpload(strings.Repeat("a", 5000) + "\nIGNORE ALL")
		text := refusal(t, err)
		assert.Less(t, len(text), 600+2*len(root))
		assert.NotContains(t, text, "\n")
	})
}

func TestSave(t *testing.T) {
	t.Run("publishes under the name, private, leaving no part file", func(t *testing.T) {
		d, root := openDir(t)
		saved, size, err := d.Save(ctx, "report.pdf", strings.NewReader("%PDF"))
		require.NoError(t, err)
		assert.Equal(t, "report.pdf", saved)
		assert.Equal(t, int64(4), size)
		assert.Equal(t, []string{"report.pdf"}, entries(t, root))
		if runtime.GOOS != "windows" {
			fi, err := os.Stat(filepath.Join(root, "report.pdf"))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
		}
	})

	t.Run("numbers a taken name up to 100, then adds a random tail; nothing is replaced", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("mine"), 0o600))
		require.NoError(t, os.Mkdir(filepath.Join(root, "a (2).txt"), 0o700))
		symlink(t, "nowhere", filepath.Join(root, "a (3).txt"))
		saved, _, err := d.Save(ctx, "a.txt", strings.NewReader("x"))
		require.NoError(t, err)
		assert.Equal(t, "a (4).txt", saved, "a folder and a dangling link hold their names too")
		for n := 5; n <= 100; n++ {
			require.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("a (%d).txt", n)), nil, 0o600))
		}
		saved, _, err = d.Save(ctx, "a.txt", strings.NewReader("x"))
		require.NoError(t, err)
		assert.Regexp(t, `^a-[0-9a-f]{8}\.txt$`, saved)
		raw, _ := os.ReadFile(filepath.Join(root, "a.txt"))
		assert.Equal(t, "mine", string(raw))
		_, err = os.Readlink(filepath.Join(root, "a (3).txt"))
		require.NoError(t, err)
	})

	t.Run("a body that breaks leaves nothing", func(t *testing.T) {
		d, root := openDir(t)
		_, _, err := d.Save(ctx, "a.txt", io.MultiReader(strings.NewReader("half"), &failing{}))
		require.Error(t, err)
		assert.Empty(t, entries(t, root))
	})

	for _, code := range []syscall.Errno{syscall.EPERM, syscall.ENOTSUP, syscall.EXDEV} {
		t.Run("without hard links ("+code.Error()+") the download fails and leaves nothing", func(t *testing.T) {
			d, root := openDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("mine"), 0o600))
			d.link = func(oldname, newname string) error {
				return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: code}
			}
			_, _, err := d.Save(ctx, "b.txt", strings.NewReader("hello"))
			text := refusal(t, err)
			assert.Regexp(t, "could not be saved in .*hard link, which failed.*not FAT or exFAT.*nothing was saved", text)
			assert.Contains(t, text, root)
			assert.Equal(t, []string{"a.txt"}, entries(t, root))
		})
	}

	t.Run("a transfer that ended before publication publishes nothing", func(t *testing.T) {
		d, root := openDir(t)
		ctx, cancel := context.WithCancel(context.Background())
		// the body is complete; the cancellation arrives before the name
		// is given
		d.beforePublish = cancel
		_, _, err := d.Save(ctx, "a.txt", strings.NewReader("whole"))
		require.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, entries(t, root))
	})

	t.Run("what is published is the file that was written, or nothing is reported", func(t *testing.T) {
		d, root := openDir(t)
		d.beforePublish = func() {
			// another writer takes the part file's name
			for _, name := range entries(t, root) {
				require.NoError(t, os.Remove(filepath.Join(root, name)))
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("planted"), 0o600))
			}
		}
		_, _, err := d.Save(ctx, "a.txt", strings.NewReader("whole"))
		require.ErrorContains(t, err, "changed while")
		// nothing under the final name, and the other writer's entry stays
		left := entries(t, root)
		require.Len(t, left, 1)
		assert.True(t, strings.HasSuffix(left[0], partSuffix), left[0])
		raw, _ := os.ReadFile(filepath.Join(root, left[0]))
		assert.Equal(t, "planted", string(raw))
	})

	t.Run("a part file swapped in the instant before the link: the link is taken back", func(t *testing.T) {
		d, root := openDir(t)
		d.beforeLink = func(string) {
			for _, name := range entries(t, root) {
				require.NoError(t, os.Remove(filepath.Join(root, name)))
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("planted"), 0o600))
			}
		}
		_, _, err := d.Save(ctx, "a.txt", strings.NewReader("whole"))
		require.ErrorContains(t, err, "changed while")
		left := entries(t, root)
		require.Len(t, left, 1, "the final name is gone again")
		assert.True(t, strings.HasSuffix(left[0], partSuffix), left[0])
	})

	t.Run("a transfer that ends between two name attempts publishes nothing", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("mine"), 0o600))
		ctx, cancel := context.WithCancel(context.Background())
		d.beforeLink = func(string) { cancel() } // during the attempt on the taken name
		_, _, err := d.Save(ctx, "a.txt", strings.NewReader("whole"))
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, []string{"a.txt"}, entries(t, root))
	})

	t.Run("a name can't leave the folder or hide in it", func(t *testing.T) {
		d, root := openDir(t)
		for _, name := range []string{"../../escape.txt", "/etc/passwd", `..\..\escape.txt`, ".hidden", "..", "", partPrefix + "x" + partSuffix} {
			saved, _, err := d.Save(ctx, name, strings.NewReader("x"))
			require.NoError(t, err, name)
			assert.Equal(t, saved, filepath.Base(saved))
			assert.False(t, strings.HasPrefix(saved, "."), saved)
		}
		assert.Len(t, entries(t, root), 7)
		assert.Len(t, entries(t, filepath.Dir(root)), 1)
	})
}

type failing struct{}

func (*failing) Read([]byte) (int, error) { return 0, errors.New("the connection broke") }

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"report.pdf":                  "report.pdf",
		"Q3 report (final).pdf":       "Q3 report (final).pdf",
		"報告.pdf":                      "報告.pdf",
		"a/b\\c:d*e?f\"g<h>i|j":       "a_b_c_d_e_f_g_h_i_j",
		"a\x00\x01\x1f\x7fb":          "a_b",
		"line\r\nbreak.txt":           "line_break.txt",
		"  spaced .txt  ":             "spaced.txt",
		"...dots...":                  "dots",
		".bashrc":                     "bashrc",
		"trailing. . .":               "trailing",
		"":                            "download",
		"...":                         "download",
		"/":                           "_",
		"CON":                         "download",
		"con.txt":                     "download.txt",
		"NUL.tar.gz":                  "download.gz",
		"aux .txt":                    "download.txt",
		"COM1.log":                    "download.log",
		"lpt9":                        "download",
		"COM¹.txt":                    "download.txt",
		"CONIN$":                      "download",
		"console.txt":                 "console.txt",
		"com10.txt":                   "com10.txt",
		"evil\u202Etxt.exe":           "evil_txt.exe",
		"bad\xffutf8.txt":             "bad_utf8.txt",
		"name.":                       "name",
		"name.tar.":                   "name.tar",
		"x.thisisnotanextensionatall": "x.thisisnotanextensionatall",
	} {
		assert.Equal(t, want, SafeName(in), "%q", in)
	}

	t.Run("keeps the extension when it shortens a long name", func(t *testing.T) {
		name := SafeName(strings.Repeat("a", 300) + ".pdf")
		assert.Len(t, name, 120)
		assert.True(t, strings.HasSuffix(name, ".pdf"))
	})

	t.Run("stays within 255 bytes with room for the collision suffix, splitting no character", func(t *testing.T) {
		name := SafeName(strings.Repeat("報", 83) + ".pdf")
		worst := strings.TrimSuffix(name, ".pdf") + " (100).pdf"
		assert.LessOrEqual(t, len(worst), 255)
		assert.True(t, strings.HasSuffix(name, ".pdf"))
		assert.True(t, utf8.ValidString(name))
		emoji := SafeName(strings.Repeat("😀", 100) + ".txt")
		assert.Regexp(t, `^(😀)+\.txt$`, emoji)
		assert.LessOrEqual(t, len(strings.TrimSuffix(emoji, ".txt"))/4*2+4, 120, "UTF-16 units")
	})

	t.Run("every result is a plain name, whatever goes in", func(t *testing.T) {
		for _, in := range []string{"a", "\x00", "..", "../..", `C:\x`, "~", " ", "\t.\t", strings.Repeat(".", 400), strings.Repeat("é", 400), "a." + strings.Repeat("b", 400)} {
			out := SafeName(in)
			assert.NotEmpty(t, out)
			assert.LessOrEqual(t, len(out), 245)
			assert.Equal(t, out, filepath.Base(out))
			assert.NotContains(t, out, `\`)
			assert.NotEqual(t, ".", out[:1])
			assert.Equal(t, out, strings.TrimSpace(out))
			assert.False(t, strings.HasSuffix(out, "."))
			assert.Equal(t, out, SafeName(out), "safe names stay as they are")
		}
	})
}

// Theme K: where the open folder really lies decides, not the path it was
// opened by: a link swapped in between the lookup and the open (another
// user renaming entries in a shared parent) can't pin a folder inside
// Anytype's own.
func TestH1R2_K_PinnedFolderIsJudgedByItsHandle(t *testing.T) {
	base := t.TempDir()
	anytype := filepath.Join(base, "home", ".anytype")
	inside := filepath.Join(anytype, "ai-connect")
	require.NoError(t, os.MkdirAll(inside, 0o700))
	elsewhere := filepath.Join(base, "shared", "files")
	require.NoError(t, os.MkdirAll(elsewhere, 0o700))
	root, err := os.OpenRoot(inside)
	require.NoError(t, err)
	info, err := root.Stat(".")
	require.NoError(t, err)
	// the folder that was opened is inside .anytype; its path says elsewhere
	d := &Dir{path: elsewhere, root: root, info: info}
	defer d.Close()
	assert.True(t, d.overlaps(anytype))
	assert.True(t, d.overlaps(filepath.Join(base, "home")), "nor around it")

	plain, err := Open(elsewhere, anytype)
	require.NoError(t, err)
	assert.NoError(t, plain.Close())
}
