package files

import (
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

func openDir(t *testing.T) (*Dir, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "files")
	d, err := Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d, path
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

	t.Run("a missing folder is created private, parents included", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a", "b", "files")
		d, err := Open(path)
		require.NoError(t, err)
		defer d.Close()
		assert.Equal(t, path, d.Path())
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
		assert.Equal(t, filepath.Clean(path), d.Path())
		assert.Equal(t, []string{"kept.txt"}, entries(t, path))
	})

	t.Run("a link to a folder is refused", func(t *testing.T) {
		base := t.TempDir()
		real := filepath.Join(base, "real")
		require.NoError(t, os.Mkdir(real, 0o700))
		symlink(t, real, filepath.Join(base, "link"))
		_, err := Open(filepath.Join(base, "link"))
		require.ErrorContains(t, err, "not a real directory")
	})

	t.Run("a file where the folder should be is refused", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "files")
		require.NoError(t, os.WriteFile(path, []byte("not a directory"), 0o600))
		_, err := Open(path)
		require.ErrorContains(t, err, "not a real directory")
		_, err = Open(filepath.Join(path, "below"))
		require.Error(t, err)
	})

	t.Run("a folder others can write is refused", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("mode bits")
		}
		for _, mode := range []os.FileMode{0o770, 0o707, 0o777, 0o720} {
			path := filepath.Join(t.TempDir(), "files")
			require.NoError(t, os.Mkdir(path, 0o700))
			require.NoError(t, os.Chmod(path, mode))
			_, err := Open(path)
			require.ErrorContains(t, err, "writable by other users", mode.String())
		}
		path := filepath.Join(t.TempDir(), "files")
		require.NoError(t, os.Mkdir(path, 0o700))
		require.NoError(t, os.Chmod(path, 0o755))
		d, err := Open(path)
		require.NoError(t, err, "readable by others is the user's choice")
		_ = d.Close()
	})

	t.Run("another user's folder is refused", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("needs a folder of another user")
		}
		_, err := Open("/")
		require.ErrorContains(t, err, "belongs to another user")
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
		assert.Equal(t, filepath.Clean(os.TempDir()), filepath.Dir(d.Path()))
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
		for _, path := range []string{"notes/today.txt", "./notes/today.txt", "notes//today.txt", "notes/./today.txt", "notes/x/../today.txt"} {
			content, name, err := read(t, d, path)
			if path == "notes/x/../today.txt" {
				// no "x" there: the system refuses, or the path is cleaned
				if err == nil {
					assert.Equal(t, "hello", content)
				}
				continue
			}
			require.NoError(t, err, path)
			assert.Equal(t, "hello", content)
			assert.Equal(t, "today.txt", name)
		}
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
		if err := os.Rename(path, filepath.Join(base, "moved")); err != nil {
			t.Skipf("an open folder can't be renamed here: %v", err)
		}
		symlink(t, other, path)
		content, _, err := read(t, d, "a.txt")
		require.NoError(t, err)
		assert.Equal(t, "chosen", content)
		saved, _, err := d.Save("b.txt", strings.NewReader("x"))
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
			root := filepath.Join(base, "files")
			d, err := Open(root)
			require.NoError(t, err)
			defer d.Close()
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
		saved, size, err := d.Save("report.pdf", strings.NewReader("%PDF"))
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
		saved, _, err := d.Save("a.txt", strings.NewReader("x"))
		require.NoError(t, err)
		assert.Equal(t, "a (4).txt", saved, "a folder and a dangling link hold their names too")
		for n := 5; n <= 100; n++ {
			require.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("a (%d).txt", n)), nil, 0o600))
		}
		saved, _, err = d.Save("a.txt", strings.NewReader("x"))
		require.NoError(t, err)
		assert.Regexp(t, `^a-[0-9a-f]{8}\.txt$`, saved)
		raw, _ := os.ReadFile(filepath.Join(root, "a.txt"))
		assert.Equal(t, "mine", string(raw))
		_, err = os.Readlink(filepath.Join(root, "a (3).txt"))
		require.NoError(t, err)
	})

	t.Run("a body that breaks leaves nothing", func(t *testing.T) {
		d, root := openDir(t)
		_, _, err := d.Save("a.txt", io.MultiReader(strings.NewReader("half"), &failing{}))
		require.Error(t, err)
		assert.Empty(t, entries(t, root))
	})

	for _, code := range []syscall.Errno{syscall.EPERM, syscall.ENOTSUP, syscall.EXDEV} {
		t.Run("without hard links ("+code.Error()+") the name is taken first and nothing is replaced", func(t *testing.T) {
			d, root := openDir(t)
			require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("mine"), 0o600))
			d.link = func(oldname, newname string) error {
				return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: code}
			}
			saved, size, err := d.Save("a.txt", strings.NewReader("hello"))
			require.NoError(t, err)
			assert.Equal(t, "a (2).txt", saved)
			assert.Equal(t, int64(5), size)
			assert.Equal(t, []string{"a (2).txt", "a.txt"}, entries(t, root))
			raw, _ := os.ReadFile(filepath.Join(root, "a.txt"))
			assert.Equal(t, "mine", string(raw))
			raw, _ = os.ReadFile(filepath.Join(root, "a (2).txt"))
			assert.Equal(t, "hello", string(raw))
		})
	}

	t.Run("a name can't leave the folder or hide in it", func(t *testing.T) {
		d, root := openDir(t)
		for _, name := range []string{"../../escape.txt", "/etc/passwd", `..\..\escape.txt`, ".hidden", "..", "", partPrefix + "x" + partSuffix} {
			saved, _, err := d.Save(name, strings.NewReader("x"))
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
