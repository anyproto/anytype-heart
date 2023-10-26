//go:build darwin

package filetime

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExtractFileTimes(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		currDir, err := os.Getwd()
		assert.Nil(t, err)
		tmp, err := os.CreateTemp(currDir, "testfile")
		assert.Nil(t, err)
		fileInfo, err := tmp.Stat()
		assert.Nil(t, err)

		filePath := filepath.Join(currDir, fileInfo.Name())
		defer os.Remove(filePath)

		modificationTime := time.Date(2023, 9, 21, 1, 0, 0, 0, time.UTC)

		err = os.Chtimes(filePath, modificationTime, modificationTime)

		assert.Nil(t, err)

		creationTime := ExtractFileTimes(filePath) // we can't check creation time, because we can't set creation time manually

		assert.NotZero(t, creationTime)
	})
	t.Run("error", func(t *testing.T) {
		nonExistentFilePath := "non_existent_file"
		creation := ExtractFileTimes(nonExistentFilePath)

		assert.Equal(t, int64(0), creation)
	})
}
