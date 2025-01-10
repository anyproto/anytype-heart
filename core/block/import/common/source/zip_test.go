package source

import (
	"archive/zip"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestZip(t *testing.T, files map[string]string) (string, error) {
	tmpFile, err := ioutil.TempFile(t.TempDir(), "test-*.zip")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	zipWriter := zip.NewWriter(tmpFile)
	for name, content := range files {
		writer, err := zipWriter.Create(name)
		if err != nil {
			return "", err
		}
		_, err = writer.Write([]byte(content))
		if err != nil {
			return "", err
		}
	}
	if err := zipWriter.Close(); err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

func TestZip_Initialize(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		files := map[string]string{
			"file1.txt":                          "test",
			filepath.Join("folder", "file2.txt"): "test",
		}

		zipPath, err := createTestZip(t, files)
		assert.NoError(t, err)
		defer os.Remove(zipPath)

		// when
		zipInstance := NewZip()
		err = zipInstance.Initialize(zipPath)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, zipInstance.archiveReader)
		assert.Len(t, zipInstance.fileReaders, 2)

		expectedRoots := []string{"."}
		assert.Equal(t, expectedRoots, zipInstance.rootDirs)
	})
	t.Run("zip files with dir inside", func(t *testing.T) {
		// given
		files := map[string]string{
			filepath.Join("folder", "file2.txt"):            "test",
			filepath.Join("folder", "file3.txt"):            "test",
			filepath.Join("folder", "folder1", "file4.txt"): "test",
		}

		zipPath, err := createTestZip(t, files)
		assert.NoError(t, err)
		defer os.Remove(zipPath)

		// when
		zipInstance := NewZip()
		err = zipInstance.Initialize(zipPath)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, zipInstance.archiveReader)
		assert.Len(t, zipInstance.fileReaders, 3)

		expectedRoots := []string{"folder"}
		assert.Equal(t, expectedRoots, zipInstance.rootDirs)
	})
	t.Run("zip files with 2 dirs inside", func(t *testing.T) {
		// given
		files := map[string]string{
			filepath.Join("folder", "file2.txt"):             "test",
			filepath.Join("folder1", "folder2", "file4.txt"): "test",
		}

		zipPath, err := createTestZip(t, files)
		assert.NoError(t, err)
		defer os.Remove(zipPath)

		// when
		zipInstance := NewZip()
		err = zipInstance.Initialize(zipPath)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, zipInstance.archiveReader)
		assert.Len(t, zipInstance.fileReaders, 2)

		expectedRoots := []string{"folder", filepath.Join("folder1", "folder2")}
		assert.Equal(t, expectedRoots, zipInstance.rootDirs)
	})
	t.Run("invalid path", func(t *testing.T) {
		// given
		zipInstance := NewZip()

		// when
		err := zipInstance.Initialize("invalid_path.zip")

		// then
		assert.Error(t, err)
	})
}
