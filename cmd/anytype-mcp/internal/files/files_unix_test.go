//go:build !windows

package files

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A FIFO never makes an upload wait for a writer: not when it is there
// from the start, not when it takes the name during the call.
func TestOpenUploadSpecialFiles(t *testing.T) {
	try := func(t *testing.T, d *Dir, path string) string {
		done := make(chan error, 1)
		go func() {
			_, _, err := d.OpenUpload(path)
			done <- err
		}()
		select {
		case err := <-done:
			return refusal(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("the open waits for a writer")
			return ""
		}
	}

	t.Run("a FIFO", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, syscall.Mkfifo(filepath.Join(root, "pipe"), 0o600))
		assert.Contains(t, try(t, d, "pipe"), "is not a regular file")
	})

	t.Run("a FIFO that takes the name during the call", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o600))
		d.afterLstat = func() {
			require.NoError(t, os.Remove(filepath.Join(root, "a.txt")))
			require.NoError(t, syscall.Mkfifo(filepath.Join(root, "a.txt"), 0o600))
		}
		assert.Contains(t, try(t, d, "a.txt"), "changed while it was being opened")
	})

	t.Run("a socket and a folder", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, os.Mkdir(filepath.Join(root, "sub"), 0o700))
		assert.Contains(t, try(t, d, "sub"), "is not a regular file")
	})

	t.Run("a device reached through a link is not opened", func(t *testing.T) {
		d, root := openDir(t)
		require.NoError(t, os.Symlink("/dev/zero", filepath.Join(root, "zero")))
		assert.Contains(t, try(t, d, "zero"), "is a link")
	})
}
