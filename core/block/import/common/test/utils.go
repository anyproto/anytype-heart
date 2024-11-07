package test

import (
	"archive/zip"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func CreateEmptyZip(t *testing.T, zipFileName string) error {
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return fmt.Errorf("Failed to create zip file: %w\n", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer func() {
		err = zipWriter.Close()
		assert.NoError(t, err)
	}()
	return nil
}
