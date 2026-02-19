package source

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/util/anyerror"
)

type OriginalFileNameGetter interface {
	GetFileOriginalName(filename string) string
}

type Zip struct {
	archiveReader             *zip.ReadCloser
	fileReaders               map[string]*zip.File
	originalToNormalizedNames map[string]string
	rootDirs                  map[string]bool
}

func NewZip() *Zip {
	return &Zip{fileReaders: make(map[string]*zip.File), originalToNormalizedNames: make(map[string]string)}
}

func (z *Zip) Initialize(importPath string) error {
	archiveReader, err := zip.OpenReader(importPath)
	z.archiveReader = archiveReader
	if err != nil {
		return err
	}
	fileReaders := make(map[string]*zip.File, len(archiveReader.File))
	filePaths := make(map[string]struct{}, len(archiveReader.File))
	for i, f := range archiveReader.File {
		if strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		normalizedName := normalizeName(f, i)
		fileReaders[normalizedName] = f
		filePaths[normalizedName] = struct{}{}
		if normalizedName != f.Name {
			z.originalToNormalizedNames[f.Name] = normalizedName
		}
	}

	z.rootDirs = findNonEmptyDirs(filePaths)
	z.fileReaders = fileReaders
	return nil
}

func normalizeName(f *zip.File, index int) string {
	fileName := f.Name
	if !utf8.ValidString(fileName) {
		fileName = fmt.Sprintf("import file %d%s", index+1, filepath.Ext(f.Name))
	}
	return fileName
}

func (z *Zip) Iterate(callback func(fileName string, fileReader io.ReadCloser) bool) error {
	for name, file := range z.fileReaders {
		fileReader, err := file.Open()
		if err != nil {
			return anyerror.CleanupError(err)
		}
		if file.FileInfo().IsDir() {
			continue
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
			return anyerror.CleanupError(err)
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

func (z *Zip) IsRootFile(fileName string) bool {
	fileDir := filepath.Dir(fileName)
	return fileDir == "." || z.rootDirs[fileDir]
}

func (z *Zip) GetFileOriginalName(fileName string) string {
	if originalName, ok := z.originalToNormalizedNames[fileName]; ok {
		return originalName
	}
	return fileName
}
