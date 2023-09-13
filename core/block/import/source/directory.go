package source

import (
	"io"
	"os"
	"path/filepath"

	oserror "github.com/anyproto/anytype-heart/util/os"
)

type Directory struct {
	fileReaders map[string]io.ReadCloser
}

func NewDirectory() *Directory {
	return &Directory{}
}

func (d *Directory) GetFileReaders(importPath string, expectedExt []string, includeFiles []string) (map[string]io.ReadCloser, error) {
	files := make(map[string]io.ReadCloser)
	err := filepath.Walk(importPath,
		func(path string, info os.FileInfo, err error) error {
			if info != nil && !info.IsDir() {
				shortPath, err := filepath.Rel(importPath+string(filepath.Separator), path)
				if err != nil {
					log.Errorf("failed to get relative path %s", err)
					return nil
				}
				if !isFileAllowedToImport(shortPath, filepath.Ext(path), expectedExt, includeFiles) {
					return nil
				}
				f, err := os.Open(path)
				if err != nil {
					log.Errorf("failed to open file: %s", oserror.TransformError(err))
					return nil
				}
				files[shortPath] = f
			}
			return nil
		},
	)
	d.fileReaders = files
	if err != nil {
		return nil, err
	}
	return files, nil
}

func (d *Directory) Close() {
	for _, fileReader := range d.fileReaders {
		fileReader.Close()
	}
}
