package files

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A junction needs no privilege to make, so it is the link a prompt-steered
// agent could plant on Windows.
func junction(t *testing.T, target, name string) {
	t.Helper()
	out, err := exec.Command("cmd", "/c", "mklink", "/J", name, target).CombinedOutput()
	if err != nil {
		t.Skipf("no junctions here: %v: %s", err, out)
	}
}

func TestJunctions(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.Mkdir(outside, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not yours"), 0o600))

	t.Run("a junction is not accepted as the folder", func(t *testing.T) {
		junction(t, outside, filepath.Join(base, "linked"))
		_, err := Open(filepath.Join(base, "linked"))
		require.ErrorContains(t, err, "not a real directory")
	})

	t.Run("a junction inside the folder that leads out is not followed", func(t *testing.T) {
		root := filepath.Join(base, "files")
		d, err := Open(root)
		require.NoError(t, err)
		defer d.Close()
		junction(t, outside, filepath.Join(root, "out"))
		_, _, err = d.OpenUpload("out/secret.txt")
		assert.Contains(t, refusal(t, err), root)
		_, _, err = d.OpenUpload(`out\secret.txt`)
		assert.Contains(t, refusal(t, err), root)
	})

	t.Run("Windows spellings of a way out are refused", func(t *testing.T) {
		root := filepath.Join(base, "files2")
		d, err := Open(root)
		require.NoError(t, err)
		defer d.Close()
		for _, path := range []string{`..\outside\secret.txt`, `sub\..\..\outside\secret.txt`, filepath.Join(outside, "secret.txt"),
			`\\?\` + filepath.Join(outside, "secret.txt"), `\\localhost\c$\Windows\win.ini`, "NUL", "CON", `sub\COM1`, "a.txt:stream", "a.txt::$DATA"} {
			_, _, err := d.OpenUpload(path)
			assert.Contains(t, refusal(t, err), root, path)
		}
	})
}
