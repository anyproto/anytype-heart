package source

import (
	"io"
	"os"
	"path/filepath"
)

type Directory struct{}

func NewDirectory() *Directory {
	return &Directory{}
}

func (d *Directory) GetFileReaders(importPath string, expectedExt []string) (map[string]io.ReadCloser, error) {
	files := make(map[string]io.ReadCloser)
	err := filepath.Walk(importPath,
		func(path string, info os.FileInfo, err error) error {
			if info != nil && !info.IsDir() {
				shortPath, err := filepath.Rel(importPath+string(filepath.Separator), path)
				if err != nil {
					log.Errorf("failed to get relative path %s", err)
					return nil
				}
				if !isSupportedExtension(filepath.Ext(path), expectedExt) {
					log.Errorf("not supported extensions")
					return nil
				}
				f, err := os.Open(path)
				if err != nil {
					log.Errorf("failed to open file: %s", err)
					return nil
				}
				files[shortPath] = f
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	return files, nil
}
