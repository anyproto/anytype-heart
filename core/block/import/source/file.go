package source

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type File struct{}

func NewFile() *File {
	return &File{}
}

func (f *File) GetFileReaders(importPath, expectedExt string) (map[string]io.ReadCloser, error) {
	shortPath := filepath.Clean(importPath)
	actualExt := filepath.Ext(importPath)
	if !strings.EqualFold(actualExt, expectedExt) {
		return nil, fmt.Errorf("not expected extension: %s, %s", expectedExt, actualExt)
	}
	files := make(map[string]io.ReadCloser, 0)
	file, err := os.Open(importPath)
	if err != nil {
		return nil, err
	}
	files[shortPath] = file
	return files, nil
}
