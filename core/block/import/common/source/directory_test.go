package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestDir(tempDir string, files map[string]string) error {
	for name, content := range files {
		fullPath := filepath.Join(tempDir, name)
		err := os.MkdirAll(filepath.Dir(fullPath), 0777)
		if err != nil {
			return err
		}
		file, err := os.Create(fullPath)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.Write([]byte(content))
		if err != nil {
			return err
		}
	}
	return nil
}

func TestDirectory_Initialize(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		files := map[string]string{
			"file1.txt":                          "test",
			filepath.Join("folder", "file2.txt"): "test",
		}
		tempDir := t.TempDir()
		err := createTestDir(tempDir, files)
		defer os.RemoveAll(tempDir)
		assert.NoError(t, err)
		// when
		directory := NewDirectory()
		err = directory.Initialize(tempDir)

		// then
		assert.NoError(t, err)
		assert.Equal(t, tempDir, directory.importPath)
		assert.Len(t, directory.fileReaders, 2)
		expectedRoots := []string{tempDir}
		assert.Equal(t, expectedRoots, directory.rootDirs)
	})
	t.Run("directory with another dir inside", func(t *testing.T) {
		// given
		files := map[string]string{
			filepath.Join("folder", "file2.txt"):            "test",
			filepath.Join("folder", "file3.txt"):            "test",
			filepath.Join("folder", "folder1", "file4.txt"): "test",
		}

		tempDir := t.TempDir()
		err := createTestDir(tempDir, files)
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// when
		directory := NewDirectory()
		err = directory.Initialize(tempDir)

		// then
		assert.NoError(t, err)
		assert.Equal(t, tempDir, directory.importPath)
		assert.Len(t, directory.fileReaders, 3)

		expectedRoots := []string{filepath.Join(tempDir, "folder")}
		assert.Equal(t, expectedRoots, directory.rootDirs)
	})
	t.Run("directory with 2 dirs inside", func(t *testing.T) {
		// given
		files := map[string]string{
			filepath.Join("folder", "file2.txt"):             "test",
			filepath.Join("folder1", "folder2", "file4.txt"): "test",
		}

		tempDir := t.TempDir()
		err := createTestDir(tempDir, files)
		assert.NoError(t, err)
		defer os.RemoveAll(tempDir)

		// when
		directory := NewDirectory()
		err = directory.Initialize(tempDir)

		// then
		assert.NoError(t, err)
		assert.Equal(t, tempDir, directory.importPath)
		assert.Len(t, directory.fileReaders, 2)

		expectedRoots := []string{filepath.Join(tempDir, "folder"), filepath.Join(tempDir, "folder1", "folder2")}
		assert.Equal(t, expectedRoots, directory.rootDirs)
	})
}
