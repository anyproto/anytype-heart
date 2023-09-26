package source

import (
	"io"
	"os"
	"path/filepath"

	"github.com/samber/lo"

	oserror "github.com/anyproto/anytype-heart/util/os"
)

type File struct {
	fileName string
}

func NewFile() *File {
	return &File{}
}

func (f *File) Initialize(importPath string) error {
	f.fileName = importPath
	return nil
}

func (f *File) Iterate(callback func(fileName string, fileReader io.ReadCloser) bool) error {
	fileReader, err := os.Open(f.fileName)
	if err != nil {
		return oserror.TransformError(err)
	}
	defer fileReader.Close()
	callback(f.fileName, fileReader)
	return nil
}

func (f *File) ProcessFile(fileName string, callback func(fileReader io.ReadCloser) error) error {
	fileReader, err := os.Open(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return oserror.TransformError(err)
	}
	defer fileReader.Close()
	return callback(fileReader)
}

func (f *File) CountFilesWithGivenExtensions(extension []string) int {
	if lo.Contains(extension, filepath.Ext(f.fileName)) {
		return 1
	}
	return 0
}

func (f *File) Close() {}
