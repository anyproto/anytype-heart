package source

import (
	"io"
	"os"
	"path/filepath"
)

type File struct{}

func NewFile() *File {
	return &File{}
}

func (f *File) GetFileReaders(importPath string, expectedExt []string) (map[string]io.ReadCloser, error) {
	resultPath := filepath.Clean(importPath)
	if !isSupportedExtension(filepath.Ext(importPath), expectedExt) {
		log.Errorf("not expected extension")
		return nil, nil
	}
	files := make(map[string]io.ReadCloser, 0)
	file, err := os.Open(importPath)
	if err != nil {
		return nil, err
	}
	files[resultPath] = file
	return files, nil
}
