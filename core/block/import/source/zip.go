package source

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/samber/lo"

	oserror "github.com/anyproto/anytype-heart/util/os"
)

type Zip struct {
	archiveReader *zip.ReadCloser
	fileReaders   map[string]*zip.File
}

func NewZip() *Zip {
	return &Zip{fileReaders: make(map[string]*zip.File, 0)}
}

func (z *Zip) Initialize(importPath string) error {
	archiveReader, err := zip.OpenReader(importPath)
	z.archiveReader = archiveReader
	if err != nil {
		return err
	}
	fileReaders := make(map[string]*zip.File, len(archiveReader.File))
	for i, f := range archiveReader.File {
		if strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		fileReaders[normalizeName(f, i)] = f
	}
	z.fileReaders = fileReaders
	return nil
}

func normalizeName(f *zip.File, index int) string {
	fileName := f.Name
	if f.NonUTF8 {
		fileName = fmt.Sprintf("import file %d%s", index+1, filepath.Ext(f.Name))
	}
	return fileName
}

func (z *Zip) Iterate(callback func(fileName string, fileReader io.ReadCloser) bool) error {
	for name, file := range z.fileReaders {
		fileReader, err := file.Open()
		if err != nil {
			return oserror.TransformError(err)
		}
		isContinue := callback(name, fileReader)
		fileReader.Close()
		if !isContinue {
			break
		}
	}
	return nil
}

func (z *Zip) ProcessFile(fileName string, callback func(fileReader io.ReadCloser) error) error {
	if file, ok := z.fileReaders[fileName]; ok {
		fileReader, err := file.Open()
		if err != nil {
			return oserror.TransformError(err)
		}
		defer fileReader.Close()
		if err = callback(fileReader); err != nil {
			return err
		}
	}
	return nil
}

func (z *Zip) CountFilesWithGivenExtensions(extension []string) int {
	var numberOfFiles int
	for name := range z.fileReaders {
		if lo.Contains(extension, filepath.Ext(name)) {
			numberOfFiles++
		}
	}
	return numberOfFiles
}

func (z *Zip) Close() {
	if z.archiveReader != nil {
		z.archiveReader.Close()
	}
}
