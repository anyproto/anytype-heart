package files

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFile_Execute(t *testing.T) {
	t.Run("data object is wrong", func(t *testing.T) {
		// given
		file := NewFile("url")

		// when
		file.Execute("test")

		// then
		assert.Empty(t, file.GetLocalPath())
	})
	t.Run("error creating file: exist", func(t *testing.T) {
		// given
		filePath := filepath.Join("tmp", "68b26ffaae8944a7a8ab6951bdd0a44f0492fe65838858051ecaa24746d2a470")
		err := os.MkdirAll("tmp", 0700)
		assert.NoError(t, err)
		tmpFile, err := os.Create(filePath)
		assert.NoError(t, err)
		defer os.RemoveAll("tmp")

		file := NewFile("url")

		// when
		file.Execute(&DataObject{
			dirPath: "tmp",
		})

		// then
		assert.Equal(t, tmpFile.Name(), file.GetLocalPath())
	})
}
func TestFile_downloadFile(t *testing.T) {
	t.Run("download success", func(t *testing.T) {
		// given
		tmpFile, err := os.CreateTemp("", "testfile")
		assert.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("file content"))
		}))
		defer server.Close()

		file := &file{url: server.URL, File: tmpFile, localPath: tmpFile.Name()}
		ctx := context.Background()

		// when
		err = file.downloadFile(ctx)

		// then
		assert.NoError(t, err)
	})
	t.Run("download finished with http error", func(t *testing.T) {
		// given
		tmpFile, err := os.CreateTemp("", "testfile")
		assert.NoError(t, err)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		file := &file{url: server.URL, File: tmpFile}
		ctx := context.Background()

		// when
		err = file.downloadFile(ctx)

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "bad status code")
	})
	t.Run("download cancelled", func(t *testing.T) {
		// given
		tmpFile, err := os.CreateTemp("", "testfile")
		assert.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(500 * time.Millisecond) // Simulate a slow response
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("file content"))
		}))
		defer server.Close()

		file := &file{url: server.URL}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// when
		err = file.downloadFile(ctx)

		// then
		assert.Error(t, err)
	})
}

func TestFile_generateFileName(t *testing.T) {
	t.Run("generate name success", func(t *testing.T) {
		// given
		dirPath := t.TempDir()
		do := &DataObject{dirPath: dirPath}
		file := NewFile("url").(*file)

		// when
		err := file.generateFile(do)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, file.File)
	})
	t.Run("generate name, file exists", func(t *testing.T) {
		// given
		dirPath := t.TempDir()
		do := &DataObject{dirPath: dirPath}
		file := NewFile("url").(*file)

		// when
		existingFilePath := filepath.Join(dirPath, "68b26ffaae8944a7a8ab6951bdd0a44f0492fe65838858051ecaa24746d2a470")
		_, err := os.Create(existingFilePath)
		assert.NoError(t, err)

		err = file.generateFile(do)

		// then
		assert.ErrorIs(t, err, os.ErrExist)
		assert.NotNil(t, file.File)
		assert.Equal(t, existingFilePath, file.GetLocalPath())
	})
	t.Run("generate name, url error", func(t *testing.T) {
		// given
		dirPath := t.TempDir()
		do := &DataObject{dirPath: dirPath}
		file := NewFile("://invalid-url").(*file)

		// when
		err := file.generateFile(do)

		// then
		assert.Error(t, err)
		assert.Nil(t, file.File)
		assert.Equal(t, "", file.GetLocalPath())
	})
	t.Run("generate name, file creation error", func(t *testing.T) {
		// given
		dirPath := "/invalid/dir/path"
		do := &DataObject{dirPath: dirPath}
		file := NewFile("url").(*file)

		// when
		err := file.generateFile(do)

		// then
		assert.Error(t, err)
		assert.Nil(t, file.File)
		assert.Equal(t, "", file.GetLocalPath())
	})
}
