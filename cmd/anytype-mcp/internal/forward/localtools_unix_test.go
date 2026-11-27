//go:build !windows

package forward

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadRefusesSpecialFiles(t *testing.T) {
	h := fileTools(t)
	require.NoError(t, syscall.Mkfifo(filepath.Join(h.root, "pipe"), 0o600))
	done := make(chan got, 1)
	go func() { done <- h.tool("upload_local_file", map[string]any{"space_id": "s", "path": "pipe"}) }()
	select {
	case g := <-done:
		assert.True(t, g.isError())
		assert.Contains(t, g.text(), "is not a regular file")
	case <-time.After(2 * time.Second):
		t.Fatal("the upload waits for a writer on the FIFO")
	}
	assert.Empty(t, h.rest())
}
