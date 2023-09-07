package source

import (
	"io"
	"os"
	"path/filepath"

	"github.com/samber/lo"

	oserror "github.com/anyproto/anytype-heart/util/os"
)

type Directory struct {
	fileReaders map[string]struct{}
}

func NewDirectory() *Directory {
	return &Directory{fileReaders: make(map[string]struct{}, 0)}
}

func (d *Directory) Initialize(importPath string) error {
	files := make(map[string]struct{})
	err := filepath.Walk(importPath,
		func(path string, info os.FileInfo, err error) error {
			if info != nil && !info.IsDir() {
				files[path] = struct{}{}
			}
			return nil
		},
	)
	d.fileReaders = files
	if err != nil {
		return err
	}
	d.fileReaders = files
	return nil
}

func (d *Directory) Iterate(callback func(fileName string, fileReader io.ReadCloser) bool) error {
	for file := range d.fileReaders {
		fileReader, err := os.Open(file)
		if err != nil {
			return oserror.TransformError(err)
		}
		stop := callback(file, fileReader)
		fileReader.Close()
		if stop {
			break
		}
	}
	return nil
}

func (d *Directory) ProcessFile(fileName string, callback func(fileReader io.ReadCloser) error) error {
	if _, ok := d.fileReaders[fileName]; ok {
		fileReader, err := os.Open(fileName)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return oserror.TransformError(err)
		}
		defer fileReader.Close()
		if err = callback(fileReader); err != nil {
			return err
		}
	}
	return nil
}

func (d *Directory) CountFilesWithGivenExtensions(extension []string) int {
	var numberOfFiles int
	for name := range d.fileReaders {
		if lo.Contains(extension, filepath.Ext(name)) {
			numberOfFiles++
		}
	}
	return numberOfFiles
}

func (d *Directory) Close() {}
