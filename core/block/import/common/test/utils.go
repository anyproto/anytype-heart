package test

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func CreateEmptyZip(t *testing.T, zipFileName string) {
	zipFile, err := os.Create(zipFileName)
	assert.NoError(t, err)
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	err = zipWriter.Close()
	assert.NoError(t, err)
}

func CreateZipWithFiles(t *testing.T, zipFileName, testDataDir string, files []*zip.FileHeader) {
	zipFile, err := os.Create(zipFileName)
	assert.NoError(t, err)
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer func() {
		err = zipWriter.Close()
		assert.NoError(t, err)
	}()

	for _, file := range files {
		writer, err := zipWriter.CreateHeader(file)
		assert.NoError(t, err)
		fileReader, err := os.Open(filepath.Join(testDataDir, file.Name))
		assert.NoError(t, err)
		_, err = io.Copy(writer, fileReader)
		assert.NoError(t, err)
	}
}
