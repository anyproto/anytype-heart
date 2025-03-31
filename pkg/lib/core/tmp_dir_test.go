package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTempDirService(t *testing.T) {
	t.Run("cleanup", func(t *testing.T) {
		// given
		s := NewTempDirService()
		s.tempDir = t.TempDir()

		oldFile := filepath.Join(s.tempDir, "old.txt")
		require.NoError(t, os.WriteFile(oldFile, []byte("old"), 0600))
		oldTime := time.Now().Add(-100 * time.Hour)
		require.NoError(t, os.Chtimes(oldFile, oldTime, oldTime))

		newFile := filepath.Join(s.tempDir, "new.txt")
		require.NoError(t, os.WriteFile(newFile, []byte("new"), 0600))

		nestedDir := filepath.Join(s.tempDir, "nested")
		require.NoError(t, os.MkdirAll(nestedDir, 0755))
		nestedOldFile := filepath.Join(nestedDir, "nested_old.txt")
		require.NoError(t, os.WriteFile(nestedOldFile, []byte("nested old"), 0600))
		require.NoError(t, os.Chtimes(nestedOldFile, oldTime, oldTime))
		nestedNewFile := filepath.Join(nestedDir, "nested_new.txt")
		require.NoError(t, os.WriteFile(nestedNewFile, []byte("nested new"), 0600))

		// when
		s.cleanUp()

		// then
		_, err := os.Stat(oldFile)
		require.True(t, os.IsNotExist(err), "old file should be deleted")

		_, err = os.Stat(newFile)
		require.NoError(t, err, "new file should remain")

		_, err = os.Stat(nestedOldFile)
		require.True(t, os.IsNotExist(err), "nested old file should be deleted")

		_, err = os.Stat(nestedNewFile)
		require.NoError(t, err, "nested new file should remain")
	})
}
