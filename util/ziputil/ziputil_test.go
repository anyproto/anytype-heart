package ziputil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZipAndUnzipFolder(t *testing.T) {
	var (
		sourceDir = t.TempDir()
		tmpZipDir = t.TempDir()
		unzipDir  = t.TempDir()
	)
	zipPath := filepath.Join(tmpZipDir, "test_archive.zip")
	err := createFolders(sourceDir)
	require.NoError(t, err)
	err = ZipFolder(sourceDir, zipPath)
	require.NoError(t, err)
	err = UnzipFolder(zipPath, unzipDir)
	require.NoError(t, err)
	err = compareDirectories(sourceDir, unzipDir)
	require.NoError(t, err)
}

func createFolders(baseDir string) error {
	subDir, err := os.MkdirTemp(baseDir, "subfolder")
	if err != nil {
		return err
	}
	file1 := filepath.Join(baseDir, "file1.txt")
	if err := os.WriteFile(file1, []byte("Hello, World!"), 0700); err != nil {
		return err
	}
	file2 := filepath.Join(subDir, "file2.txt")
	if err := os.WriteFile(file2, []byte("Nested file content"), 0700); err != nil {
		return err
	}
	return nil
}

func compareDirectories(dir1, dir2 string) error {
	return filepath.Walk(dir1, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir1, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		pathInDir2 := filepath.Join(dir2, relPath)
		info2, err := os.Stat(pathInDir2)
		if err != nil {
			return err
		}
		if info.IsDir() {
			if !info2.IsDir() {
				return &mismatchError{path, pathInDir2, "dir vs file"}
			}
		} else {
			if info2.IsDir() {
				return &mismatchError{path, pathInDir2, "file vs dir"}
			}
			content1, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			content2, err := ioutil.ReadFile(pathInDir2)
			if err != nil {
				return err
			}
			if string(content1) != string(content2) {
				return &mismatchError{path, pathInDir2, "file content mismatch"}
			}
		}
		return nil
	})
}

type mismatchError struct {
	path1, path2, reason string
}

func (m *mismatchError) Error() string {
	return "Mismatch between " + m.path1 + " and " + m.path2 + ": " + m.reason
}
